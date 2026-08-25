package build

import (
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/wikidata"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

const charsWikidata = `{"entities":{
  "Q85805158":{"id":"Q85805158","labels":{"en":{"language":"en","value":"Tanjirō Kamado"},"ja":{"language":"ja","value":"竈門炭治郎"}}},
  "Q2596113":{"id":"Q2596113","labels":{"en":{"language":"en","value":"Natsuki Hanae"},"ja":{"language":"ja","value":"花江夏樹"}}},
  "Q3":{"id":"Q3","labels":{"en":{"language":"en","value":"Only English"}}},
  "Q4":{"id":"Q4","labels":{"en":{"language":"en","value":"Zach Aguilar"},"ja":{"language":"ja","value":"ザック・アギラール"}}},
  "Q5":{"id":"Q5","labels":{"ja":{"language":"ja","value":"ザック・アギラール"}}},
  "Q6":{"id":"Q6","labels":{"en":{"language":"en","value":"Aimi"},"ja":{"language":"ja","value":"アイミ"}}},
  "Q7":{"id":"Q7","labels":{"ja":{"language":"ja","value":"アイミ"}}}
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
	b.fillNames(characterName, "character a", authored, "Q85805158", &Report{})
	if authored.Translations["en"] != "Tanjiro" || authored.Translations["ja"] != "竈門炭治郎" {
		t.Errorf("merge wrong: %+v", authored.Translations)
	}

	// Missing QID -> note, no fill.
	r := &Report{}
	var title model.Title
	b.fillNames(characterName, "character x", &title, "", r)
	if r.Empty() {
		t.Error("expected a note for missing wikidataId")
	}

	// QID not in cache -> note.
	r = &Report{}
	b.fillNames(characterName, "character y", &title, "Q999999", r)
	if r.Empty() {
		t.Error("expected a note for qid not in cache")
	}

	// No Japanese label: the English one is the only name there is, so it
	// becomes the original rather than leaving the record with none. This used
	// to report "original left empty" for every English-language name in the
	// dataset, which was noise about the commonest correct case.
	r = &Report{}
	var t3 model.Title
	b.fillNames(characterName, "character z", &t3, "Q3", r)
	if t3.Translations["en"] != "Only English" || t3.Original != "Only English" {
		t.Errorf("unexpected fill: %+v", t3)
	}
	if !r.Empty() {
		t.Errorf("an English-only name is not worth reporting: %v", r.String())
	}

	// Nil Wikidata source -> no-op, no panic.
	var t4 model.Title
	New(Sources{}).fillNames(characterName, "character w", &t4, "Q1", &Report{})
	if !t4.IsZero() {
		t.Error("nil source should not fill names")
	}
}

// Which label is the name itself, rather than a rendering of it, differs
// between a person and a character — and getting it wrong recorded 30 of the
// 51 English voice actors under a katakana spelling of a name written in Latin
// script.
func TestFillNamesPicksTheNameOverItsRendering(t *testing.T) {
	b := New(wdSources(t))

	// A person whose Japanese label is katakana has a foreign name: the
	// katakana is how Japanese writes it, not what it is.
	var foreign model.Title
	b.fillNames(personName, "staff a", &foreign, "Q4", &Report{})
	if foreign.Original != "Zach Aguilar" {
		t.Errorf("original = %q, want the name rather than its katakana", foreign.Original)
	}
	if foreign.Translations["ja"] != "ザック・アギラール" {
		t.Errorf("the katakana is still how a Japanese reader sees it: %+v", foreign.Translations)
	}

	// A person whose Japanese label carries kanji has a Japanese name.
	var japanese model.Title
	b.fillNames(personName, "staff b", &japanese, "Q2596113", &Report{})
	if japanese.Original != "花江夏樹" {
		t.Errorf("original = %q, want the kanji", japanese.Original)
	}

	// A character keeps its Japanese name whatever script it is in: セイバー is
	// what the character is called in the work, not a rendering of "Saber".
	var character model.Title
	b.fillNames(characterName, "character c", &character, "Q4", &Report{})
	if character.Original != "ザック・アギラール" {
		t.Errorf("a character's original = %q, want the Japanese label as written", character.Original)
	}

	// The shape most of the English cast has: an English label and nothing
	// else. The name is the only one there is, and it needs no report.
	r := &Report{}
	var englishOnly model.Title
	b.fillNames(personName, "staff c", &englishOnly, "Q3", r)
	if englishOnly.Original != "Only English" {
		t.Errorf("original = %q, want the English label", englishOnly.Original)
	}
	if !r.Empty() {
		t.Errorf("an English-only name is the common correct case: %v", r.String())
	}

	// Katakana with no English label to prefer: the rendering stands in for a
	// name we do not otherwise have, and says so.
	r = &Report{}
	var noEnglish model.Title
	b.fillNames(personName, "staff d", &noEnglish, "Q5", r)
	if noEnglish.Original != "ザック・アギラール" {
		t.Errorf("original = %q, want the katakana as the only name available", noEnglish.Original)
	}
	if r.Empty() {
		t.Error("a katakana original with nothing behind it should be reported")
	}
}

// A Japanese stage name written in katakana is the case the script rule gets
// wrong, so it has to be reported rather than quietly resolved. It is told
// apart by the separator a rendered foreign name carries and a one-part stage
// name does not.
func TestFillNamesReportsAKatakanaNameThatMayNotBeForeign(t *testing.T) {
	b := New(wdSources(t))

	// アイミ: katakana, no separator — this may be a Japanese performer, and
	// the English label may be a romanization rather than her name.
	r := &Report{}
	var ambiguous model.Title
	b.fillNames(personName, "staff e", &ambiguous, "Q6", r)
	if ambiguous.Original != "Aimi" {
		t.Errorf("original = %q; the English label is still the better guess", ambiguous.Original)
	}
	if !strings.Contains(r.String(), "stage name") {
		t.Errorf("a katakana name with no separator should be reported: %v", r.String())
	}

	// ザック・アギラール carries the separator, so it is a foreign name rendered
	// for Japanese readers and there is nothing to report.
	quiet := &Report{}
	var foreign model.Title
	b.fillNames(personName, "staff f", &foreign, "Q4", quiet)
	if !quiet.Empty() {
		t.Errorf("a separated katakana name is unambiguous: %v", quiet.String())
	}

	// Katakana, no separator, and no English label either: both things are
	// worth saying, and they are said in one line. Two reports about one name
	// is noise dressed as thoroughness.
	both := &Report{}
	var neither model.Title
	b.fillNames(personName, "staff g", &neither, "Q7", both)
	if neither.Original != "アイミ" {
		t.Errorf("original = %q, want the katakana as the only name available", neither.Original)
	}
	if lines := strings.Count(both.String(), "\n"); lines != 1 {
		t.Errorf("want exactly one report line, got %d:\n%s", lines, both.String())
	}
	if !strings.Contains(both.String(), "no English label") || !strings.Contains(both.String(), "stage name") {
		t.Errorf("the one line should carry both reasons: %v", both.String())
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
