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

func sampleChars() overrides.CharactersOverride {
	return overrides.CharactersOverride{
		Path:  "characters/demon-slayer.yaml",
		Staff: []model.Staff{{ID: "natsuki-hanae", ExternalIDs: model.ExternalIDs{WikidataID: "Q2596113"}}},
		Characters: []model.Character{{
			ID:          "tanjiro-kamado",
			ExternalIDs: model.ExternalIDs{WikidataID: "Q85805158"},
			VoiceActors: []model.VoiceActor{{StaffID: "natsuki-hanae", Language: "ja"}},
			Appearances: []model.CharacterAppearance{{
				SeriesID: "demon-slayer",
				Scope:    []model.ScopeRef{{SeasonID: "ds-s1"}},
			}},
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

func TestBuildCharacters(t *testing.T) {
	rec, _, err := New(wdSources(t)).BuildCharacters(sampleChars(), charCtx())
	if err != nil {
		t.Fatal(err)
	}
	c := rec.Characters[0]
	if c.Names.Original != "竈門炭治郎" || c.Names.Translations["en"] != "Tanjirō Kamado" || c.Names.Translations["ja"] != "竈門炭治郎" {
		t.Errorf("character names not filled: %+v", c.Names)
	}
	if rec.Staff[0].Names.Translations["en"] != "Natsuki Hanae" {
		t.Errorf("staff names not filled: %+v", rec.Staff[0].Names)
	}
}

func TestBuildCharactersAuthoredNameWins(t *testing.T) {
	o := sampleChars()
	o.Characters[0].Names = model.Title{Translations: map[string]string{"en": "Tanjiro"}}
	rec, _, err := New(wdSources(t)).BuildCharacters(o, charCtx())
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.Characters[0].Names.Translations["en"]; got != "Tanjiro" {
		t.Errorf("authored en should win, got %q", got)
	}
	if rec.Characters[0].Names.Translations["ja"] != "竈門炭治郎" {
		t.Errorf("ja should still be filled: %+v", rec.Characters[0].Names.Translations)
	}
}

func TestFillNames(t *testing.T) {
	b := New(wdSources(t))

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
	nb := New(Sources{})
	var t4 model.Title
	nb.fillNames("character w", &t4, "Q1", &Report{})
	if !t4.IsZero() {
		t.Error("nil source should not fill names")
	}
}

func TestValidateCharactersErrors(t *testing.T) {
	ctx := charCtx()
	good := sampleChars()

	tests := []struct {
		name   string
		mutate func(*overrides.CharactersOverride)
	}{
		{"no appearances", func(o *overrides.CharactersOverride) { o.Characters[0].Appearances = nil }},
		{"unknown series", func(o *overrides.CharactersOverride) { o.Characters[0].Appearances[0].SeriesID = "ghost" }},
		{"unknown scope season", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{SeasonID: "ghost"}}
		}},
		{"unknown scope movie", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{MovieID: "ghost"}}
		}},
		{"unknown scope special", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{SpecialID: "ghost"}}
		}},
		{"scope sets none", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{}}
		}},
		{"scope sets two", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{SeasonID: "ds-s1", MovieID: "ds-movie"}}
		}},
		{"unknown default VA staff", func(o *overrides.CharactersOverride) {
			o.Characters[0].VoiceActors = []model.VoiceActor{{StaffID: "ghost", Language: "ja"}}
		}},
		{"unknown appearance VA staff", func(o *overrides.CharactersOverride) {
			o.Characters[0].Appearances[0].VoiceActors = []model.VoiceActor{{StaffID: "ghost", Language: "ja"}}
		}},
		{"VA no staffId", func(o *overrides.CharactersOverride) {
			o.Characters[0].VoiceActors = []model.VoiceActor{{Language: "ja"}}
		}},
		{"VA no language", func(o *overrides.CharactersOverride) {
			o.Characters[0].VoiceActors = []model.VoiceActor{{StaffID: "natsuki-hanae"}}
		}},
		{"character no id", func(o *overrides.CharactersOverride) { o.Characters[0].ID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := good // copy of the struct; slices are shared but mutated fields are replaced
			o.Characters = append([]model.Character(nil), good.Characters...)
			o.Characters[0].Appearances = append([]model.CharacterAppearance(nil), good.Characters[0].Appearances...)
			tt.mutate(&o)
			if _, _, err := New(wdSources(t)).BuildCharacters(o, ctx); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestValidateCharactersOKWithScopes(t *testing.T) {
	o := sampleChars()
	o.Characters[0].Appearances[0].Scope = []model.ScopeRef{{MovieID: "ds-movie"}, {SpecialID: "ds-ova"}}
	o.Characters[0].Appearances[0].VoiceActors = []model.VoiceActor{{StaffID: "natsuki-hanae", Language: "en"}}
	if _, _, err := New(wdSources(t)).BuildCharacters(o, charCtx()); err != nil {
		t.Errorf("expected valid, got %v", err)
	}
}
