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
	}
	for _, tc := range tests {
		if got := resolveTitle(tc.in, tc.lang); got != tc.want {
			t.Errorf("%s: resolveTitle(_, %q) = %q, want %q", tc.name, tc.lang, got, tc.want)
		}
	}
}
