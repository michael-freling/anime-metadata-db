package build

import (
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/sources/wikidata"
)

const charsWikidata = `{"entities":{
  "Q85805158":{"id":"Q85805158","labels":{"en":{"language":"en","value":"Tanjirō Kamado"},"ja":{"language":"ja","value":"竈門炭治郎"}}},
  "Q2596113":{"id":"Q2596113","labels":{"en":{"language":"en","value":"Natsuki Hanae"},"ja":{"language":"ja","value":"花江夏樹"}}},
  "Q3":{"id":"Q3","labels":{"en":{"language":"en","value":"Only English"}}}
}}`

func wdSources(t *testing.T) Sources {
	t.Helper()
	wd, err := wikidata.Parse(strings.NewReader(charsWikidata))
	if err != nil {
		t.Fatal(err)
	}
	return Sources{Wikidata: wd}
}

func charCtx() CharacterContext {
	idx := NewIDIndex()
	idx.Series["demon-slayer"] = true
	idx.Season["ds-s1"] = true
	idx.Movie["ds-movie"] = true
	idx.Special["ds-ova"] = true
	return CharacterContext{R1: idx, Staff: map[string]bool{"natsuki-hanae": true}}
}

func sampleCharacter() model.Character {
	return model.Character{
		ID:          "tanjiro-kamado",
		ExternalIDs: model.ExternalIDs{WikidataID: "Q85805158"},
		VoiceActors: []model.VoiceActor{{StaffID: "natsuki-hanae", Language: "ja"}},
		Appearances: []model.CharacterAppearance{{
			SeriesID: "demon-slayer",
			Scope:    []model.ScopeRef{{SeasonID: "ds-s1"}},
		}},
	}
}

func TestIDIndexCollect(t *testing.T) {
	idx := NewIDIndex()
	idx.Collect(model.Record{Series: &model.Series{
		ID:       "demon-slayer",
		Seasons:  []model.Season{{ID: "ds-s1", Number: 1}},
		Movies:   []model.Movie{{ID: "ds-movie"}},
		Specials: []model.Special{{ID: "ds-ova"}},
	}})
	if !idx.Series["demon-slayer"] || !idx.Season["ds-s1"] || !idx.Movie["ds-movie"] || !idx.Special["ds-ova"] {
		t.Errorf("ids not collected: %+v", idx)
	}
}

func TestBuildStaff(t *testing.T) {
	o := overrides.StaffOverride{Staff: []model.Staff{
		{ID: "natsuki-hanae", ExternalIDs: model.ExternalIDs{WikidataID: "Q2596113"}},
	}}
	rec, _, err := New(wdSources(t)).BuildStaff(o)
	if err != nil {
		t.Fatal(err)
	}
	n := rec.Staff[0].Names
	if n.Original != "花江夏樹" || n.Translations["en"] != "Natsuki Hanae" || n.Translations["ja"] != "花江夏樹" {
		t.Errorf("staff names not filled: %+v", n)
	}
}

func TestFillNames(t *testing.T) {
	b := New(wdSources(t))

	// Authored name wins; ja is still filled.
	authored := &model.Title{Translations: map[string]string{"en": "Tanjiro"}}
	b.fillNames("character a", authored, "Q85805158", &Report{})
	if authored.Translations["en"] != "Tanjiro" || authored.Translations["ja"] != "竈門炭治郎" {
		t.Errorf("merge wrong: %+v", authored.Translations)
	}

	// Missing QID -> note, no fill.
	r := &Report{}
	var title model.Title
	b.fillNames("character x", &title, "", r)
	if r.Empty() {
		t.Error("expected a note for missing wikidataId")
	}

	// QID not in cache -> note.
	r = &Report{}
	b.fillNames("character y", &title, "Q999999", r)
	if r.Empty() {
		t.Error("expected a note for qid not in cache")
	}

	// Entity with no Japanese label -> en filled, original empty, note.
	r = &Report{}
	var t3 model.Title
	b.fillNames("character z", &t3, "Q3", r)
	if t3.Translations["en"] != "Only English" || t3.Original != "" {
		t.Errorf("unexpected fill: %+v", t3)
	}
	if r.Empty() {
		t.Error("expected a note for missing Japanese label")
	}

	// Nil Wikidata source -> no-op, no panic.
	var t4 model.Title
	New(Sources{}).fillNames("character w", &t4, "Q1", &Report{})
	if !t4.IsZero() {
		t.Error("nil source should not fill names")
	}
}

