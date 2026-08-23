package build

import (
	"unicode"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// nativeScripts are the Unicode ranges that mark a title as native (CJK) script
// rather than a Latin romanization.
var nativeScripts = []*unicode.RangeTable{
	unicode.Han,
	unicode.Hiragana,
	unicode.Katakana,
	unicode.Hangul,
}

// romanizedLang is the BCP-47 tag for a romanized Japanese title, the form the
// upstream catalogues supply as their Latin-script main title.
const romanizedLang = "ja-Latn"

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
// Latin main title becomes the `ja-Latn` translation. fillTitles merges this
// into any title the override already authored, and reports the guesses.
//
// The Latin title is a *romanization*, not a translation: the upstream
// catalogues carry "Kimetsu no Yaiba", not "Demon Slayer". Filing it under `en`
// — which this did until it was measured, for 147 of 152 series — tells every
// consumer asking for English that a transliteration is an English title. An
// English title is a different fact, and one only a human can supply, so it
// stays in the overrides.
//
// `ja-Latn` assumes the original is Japanese, which is true of this catalogue
// bar a handful of Korean and Chinese titles. Those are authored with their own
// romanization tag, and fillTitles leaves any authored romanization alone.
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
		if original == "" {
			// Nothing native to romanize: the Latin string is the title, the
			// way "Fate/stay night" is written that way in Japan too. Calling
			// it a romanization would invent a native form that does not exist.
			title.Original = latin
		} else {
			translations[romanizedLang] = latin
		}
	}
	if len(translations) > 0 {
		title.Translations = translations
	}
	return title
}

// hasRomanization reports whether a title already carries one, in any language.
func hasRomanization(t model.Title) bool {
	for code, val := range t.Translations {
		if val != "" && model.IsRomanization(code) {
			return true
		}
	}
	return false
}
