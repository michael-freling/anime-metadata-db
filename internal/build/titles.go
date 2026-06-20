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
	if original == "" {
		notes = append(notes, "no native-script title found; original left empty")
	}
	if latin != "" {
		title.Translations = map[string]string{"en": latin}
		notes = append(notes, fmt.Sprintf("assumed %q is English (en); could be a romanization (ja-Latn)", latin))
	}
	return title, notes
}
