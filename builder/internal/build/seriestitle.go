package build

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// fillSeriesTitle fills a series' English title from the Wikidata work its
// externalIds name.
//
// A series is the one node with no AniList id — it spans several of them, one
// per installment — so the offline database cannot reach it and every series
// title is authored. That is also why it is the node most worth filling: the
// English title is the field the open catalogues do not carry in a usable form
// (anime-offline-database has a single untagged `synonyms` bag, which for
// 葬送のフリーレン holds four English strings and no way to tell the licensed
// title from three literal glosses), while Wikidata asserts exactly one.
//
// An authored title always wins, and a series with no wikidataId is left
// exactly as it is today — with an `en` absent, the API answers an English
// request with the romanization, which is the existing behaviour and not a
// regression.
func (b *Builder) fillSeriesTitle(s *model.Series, report *Report) {
	qid := s.ExternalIDs.WikidataID
	if qid == "" || b.sources.Wikidata == nil {
		// A series with no wikidataId is not a guess the builder made, so it
		// earns no note: the report is for low-confidence decisions, and 149 of
		// 152 series carrying an identical "not configured" line would bury the
		// ones that say something. The research script lists the backlog.
		return
	}
	if _, authored := s.Titles.Translations["en"]; authored {
		return
	}
	entity := "series " + s.ID
	ent, ok := b.sources.Wikidata.Lookup(qid)
	if !ok {
		report.add(entity, "titles", fmt.Sprintf("Wikidata %s not in cache; English title not filled (run `builder init`)", qid))
		return
	}

	// P1476 first, the label second: see wikidata.titleProperty.
	en := ent.Titles["en"]
	fromLabel := en == ""
	if fromLabel {
		en = ent.Labels["en"]
	}
	if en == "" {
		report.add(entity, "titles", fmt.Sprintf("Wikidata %s has no English title or label; en left unset", qid))
		return
	}
	if existing, isRendering := isRomanizationOf(en, s.Titles); isRendering {
		report.add(entity, "titles", fmt.Sprintf(
			"Wikidata %s offers %q as English, but it is a rendering of %q rather than a translation of it; en left unset",
			qid, en, existing))
		return
	}
	addTranslation(&s.Titles, "en", en)
	if fromLabel {
		// A P1476 claim states the work's title and needs no second opinion. A
		// label is a name for the Wikidata item, which usually coincides with
		// the English title and sometimes — where the English Wikipedia article
		// is at the shortest unambiguous name — is shorter than it. Worth a
		// human glance, so it says where the value came from.
		report.add(entity, "titles", fmt.Sprintf(
			"filled translations.en from the Wikidata %s label (no P1476 title claim): %q", qid, en))
	}
}

// isRomanizationOf reports whether a candidate English title is really the
// title we already hold, written in Latin letters — returning the form it
// matched so the report can name it.
//
// Two ways a candidate arrives that is not a translation:
//
//   - a macron-spelled romanization, where Wikidata writes Hyakki Yakōshō for
//     the Hyakki Yakou Shou we already store as ja-Latn; and
//   - a Latin-script original, where the label simply repeats the title —
//     Fate/stay night, Golden Kamuy, Jujutsu Kaisen are written that way in
//     Japan too, and nobody translated them.
//
// Filing either under `en` asserts that an English title exists when none
// does, which is the fact this dataset keeps apart from a romanization (see
// inferTitle). So the candidate is compared against both the romanization and
// the original, and a match means the title is dropped rather than filed.
//
// The comparison is deliberately lossy — it folds case, punctuation, spacing,
// macrons and the long-vowel spellings that distinguish one Hepburn convention
// from another. Over-matching costs a real English title that a human can still
// author by hand, and that the report names; under-matching puts a romanization
// back into `en`, which is the bug this guards.
func isRomanizationOf(candidate string, t model.Title) (string, bool) {
	want := foldRomanization(candidate)
	if want == "" {
		return "", false
	}
	for _, existing := range append([]string{t.Original}, romanizations(t)...) {
		if existing != "" && foldRomanization(existing) == want {
			return existing, true
		}
	}
	return "", false
}

// romanizations returns every Latin-script rendering the title carries, in a
// fixed order.
func romanizations(t model.Title) []string {
	var out []string
	for _, code := range sortedCodes(t) {
		if t.Translations[code] != "" && model.IsRomanization(code) {
			out = append(out, t.Translations[code])
		}
	}
	return out
}

// longVowels flattens the ways a Japanese long vowel gets written into Latin
// letters. The macrons of Hepburn (ō) and the circumflexes of Nihon-shiki (ô)
// are stripped to the bare vowel, and the digraphs that spell the same sound
// without a diacritic ("ou", "uu") follow.
//
// This is a closed set — Japanese has five vowels and two conventions for
// marking them long — so it is written out rather than reached for through a
// Unicode normalization dependency the builder does not otherwise need.
var longVowels = strings.NewReplacer(
	"ā", "a", "ē", "e", "ī", "i", "ō", "o", "ū", "u",
	"â", "a", "ê", "e", "î", "i", "ô", "o", "û", "u",
	"ou", "o", "oo", "o", "uu", "u", "aa", "a", "ei", "e",
)

// nonAlphanumeric matches everything punctuation, spacing and symbols, which
// carry no information for this comparison — "Fate/stay night" and
// "Fate/Stay Night" are the same string for our purposes.
var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

// foldRomanization reduces a Latin-script title to a form in which two
// spellings of the same romanization compare equal. A title in native script
// folds to the empty string, which isRomanizationOf never matches — 葬送のフリーレン
// is not a spelling of anything written in Latin letters.
func foldRomanization(s string) string {
	return nonAlphanumeric.ReplaceAllString(longVowels.Replace(strings.ToLower(s)), "")
}
