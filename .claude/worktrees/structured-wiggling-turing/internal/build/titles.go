package build

import (
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
// native-script form becomes the original and the `ja` translation, and the
// Latin main title becomes the `en` translation. Both are filled by default so
// generated records carry English and Japanese names. fillTitles merges this
// into any title the override already authored, and reports the guesses.
func inferTitle(a offlinedb.Anime) model.Title {
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
				break
			}
		}
	}

	title := model.Title{Original: original}
	translations := map[string]string{}
	if original != "" {
		// Anime originals are Japanese: expose the native title under `ja`.
		translations["ja"] = original
	}
	if latin != "" {
		// The dump's Latin main title — the common English/romanized name.
		translations["en"] = latin
	}
	if len(translations) > 0 {
		title.Translations = translations
	}
	return title
}