func TestDefaultAppearances(t *testing.T) {
	// Standalone series: no appearances -> one in the home series.
	c := model.Character{ID: "x"}
	defaultAppearances(&c, "demon-slayer")
	if len(c.Appearances) != 1 || c.Appearances[0].SeriesID != "demon-slayer" {
		t.Errorf("expected default appearance, got %+v", c.Appearances)
	}

	// Appearance that omits seriesId -> filled; scope preserved.
	c2 := model.Character{ID: "y", Appearances: []model.CharacterAppearance{{Scope: []model.ScopeRef{{SeasonID: "ds-s1"}}}}}
	defaultAppearances(&c2, "demon-slayer")
	if c2.Appearances[0].SeriesID != "demon-slayer" || len(c2.Appearances[0].Scope) != 1 {
		t.Errorf("seriesId not defaulted / scope lost: %+v", c2.Appearances[0])
	}

	// Explicit seriesId (another series) is kept.
	c3 := model.Character{ID: "z", Appearances: []model.CharacterAppearance{{SeriesID: "other"}}}
	defaultAppearances(&c3, "demon-slayer")
	if c3.Appearances[0].SeriesID != "other" {
		t.Errorf("explicit seriesId overwritten: %+v", c3.Appearances[0])
	}

	// Franchise (no home) -> no defaulting.
	c4 := model.Character{ID: "w"}
	defaultAppearances(&c4, "")
	if len(c4.Appearances) != 0 {
		t.Errorf("franchise should not default appearances: %+v", c4.Appearances)
	}
}

func TestValidateCharactersOK(t *testing.T) {
	c := sampleCharacter()
	c.Appearances[0].Scope = []model.ScopeRef{{MovieID: "ds-movie"}, {SpecialID: "ds-ova"}}
	c.Appearances[0].VoiceActors = []model.VoiceActor{{StaffID: "natsuki-hanae", Language: "en"}}
	if err := ValidateCharacters([]model.Character{c}, charCtx()); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}

func TestValidateCharactersErrors(t *testing.T) {
	ctx := charCtx()
	tests := []struct {
		name   string
		mutate func(*model.Character)
	}{
		{"no id", func(c *model.Character) { c.ID = "" }},
		{"no appearances", func(c *model.Character) { c.Appearances = nil }},
		{"empty seriesId", func(c *model.Character) { c.Appearances[0].SeriesID = "" }},
		{"unknown series", func(c *model.Character) { c.Appearances[0].SeriesID = "ghost" }},
		{"unknown scope season", func(c *model.Character) { c.Appearances[0].Scope = []model.ScopeRef{{SeasonID: "ghost"}} }},
		{"unknown scope movie", func(c *model.Character) { c.Appearances[0].Scope = []model.ScopeRef{{MovieID: "ghost"}} }},
		{"unknown scope special", func(c *model.Character) { c.Appearances[0].Scope = []model.ScopeRef{{SpecialID: "ghost"}} }},
		{"scope none", func(c *model.Character) { c.Appearances[0].Scope = []model.ScopeRef{{}} }},
		{"scope two", func(c *model.Character) {
			c.Appearances[0].Scope = []model.ScopeRef{{SeasonID: "ds-s1", MovieID: "ds-movie"}}
		}},
		{"unknown default VA", func(c *model.Character) { c.VoiceActors = []model.VoiceActor{{StaffID: "ghost", Language: "ja"}} }},
		{"unknown appearance VA", func(c *model.Character) {
			c.Appearances[0].VoiceActors = []model.VoiceActor{{StaffID: "ghost", Language: "ja"}}
		}},
		{"VA no staffId", func(c *model.Character) { c.VoiceActors = []model.VoiceActor{{Language: "ja"}} }},
		{"VA no language", func(c *model.Character) { c.VoiceActors = []model.VoiceActor{{StaffID: "natsuki-hanae"}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := sampleCharacter()
			c.VoiceActors = append([]model.VoiceActor(nil), c.VoiceActors...)
			c.Appearances = append([]model.CharacterAppearance(nil), c.Appearances...)
			tt.mutate(&c)
			if err := ValidateCharacters([]model.Character{c}, ctx); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
