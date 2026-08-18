// Package index defines the compact listing index the API serves browse,
// search and catalog requests from.
//
// # Why this exists
//
// data/ is the product: one reviewable YAML record per series, plus the staff
// files. The API used to load all of it at startup and keep the whole object
// graph resident. Measured against a dataset scaled to the full upstream
// catalogue (~41k works, 97 MB of YAML), that costs 9.7 s of parsing, 2.8 GB of
// allocation and a 443 MB peak heap — paid on every boot of the server, which
// scales to zero between visits.
//
// Almost none of that is needed to answer a listing. A browse page shows a
// title, a year, a quarter and an episode count; the episodes, appearances and
// watch orders behind it are only ever read one id at a time. So the builder
// emits the listing fields into a single index file, and the server reads the
// full YAML record lazily, for the one id a detail request names.
//
// # Why a flat text file
//
// The index is embedded with go:embed into a string. A Go string constant lives
// in the binary's read-only data, so holding it costs no heap and no startup
// work — the OS pages it in on demand. Boot is one scan that records byte
// offsets; no field is copied out of the blob until a request materialises the
// twenty rows it is actually returning.
//
// It stays text, one record per line, in deterministic order, because it is
// committed alongside the data it describes and has to survive review: a
// changed title should show up as a one-line diff. It is generated, never
// hand-edited, and CI regenerates it to check it still matches data/.
//
// # Format
//
// A version line, then sections. Each section opens with a header line naming
// its columns, so the file describes its own schema:
//
//	!anime-metadata-db-index	1
//	!stats	franchises=1	series=152	...
//	!franchises	id	file	titles
//	fate	fate.yaml	|en=Fate|ja=フェイト
//	!series	id	file	franchise	titles
//	fate-stay-night	fate.yaml	fate	Fate/stay night|en=Fate/stay night
//	...
//
// Columns are tab-separated. A column holding a list uses "|" between elements;
// a title column is the native original followed by "lang=translation" pairs.
// Tab, newline, backslash, "|" and "=" are backslash-escaped inside a value, so
// every line has exactly as many tabs as its header.
//
// Rows reference each other by row number within a section rather than by id,
// which keeps the file compact and makes a lookup an array index instead of a
// map probe.
package index

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Path is where the generated index lives, relative to the repository root.
// The API embeds it from here; cmd/index writes it here.
const Path = "data/index.tsv"

// Version is the format version written into the first line. A reader refuses
// anything else: the index is generated from data/ by the same build that
// serves it, so a mismatch means the two have drifted, which is a bug rather
// than a case to migrate.
const Version = 1

// magic opens the file. It names the project so that a stray copy of the file
// is identifiable on its own.
const magic = "!anime-metadata-db-index"

// The section names, in the order the writer emits them. Order matters: a
// section may only reference rows of a section already read, so a single
// forward pass resolves every reference.
const (
	sectionStats      = "stats"
	sectionFranchises = "franchises"
	sectionSeries     = "series"
	sectionEntries    = "entries"
	sectionWorks      = "works"
	sectionCharacters = "characters"
	sectionStaff      = "staff"
	sectionCredits    = "credits"
)

// span locates one value inside the blob. Two uint32s rather than a string
// header (which would be 16 bytes and, worse, a pointer the collector has to
// trace) — at a few hundred thousand rows the difference is the whole point of
// the design.
type span struct {
	off, len uint32
}

// text returns the value, still escaped, without copying.
func (s span) text(blob string) string {
	return blob[s.off : s.off+s.len]
}

// escape encodes one value for a column. The five escaped bytes are the ones
// that carry structure: the field, line, list and key separators, and the
// escape itself.
func escape(v string) string {
	if !strings.ContainsAny(v, "\\\t\n|=") {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 8)
	for _, r := range v {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '|':
			b.WriteString(`\p`)
		case '=':
			b.WriteString(`\e`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unescape reverses escape. An unknown escape sequence is left as-is rather
// than rejected: the reader's job is to serve data, and a malformed byte in one
// title should not take the whole dataset down.
func unescape(v string) string {
	if !strings.ContainsRune(v, '\\') {
		return v
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 >= len(v) {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch v[i] {
		case '\\':
			b.WriteByte('\\')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'p':
			b.WriteByte('|')
		case 'e':
			b.WriteByte('=')
		default:
			b.WriteByte('\\')
			b.WriteByte(v[i])
		}
	}
	return b.String()
}

// eachElement calls fn for each "|"-separated element of an escaped list
// column, without allocating. It stops early if fn returns false.
func eachElement(field string, fn func(elem string) bool) {
	for field != "" {
		var elem string
		if i := indexUnescaped(field, '|'); i >= 0 {
			elem, field = field[:i], field[i+1:]
		} else {
			elem, field = field, ""
		}
		if !fn(elem) {
			return
		}
	}
}

// indexUnescaped finds the first unescaped occurrence of sep. A backslash
// always escapes exactly the byte after it, so scanning forward and skipping a
// byte after each backslash identifies separators unambiguously.
func indexUnescaped(s string, sep byte) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case sep:
			return i
		}
	}
	return -1
}

// containsFold reports whether haystack contains needle, case-insensitively.
//
// strings.Contains(strings.ToLower(h), strings.ToLower(n)) would be one line,
// but it allocates a lowered copy of every title on every search. At a few
// hundred thousand values per request that is the difference between a search
// that costs microseconds and one that costs a garbage collection.
//
// Both sides are folded rather than requiring a pre-lowered needle: callers do
// normalise queries, but a function that quietly returns the wrong answer for
// an unnormalised argument is a trap, and folding the needle costs one
// comparison per rune rather than one allocation per value.
func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	// Match the first rune before decoding the rest. A search scans every title
	// in the dataset, and almost every position fails on the first rune, so the
	// full comparison is worth avoiding rather than entering and abandoning.
	first, _ := utf8.DecodeRuneInString(needle)
	first = unicode.ToLower(first)
	for i := 0; i < len(haystack); {
		r, size := utf8.DecodeRuneInString(haystack[i:])
		if unicode.ToLower(r) == first && hasPrefixFold(haystack[i:], needle) {
			return true
		}
		i += size
	}
	return false
}

// hasPrefixFold reports whether s starts with needle, case-insensitively.
func hasPrefixFold(s, needle string) bool {
	for len(needle) > 0 {
		if len(s) == 0 {
			return false
		}
		sr, sn := utf8.DecodeRuneInString(s)
		nr, nn := utf8.DecodeRuneInString(needle)
		if unicode.ToLower(sr) != unicode.ToLower(nr) {
			return false
		}
		s, needle = s[sn:], needle[nn:]
	}
	return true
}

// parseInt reads a column as an integer, treating an empty column as zero so
// the writer can omit defaults.
func parseInt(field string) (int, error) {
	if field == "" {
		return 0, nil
	}
	return strconv.Atoi(field)
}

// formatInt writes an integer column, omitting zero.
func formatInt(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}

// errRow reports a malformed row with enough context to find it in the file.
func errRow(section string, line int, err error) error {
	return fmt.Errorf("index: %s section, line %d: %w", section, line, err)
}
