package build

import (
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/wikidata"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// A series reaches Wikidata by resolving its own native title, not by an
// authored id — it has no AniList id to join on. The resolved work fills the
// English title and is recorded on the node, the way fillExternalIDs records
// the ids it cross-maps.
func TestFillSeriesTitleFromResolvedTitle(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q98642652":{"id":"Q98642652",
	  "labels":{"en":{"language":"en","value":"Frieren"}},
	  "titles":{"en":"Frieren: Beyond Journey's End"}}},
	  "resolvedByTitle":{"葬送のフリーレン":"Q98642652"}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID: "sousou-no-frieren",
		Titles: model.Title{
			Original:     "葬送のフリーレン",
			Translations: map[string]string{"ja-Latn": "Sousou no Frieren"},
		},
	}
	b.fillSeriesTitle(s, &Report{})

	if got := s.Titles.Translations["en"]; got != "Frieren: Beyond Journey's End" {
		t.Errorf("en = %q", got)
	}
	if got := s.ExternalIDs.WikidataID; got != "Q98642652" {
		t.Errorf("the resolved work must be recorded on the node, got %q", got)
	}
}

// An authored id is how a wrong or unreachable resolution gets corrected, so it
// has to beat the resolution rather than merely fill in for it.
func TestFillSeriesTitleAuthoredIDBeatsResolution(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{
	  "Q1":{"id":"Q1","titles":{"en":"The Authored Work"}},
	  "Q2":{"id":"Q2","titles":{"en":"The Resolved Work"}}},
	  "resolvedByTitle":{"日本語":"Q2"}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "x",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q1"},
		Titles: model.Title{
			Original:     "日本語",
			Translations: map[string]string{"ja-Latn": "Nihongo"},
		},
	}
	b.fillSeriesTitle(s, &Report{})

	if got := s.Titles.Translations["en"]; got != "The Authored Work" {
		t.Errorf("the authored id must win, got %q", got)
	}
}

// A title that resolved to nothing usable — no article, or a disambiguation
// page — leaves the series exactly as it was, with no en and no note. The
// tally of what failed and why belongs to `builder init`, which has the reason;
// one identical line per unresolved series here would bury the notes that say
// something.
func TestFillSeriesTitleUnresolvedTitle(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{},"resolvedByTitle":{"別のもの":"Q9"}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{ID: "mao", Titles: model.Title{Original: "マオ"}}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if _, ok := s.Titles.Translations["en"]; ok {
		t.Error("an unresolved title must not fill en")
	}
	if s.ExternalIDs.WikidataID != "" {
		t.Errorf("nothing resolved, so nothing to record: %q", s.ExternalIDs.WikidataID)
	}
	if !report.Empty() {
		t.Errorf("an unresolved title is not a low-confidence guess: %v", report.Notes)
	}
}

// wikidataFrom builds an Entities index from a wbgetentities-shaped document,
// so these tests exercise the same parse path a cached fetch goes through.
func wikidataFrom(t *testing.T, doc string) *wikidata.Entities {
	t.Helper()
	ents, err := wikidata.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return ents
}

// A series names a work in Wikidata; the P1476 title claim beats the label,
// which is a name for the item and follows the shortest-unambiguous-name
// convention. Frieren is the case that motivated this: its label is "Frieren".
func TestFillSeriesTitlePrefersTitleClaim(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q98642652":{"id":"Q98642652",
	  "labels":{"en":{"language":"en","value":"Frieren"}},
	  "titles":{"en":"Frieren: Beyond Journey's End"}}}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "sousou-no-frieren",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q98642652"},
		Titles: model.Title{
			Original:     "葬送のフリーレン",
			Translations: map[string]string{"ja-Latn": "Sousou no Frieren"},
		},
	}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if got := s.Titles.Translations["en"]; got != "Frieren: Beyond Journey's End" {
		t.Errorf("en = %q, want the P1476 title not the label", got)
	}
	// The romanization must survive: en and ja-Latn are different facts.
	if got := s.Titles.Translations["ja-Latn"]; got != "Sousou no Frieren" {
		t.Errorf("romanization clobbered: %q", got)
	}
	// A title claim is an assertion about the work, not a guess worth a note.
	if !report.Empty() {
		t.Errorf("P1476 fill should be silent, got %v", report.Notes)
	}
}

