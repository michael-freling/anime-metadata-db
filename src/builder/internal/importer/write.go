package importer

import (
	"fmt"
	"strings"
)

// Render writes one override file.
//
// Every value goes through scalar, ids included. A slug is usually plainly safe
// — lowercase ASCII and hyphens — but not always: a work titled "663114" slugs
// to exactly that, and written bare it parses as an integer and fails the
// schema, which requires a string.
//
// Composed as text rather than marshalled from a struct because the authored
// layer carries comments and flow maps that a round-trip through a YAML encoder
// would discard, and these files are read and edited by people — the comment
// saying not to hand-edit data/ is the point of the comment.
func Render(s Series, year int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Authored override — %d catalogue import. Holds only the structure the\n", year)
	b.WriteString("# open sources cannot express; `builder build` fills the facts into data/.\n")
	b.WriteString("# Never hand-edit data/.\n")
	b.WriteString("series:\n")
	fmt.Fprintf(&b, "  id: %s\n", scalar(s.ID))
	b.WriteString("  titles:\n")
	fmt.Fprintf(&b, "    original: %s\n", scalar(s.Original))
	fmt.Fprintf(&b, "    translations: { ja-Latn: %s }\n", scalar(s.Romanized))
	if len(s.Seasons) > 0 {
		b.WriteString("  seasons:\n")
		for i, id := range s.Seasons {
			fmt.Fprintf(&b, "    - id: %s\n      number: %d\n", scalar(id), i+1)
		}
	}
	if len(s.Movies) > 0 {
		b.WriteString("  movies:\n")
		for _, id := range s.Movies {
			fmt.Fprintf(&b, "    - id: %s\n", scalar(id))
		}
	}
	b.WriteString("# One linear storyline: episodes carry a continuous absoluteNumber.\n")
	fmt.Fprintf(&b, "numbered: [%s]\n", scalar(s.ID))
	return b.String()
}

// scalar renders a title as a YAML scalar, quoting unless the value is plainly
// safe unquoted.
//
// Inverted deliberately: the test is what may stay bare, not what must be
// quoted. Three earlier drafts reasoned the other way and all three were
// wrong — one missed that a plain scalar may carry interior double quotes, one
// let the title "null" through (which parses back as YAML's null), and one
// checked the indicator characters only at the start of the value, so
// "Gochuumon wa Usagi desu ka? Bloom" broke the flow mapping it sits in. A
// round-trip test over every title this writes is what proves this version
// right, rather than a reading of the spec.
//
// Quoting is hand-rolled rather than delegated to the YAML encoder because the
// encoder folds long scalars across lines, and these values sit in a template
// that expects one line.
func scalar(v string) string {
	if plainSafe(v) {
		return v
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

// yamlKeywords are the values YAML 1.1 and 1.2 resolve to something other than
// a string. A title that happens to be one of them has to be quoted or it comes
// back as a bool, a null, or nothing at all.
var yamlKeywords = map[string]bool{
	"y": true, "yes": true, "n": true, "no": true, "true": true, "false": true,
	"on": true, "off": true, "null": true, "nil": true, "~": true, "": true,
	".inf": true, "-.inf": true, ".nan": true,
}

// flowIndicators are structural wherever they appear in a value, not only at
// its start, because the ja-Latn title is written inside a flow mapping.
// "?" is here for the same reason: "ka? Bloom" is a complex-key indicator
// followed by a space.
const flowIndicators = ",[]{}:#?"

// plainSafe reports whether v can be written without quotes.
func plainSafe(v string) bool {
	if yamlKeywords[strings.ToLower(v)] {
		return false
	}
	if v == "" || strings.TrimSpace(v) != v {
		return false
	}
	// Anything the resolver might read as a number.
	if strings.IndexFunc(v, func(r rune) bool {
		return !strings.ContainsRune("0123456789.-+eExXaAbBcCdDfF_", r)
	}) == -1 {
		return false
	}
	if strings.ContainsAny(v, flowIndicators) {
		return false
	}
	// These change meaning only in first position: a leading "-" starts a
	// sequence entry, a leading "!" a tag, a leading quote a quoted scalar.
	// Mid-value they are ordinary text, and they are ordinary in most
	// romanised titles — "Kaguya-sama", "Heaven's Feel", "Aikatsu!" — so
	// rejecting them anywhere would quote half the corpus for nothing.
	if strings.ContainsAny(v[:1], "-&*!|>'\"%@`") {
		return false
	}
	return !strings.ContainsAny(v, "\t\n")
}
