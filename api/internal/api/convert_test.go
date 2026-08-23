package api

import (
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

func TestNewLocalizer(t *testing.T) {
	tests := []struct {
		in   string
		lang string
		full bool
	}{
		{"", "en", false},                  // no header defaults to English
		{"*", "en", true},                  // wildcard asks for every language
		{"en", "en", false},                // simple tag
		{"  EN  ", "en", false},            // trimmed + lowercased
		{"en-US,en;q=0.9", "en-us", false}, // first entry wins
		{"ja;q=0.8", "ja", false},          // q-weight stripped
		{";q=1", "en", false},              // empty primary tag defaults to English
	}
	for _, tc := range tests {
		l := newLocalizer(tc.in)
		if l.lang != tc.lang || l.full != tc.full {
			t.Errorf("newLocalizer(%q) = {lang:%q full:%v}, want {lang:%q full:%v}", tc.in, l.lang, l.full, tc.lang, tc.full)
		}
	}
}

func TestResolveTitle(t *testing.T) {
	mk := func(orig string, tr map[string]string) model.Title {
		return model.Title{Original: orig, Translations: tr}
	}
	tests := []struct {
		name string
		lang string
		in   model.Title
		want string
	}{
		{"exact language", "ja", mk("", map[string]string{"ja": "じゃ", "en": "en"}), "じゃ"},
		{"primary subtag", "en-us", mk("", map[string]string{"en": "English"}), "English"},
		{"non-english prefers native original", "ja", mk("ネイティブ", map[string]string{"en": "English"}), "ネイティブ"},
		{"english fallback", "fr", mk("", map[string]string{"en": "English"}), "English"},
		{"english request uses original when no en", "en", mk("Native", nil), "Native"},
		{"deterministic first translation", "fr", mk("", map[string]string{"zz": "Z", "aa": "A"}), "A"},
		{"empty title", "en", mk("", nil), ""},

		// Most of the catalogue has a romanization and no English title. An
		// English request must reach it rather than falling through to a
		// native script the reader cannot read.
		{"english falls back to a romanization, not to kanji", "en",
			mk("鬼滅の刃", map[string]string{"ja": "鬼滅の刃", "ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		{"a real english title still wins over the romanization", "en",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba", "en": "Demon Slayer"}), "Demon Slayer"},
		{"japanese still gets native script, never its own romanization", "ja",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "鬼滅の刃"},
		{"any other language reaches the romanization too", "fr",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		// Accept-Language is lowercased on the way in; the script subtag is
		// conventionally title case. The two have to meet.
		{"a lowercased script subtag still matches", "ja-latn",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		// A real English title beats a romanization for every language, not
		// just for English — a translation someone made is a better answer
		// than a transliteration. Checking the romanization first made French
		// (and every other language) prefer "Fate/stay night: Unlimited Blade
		// Works" to the actual English title.
		{"a french request prefers the english title to the romanization", "fr",
			mk("Fate/stay night [UBW]", map[string]string{
				"en": "Unlimited Blade Works", "ja-Latn": "Fate/stay night: Unlimited Blade Works"}),
			"Unlimited Blade Works"},
		{"and takes the romanization when there is no english title", "fr",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		// A Japanese reader still gets Japanese even when an English title
		// exists, which is what the original is for.
		{"a japanese request prefers the original to an english title", "ja",
			mk("鬼滅の刃", map[string]string{"en": "Demon Slayer", "ja-Latn": "Kimetsu no Yaiba"}), "鬼滅の刃"},
		// Not every original here is Japanese, and a Korean title romanized as
		// ko-Latn must not be served to a Korean reader in place of Hangul.
		{"korean gets hangul, everyone else its romanization", "ko",
			mk("메탈카드봇W", map[string]string{"ko-Latn": "Metal Cardbot W"}), "메탈카드봇W"},
		{"a japanese reader gets the korean romanization, not hangul", "ja",
			mk("메탈카드봇W", map[string]string{"ko-Latn": "Metal Cardbot W"}), "Metal Cardbot W"},
		// With no romanization to name the original's language, the language
		// itself decides: Japanese is written in a native script, French is not.
		{"japanese still reaches the original when nothing names its language", "ja",
			mk("フェイト", map[string]string{"en": "Fate"}), "フェイト"},
		{"french takes the english title in the same situation", "fr",
			mk("フェイト", map[string]string{"en": "Fate"}), "Fate"},
		// The same fallback has to hold for the other scripts this catalogue
		// keeps originals in, or a reader in one of them gets an English title
		// where the native one exists.
		{"korean reaches the original with nothing to name its language", "ko",
			mk("메탈카드봇W", map[string]string{"en": "Metal Cardbot W"}), "메탈카드봇W"},
		{"chinese reaches the original the same way", "zh",
			mk("喜羊羊与灰太狼", map[string]string{"en": "Pleasant Goat"}), "喜羊羊与灰太狼"},
		// A request that names the script is asking for the Latin form. It used
		// to fall back to the bare primary subtag and get native script — the
		// opposite of what it asked for.
		{"a request naming the script gets the romanization", "ja-latn",
			mk("鬼滅の刃", map[string]string{"ja": "鬼滅の刃", "ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		{"a script request with a region too", "ja-latn-jp",
			mk("鬼滅の刃", map[string]string{"ja": "鬼滅の刃", "ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		{"a script request for a language the title has no romanization in", "ko-latn",
			mk("鬼滅の刃", map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "Kimetsu no Yaiba"},
		// With no Latin form to offer, a script request must not be answered by
		// the bare language it was built from — that hands back the native
		// script it explicitly asked to avoid. An English title is the better
		// answer, and the original is the last resort.
		{"a script request with no romanization takes the english title", "ja-latn",
			mk("鬼滅の刃", map[string]string{"ja": "鬼滅の刃", "en": "Demon Slayer"}), "Demon Slayer"},
		{"and the original only when there is nothing else", "ja-latn",
			mk("鬼滅の刃", map[string]string{"ja": "鬼滅の刃"}), "鬼滅の刃"},
	}
	for _, tc := range tests {
		if got := resolveTitle(tc.in, tc.lang); got != tc.want {
			t.Errorf("%s: resolveTitle(_, %q) = %q, want %q", tc.name, tc.lang, got, tc.want)
		}
	}
}

// The cast an appearance reports is the character's plus its own, and a credit
// named in both must not be listed twice. Restating the constant cast on an
// appearance is redundant rather than wrong — it is exactly what the old
// replacing rule forced authors to do — so old-style data has to keep resolving
// to a sensible list.
func TestEffectiveCast(t *testing.T) {
	ja := model.VoiceActor{StaffID: "ayako-kawasumi", Language: "ja"}
	en := model.VoiceActor{StaffID: "kate-higgins", Language: "en"}
	// The same person cast in two languages is two credits, not one.
	dual := model.VoiceActor{StaffID: "ayako-kawasumi", Language: "en"}

	for _, tc := range []struct {
		name                 string
		throughout, specific []model.VoiceActor
		want                 []model.VoiceActor
	}{
		{"nothing specific keeps the constant cast", []model.VoiceActor{ja}, nil, []model.VoiceActor{ja}},
		{"the constant cast leads, the specific follows",
			[]model.VoiceActor{ja}, []model.VoiceActor{en}, []model.VoiceActor{ja, en}},
		{"a restated credit is not doubled",
			[]model.VoiceActor{ja}, []model.VoiceActor{ja, en}, []model.VoiceActor{ja, en}},
		{"a duplicate within one list is dropped too",
			[]model.VoiceActor{ja, ja}, []model.VoiceActor{en, en}, []model.VoiceActor{ja, en}},
		{"the same actor in another language is a separate credit",
			[]model.VoiceActor{ja}, []model.VoiceActor{dual}, []model.VoiceActor{ja, dual}},
		{"a character with no constant cast reports only the appearance's",
			nil, []model.VoiceActor{en}, []model.VoiceActor{en}},
		{"no cast anywhere", nil, nil, nil},
	} {
		got := effectiveCast(tc.throughout, tc.specific)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
				break
			}
		}
	}

	// The character's own slice must not be written through when an appearance
	// adds to it: every other appearance reads the same backing array.
	throughout := []model.VoiceActor{ja}
	if got := effectiveCast(throughout, []model.VoiceActor{en}); len(got) != 2 || len(throughout) != 1 {
		t.Errorf("merging appended into the character's own cast: %+v", throughout)
	}
}

// romanization is reached from resolveTitle only after the original has been
// ruled out, so its own same-language skip is easy to leave untested and let
// rot. It is what stops a Korean reader being handed ko-Latn in place of
// Hangul, so it is pinned here directly — including the shape that does reach
// it through resolveTitle, a title carrying a romanization and no original.
func TestRomanization(t *testing.T) {
	mk := func(tr map[string]string) model.Title { return model.Title{Translations: tr} }

	if got := romanization(mk(map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "en"); got != "Kimetsu no Yaiba" {
		t.Errorf("english request = %q, want the romanization", got)
	}
	if got := romanization(mk(map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "ja"); got != "" {
		t.Errorf("japanese request = %q, want no romanization of their own language", got)
	}
	if got := romanization(mk(map[string]string{"ja-Latn": "Kimetsu no Yaiba"}), "ja-JP"); got != "" {
		t.Errorf("ja-JP request = %q, want the primary subtag to match too", got)
	}
	// With two, the first in key order wins, so the same request always gets
	// the same answer.
	both := mk(map[string]string{"ko-Latn": "Metal Cardbot W", "ja-Latn": "Kimetsu no Yaiba"})
	if got := romanization(both, "en"); got != "Kimetsu no Yaiba" {
		t.Errorf("two romanizations = %q, want the first in key order", got)
	}
	if got := romanization(mk(map[string]string{"en": "Demon Slayer"}), "fr"); got != "" {
		t.Errorf("a translation is not a romanization: %q", got)
	}
	if got := romanization(mk(nil), "en"); got != "" {
		t.Errorf("empty title = %q", got)
	}

	// Through resolveTitle: a title with a romanization and no original is the
	// one shape where the skip decides the answer.
	noOriginal := mk(map[string]string{"ko-Latn": "Metal Cardbot W"})
	if got := resolveTitle(noOriginal, "ko"); got != "Metal Cardbot W" {
		// Nothing else to offer a Korean reader, so they get it anyway — but
		// only after the skip has declined to serve it as their own language.
		t.Errorf("korean request with no original = %q", got)
	}
	if got := resolveTitle(noOriginal, "en"); got != "Metal Cardbot W" {
		t.Errorf("english request with no original = %q", got)
	}
}