// Only a quarter of the works carry P1476, so the label is the fallback — and
// because it is the weaker source, the fill says so.
func TestFillSeriesTitleFallsBackToLabel(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q1":{"id":"Q1",
	  "labels":{"en":{"language":"en","value":"The Blue Wolves of Mibu"}}}}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "ao-no-miburo",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q1"},
		Titles: model.Title{
			Original:     "青のミブロ",
			Translations: map[string]string{"ja-Latn": "Ao no Miburo"},
		},
	}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if got := s.Titles.Translations["en"]; got != "The Blue Wolves of Mibu" {
		t.Errorf("en = %q", got)
	}
	if report.Empty() {
		t.Error("a label fill is lower confidence and must be reported")
	}
}

// The guard: a candidate that is the title we already hold, respelled, is not a
// translation. Filing it as `en` is the bug inferTitle exists to prevent, so it
// must not come back through a different source.
func TestFillSeriesTitleRefusesRomanization(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		titles    model.Title
	}{
		{
			// Wikidata spells the long vowel with a macron, we store "ou".
			name:      "macron variant of the romanization",
			candidate: "Hyakki Yakōshō",
			titles: model.Title{
				Original:     "百鬼夜行抄",
				Translations: map[string]string{"ja-Latn": "Hyakki Yakou Shou"},
			},
		},
		{
			// Written in Latin script in Japan too, so nobody translated it.
			name:      "Latin-script original repeated by the label",
			candidate: "Fate/stay night",
			titles:    model.Title{Original: "Fate/stay night"},
		},
		{
			name:      "original differing only in spacing and case",
			candidate: "Kirio Fan Club",
			titles:    model.Title{Original: "Kirio Fanclub"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ents := wikidataFrom(t, `{"entities":{"Q1":{"id":"Q1",
			  "labels":{"en":{"language":"en","value":"`+tc.candidate+`"}}}}}`)
			b := New(Sources{Wikidata: ents})
			s := &model.Series{ID: "x", ExternalIDs: model.ExternalIDs{WikidataID: "Q1"}, Titles: tc.titles}
			report := &Report{}
			b.fillSeriesTitle(s, report)

			if got, ok := s.Titles.Translations["en"]; ok {
				t.Errorf("filed a romanization as English: %q", got)
			}
			if report.Empty() {
				t.Error("a refusal must be reported so a human can author the title")
			}
		})
	}
}

// The guard compares a candidate against a Latin-script form the title already
// holds. A native-script original with no romanization gives it nothing to
// compare, and "Ao no Miburo" is then indistinguishable from "The Blue Wolves
// of Mibu" — so nothing is filled, rather than a romanization being asserted as
// a translation on no evidence.
func TestFillSeriesTitleWithNothingToCheckAgainst(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q1":{"id":"Q1",
	  "labels":{"en":{"language":"en","value":"Ao no Miburo"}}}}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "ao-no-miburo",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q1"},
		Titles:      model.Title{Original: "青のミブロ"}, // no romanization authored yet
	}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if got, ok := s.Titles.Translations["en"]; ok {
		t.Errorf("filled an unverifiable candidate as English: %q", got)
	}
	if !strings.Contains(report.String(), "no romanization to check it against") {
		t.Errorf("the refusal must say why it could not check: %v", report.Notes)
	}
}

// The same title with a romanization present is checkable again, and a real
// translation goes in. Without this the fix above would read as "never fill a
// native-script series", which is not what it does.
func TestFillSeriesTitleCheckableOnceRomanized(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q1":{"id":"Q1",
	  "labels":{"en":{"language":"en","value":"The Blue Wolves of Mibu"}}}}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "ao-no-miburo",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q1"},
		Titles: model.Title{
			Original:     "青のミブロ",
			Translations: map[string]string{"ja-Latn": "Ao no Miburo"},
		},
	}
	b.fillSeriesTitle(s, &Report{})

	if got := s.Titles.Translations["en"]; got != "The Blue Wolves of Mibu" {
		t.Errorf("en = %q", got)
	}
}

// An authored title is the author saying what the English title is, and no
// source overrides it. Without this a rebuild would be non-idempotent for any
// series whose Wikidata entry disagrees with the override.
func TestFillSeriesTitleAuthoredWins(t *testing.T) {
	ents := wikidataFrom(t, `{"entities":{"Q1":{"id":"Q1",
	  "labels":{"en":{"language":"en","value":"Demon Slayer"}},
	  "titles":{"en":"Demon Slayer: Kimetsu no Yaiba"}}}}`)
	b := New(Sources{Wikidata: ents})
	s := &model.Series{
		ID:          "demon-slayer",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q1"},
		Titles: model.Title{
			Original:     "鬼滅の刃",
			Translations: map[string]string{"en": "Demon Slayer", "ja-Latn": "Kimetsu no Yaiba"},
		},
	}
	b.fillSeriesTitle(s, &Report{})

	if got := s.Titles.Translations["en"]; got != "Demon Slayer" {
		t.Errorf("authored en overwritten: %q", got)
	}
}

