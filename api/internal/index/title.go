package index

import (
	"sort"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// A title column packs a model.Title into one field: the native original,
// followed by one "lang=translation" element per translation, sorted by
// language so the file is byte-stable across runs.
//
//	原作|en=Fate/stay night|ja=フェイト
//
// The original may be empty, leaving a leading empty element:
//
//	|en=Season 2

// packTitle encodes a title into its column form.
func packTitle(t model.Title) string {
	langs := make([]string, 0, len(t.Translations))
	for lang := range t.Translations {
		langs = append(langs, lang)
	}
	sort.Strings(langs)

	var b strings.Builder
	b.WriteString(escape(t.Original))
	for _, lang := range langs {
		b.WriteByte('|')
		b.WriteString(escape(lang))
		b.WriteByte('=')
		b.WriteString(escape(t.Translations[lang]))
	}
	return b.String()
}

// unpackTitle decodes a title column. It allocates, so it is called only for
// the rows a response actually carries, never while filtering.
func unpackTitle(field string) model.Title {
	var t model.Title
	first := true
	eachElement(field, func(elem string) bool {
		if first {
			first = false
			t.Original = unescape(elem)
			return true
		}
		i := indexUnescaped(elem, '=')
		if i < 0 {
			return true
		}
		if t.Translations == nil {
			t.Translations = map[string]string{}
		}
		t.Translations[unescape(elem[:i])] = unescape(elem[i+1:])
		return true
	})
	return t
}

// titleMatches reports whether any form of the packed title contains query,
// case-insensitively.
//
// It matches against the decoded values one at a time rather than against the
// raw column, so a query containing "|" or "=" cannot match the packing itself
// and report a title that does not contain it. It allocates only when a value
// actually carries an escape sequence.
func titleMatches(field, query string) bool {
	found := false
	eachElement(field, func(elem string) bool {
		value := elem
		// Every element after the first is "lang=value"; the language is not
		// part of the title and must not be searched, or every Japanese title
		// would match the query "ja".
		if i := indexUnescaped(elem, '='); i >= 0 {
			value = elem[i+1:]
		}
		if containsFold(unescape(value), query) {
			found = true
			return false
		}
		return true
	})
	return found
}

// NormalizeQuery lowercases and trims a search term. Every name and title
// search goes through it so they cannot drift apart in case or whitespace.
func NormalizeQuery(query string) string {
	return strings.TrimSpace(strings.ToLower(query))
}
