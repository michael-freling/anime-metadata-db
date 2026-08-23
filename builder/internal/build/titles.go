package build

import (
	"sort"
	"strings"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

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
// Which language the native title is in is decided by its script where the
// script decides it: kana is Japanese and Hangul is Korean, both unambiguously.
// A title written only in Han characters could be either Japanese or Chinese —
// 呪術廻戦 and 喜羊羊与灰太狼 look alike to a range check — so it defaults to
// Japanese, this being a catalogue of anime, and fillTitles reports the
// assumption so a human can correct it by authoring the language.
func inferTitle(a offlinedb.Anime) model.Title {
	var original, latin string
	if model.HasNativeScript(a.Title) {
		original = a.Title
	} else {
		latin = a.Title
	}
	if original == "" {
		for _, syn := range a.Synonyms {
			if model.HasNativeScript(syn) {
				original = syn
				break
			}
		}
	}

	title := model.Title{Original: original}
	translations := map[string]string{}
	lang, _ := model.NativeLanguage(original)
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

// romanizationLanguage returns the language named by a title's romanization
// tag — "ko" for `ko-Latn` — or "" when it carries none. An authored one is the
// author saying what language the title is in, which the build cannot infer.
func romanizationLanguage(t model.Title) string {
	for _, code := range sortedCodes(t) {
		if t.Translations[code] != "" && model.IsRomanization(code) {
			return primaryTag(code)
		}
	}
	return ""
}

// hasRomanization reports whether a title already carries one, in any language.
func hasRomanization(t model.Title) bool {
	return romanizationLanguage(t) != ""
}

// sortedCodes returns a title's language tags in a fixed order, so a title with
// more than one romanization always yields the same answer.
func sortedCodes(t model.Title) []string {
	codes := make([]string, 0, len(t.Translations))
	for code := range t.Translations {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// primaryTag returns the primary subtag of a BCP-47 tag ("ja-Latn" -> "ja").
func primaryTag(tag string) string {
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		return tag[:i]
	}
	return tag
}
