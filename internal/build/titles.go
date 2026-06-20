package build

import (
	"fmt"
	"unicode"

	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/sources/offlinedb"
)

// nativeScripts are the Unicode ranges that mark a title as native (CJK) script
// rather than a Latin romanization.
var nativeScripts = []*unicode.RangeTable{
	unicode.Han,
	unicode.Hiragana,
	unicode.Katakana,
	unicode.Hangul,
}

// hasNativeScript reports whether s contains any CJK/Hangul character.
func hasNativeScript(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && unicode.In(r, nativeScripts...) {
			return true
		}
	}
	return false
}

// inferTitle best-effort derives a Title from an offline-database entry: the
// native-script form becomes original, the Latin main title becomes the en
// translation. It returns notes describing every low-confidence guess so the
// build report can flag them for review (design Part 4 / Part 8).
func inferTitle(a offlinedb.Anime) (model.Title, []string) {
	var notes []string
	var original, latin string

	if hasNativeScript(a.Title) {
		original = a.Title
	} else {
		latin = a.Title
	}

	if original == "" {
		for _, syn := range a.Synonyms {
			if hasNativeScript(syn) {
				original = syn
				notes = append(notes, fmt.Sprintf("chose synonym %q as original (native script)", syn))
				break
			}
		}
	}

	title := model.Title{Original: original}
	translations := map[string]string{}
	if original != "" {
		// Anime originals are Japanese by default: expose the native title under
		// the `ja` key, and treat the dump's Latin main title as its Japanese
		// romanization (`ja-Latn`).
		translations["ja"] = original
		if latin != "" {
			translations["ja-Latn"] = latin
			notes = append(notes, fmt.Sprintf("tagged %q as ja-Latn (romanization); set translations.en via an override if it is English", latin))
		}
	} else {
		// No native script found, so the language of the Latin title is unknown;
		// fall back to a best-effort English tag.
		notes = append(notes, "no native-script title found; original left empty")
		if latin != "" {
			translations["en"] = latin
			notes = append(notes, fmt.Sprintf("assumed %q is English (en)", latin))
		}
	}
	if len(translations) > 0 {
		title.Translations = translations
	}
	return title, notes
}