// A series with no wikidataId keeps exactly today's behaviour — no en, and no
// note, because an unconfigured id is not a decision the builder made.
func TestFillSeriesTitleWithoutQID(t *testing.T) {
	b := New(Sources{Wikidata: wikidataFrom(t, `{"entities":{}}`)})
	s := &model.Series{
		ID:     "kami-no-shizuku",
		Titles: model.Title{Original: "神の雫", Translations: map[string]string{"ja-Latn": "Kami no Shizuku"}},
	}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if _, ok := s.Titles.Translations["en"]; ok {
		t.Error("no qid must mean no en")
	}
	if !report.Empty() {
		t.Errorf("an unconfigured id is not a low-confidence guess: %v", report.Notes)
	}
}

// A QID the cache has not resolved is a real problem — the author named a work
// and the build could not read it — so unlike a missing id it is reported.
func TestFillSeriesTitleUncachedQIDReports(t *testing.T) {
	b := New(Sources{Wikidata: wikidataFrom(t, `{"entities":{}}`)})
	s := &model.Series{ID: "x", ExternalIDs: model.ExternalIDs{WikidataID: "Q404"}}
	report := &Report{}
	b.fillSeriesTitle(s, report)

	if report.Empty() || !strings.Contains(report.String(), "Q404") {
		t.Errorf("expected a note naming the uncached qid, got %v", report.Notes)
	}
}

// The build must run with no Wikidata source at all (an unconfigured source, or
// a test fixture that supplies none) rather than panic on a nil index.
func TestFillSeriesTitleNoSource(t *testing.T) {
	b := New(Sources{})
	s := &model.Series{ID: "x", ExternalIDs: model.ExternalIDs{WikidataID: "Q1"}}
	b.fillSeriesTitle(s, &Report{})

	if _, ok := s.Titles.Translations["en"]; ok {
		t.Error("no source must mean no fill")
	}
}

// The folding table, drawn from the cases the catalogue actually produced when
// every series was resolved against Wikidata.
func TestIsRomanizationOf(t *testing.T) {
	tests := []struct {
		candidate string
		titles    model.Title
		want      bool
	}{
		// Refused: the same string, respelled.
		{"Hyakki Yakōshō", mkTitle("百鬼夜行抄", "ja-Latn", "Hyakki Yakou Shou"), true},
		{"Ichijōma Mankitsu Gurashi!", mkTitle("一条摩満喫暮らし", "ja-Latn", "Ichijouma Mankitsugurashi!"), true},
		{"Sōsō no Frieren", mkTitle("葬送のフリーレン", "ja-Latn", "Sousou no Frieren"), true},
		{"Golden Kamuy", mkTitle("Golden Kamuy"), true},
		{"Jujutsu Kaisen", mkTitle("呪術廻戦", "ja-Latn", "Jujutsu Kaisen"), true},
		// A Korean work romanizes under ko-Latn, and the guard must read it.
		{"Metal Cardbot W", mkTitle("메탈카드봇W", "ko-Latn", "Metal Cardbot W"), true},

		// Allowed: genuine translations, including ones sharing a proper noun
		// with the romanization.
		{"Frieren: Beyond Journey's End", mkTitle("葬送のフリーレン", "ja-Latn", "Sousou no Frieren"), false},
		{"The Blue Wolves of Mibu", mkTitle("青のミブロ", "ja-Latn", "Ao no Miburo"), false},
		{"Fire Force", mkTitle("炎炎ノ消防隊", "ja-Latn", "Enen no Shouboutai"), false},
		{"Demon Slayer", mkTitle("鬼滅の刃", "ja-Latn", "Kimetsu no Yaiba"), false},
		// A native-script original is not a spelling of anything Latin, so it
		// can never block a candidate.
		{"Anything At All", mkTitle("葬送のフリーレン"), false},
	}
	for _, tc := range tests {
		if _, got := isRomanizationOf(tc.candidate, tc.titles); got != tc.want {
			t.Errorf("isRomanizationOf(%q, %+v) = %v, want %v", tc.candidate, tc.titles, got, tc.want)
		}
	}
}

// mkTitle builds a Title from an original plus alternating code/value pairs.
func mkTitle(original string, kv ...string) model.Title {
	t := model.Title{Original: original}
	if len(kv) > 0 {
		t.Translations = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			t.Translations[kv[i]] = kv[i+1]
		}
	}
	return t
}
