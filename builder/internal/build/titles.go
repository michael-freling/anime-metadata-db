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

// nativeLanguage names the language a native-script title is written in.
//
// Kana occurs only in Japanese and Hangul only in Korean, so either settles it.
// Han characters are shared: 呪術廻戦 and 喜羊羊与灰太狼 look alike to a range
// check. This is a catalogue of anime, so a Han-only title defaults to Japanese
// — and says so by returning certain=false, which fillTitles reports for a
// human to confirm or correct. Guessing loudly beats both guessing silently
// (which mislabelled every Korean title before this) and refusing to guess
// (which would tag 劇場版「…」 as an undetermined language it plainly is not).
func nativeLanguage(s string) (lang string, certain bool) {
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			return "ja", true
		case unicode.In(r, unicode.Hangul):
			return "ko", true
		}
	}
	if hasNativeScript(s) {
		return "ja", false
	}
	return "", false
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
// Which language the native title is in is decided by its script, not assumed:
// kana is Japanese and Hangul is Korean, both unambiguously. A title written
// only in Han characters could be either Japanese or Chinese — 呪術廻戦 and
// 喜羊羊与灰太狼 look alike to a range check — so no language is claimed for it
// and the guess is reported for a human instead. Nothing is lost by that: the
// language key duplicates `original`, and a request in the original's language
// resolves to `original` anyway.
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
	lang, _ := nativeLanguage(original)
	if lang != "" {
		translations[lang] = original
	}
	if latin != "" {
		if original == "" {
			// Nothing native to romanize: the Latin string is the title, the
			// way "Fate/stay night" is written that way in Japan too. Calling
			// it a romanization would invent a native form that does not exist.
			title.Original = latin
		} else {
			translations[lang+"-Latn"] = latin
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
