package build

import (
	"fmt"
	"strings"

	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// IDIndex holds the resolved R1 node ids by kind, used to validate the R2
// appearance graph (a character's seriesId/scope must reference real nodes).
type IDIndex struct {
	Series  map[string]bool
	Season  map[string]bool
	Movie   map[string]bool
	Special map[string]bool
}

// NewIDIndex returns an empty index.
func NewIDIndex() IDIndex {
	return IDIndex{
		Series:  map[string]bool{},
		Season:  map[string]bool{},
		Movie:   map[string]bool{},
		Special: map[string]bool{},
	}
}

// Collect adds a resolved R1 record's ids to the index.
func (x IDIndex) Collect(rec model.Record) {
	rec.EachSeries(func(s *model.Series) {
		x.Series[s.ID] = true
		for i := range s.Seasons {
			x.Season[s.Seasons[i].ID] = true
		}
		for i := range s.Movies {
			x.Movie[s.Movies[i].ID] = true
		}
		for i := range s.Specials {
			x.Special[s.Specials[i].ID] = true
		}
	})
}

// CharacterContext carries the cross-file id universes the character graph is
// validated against: the R1 node ids and every declared staff id.
type CharacterContext struct {
	R1    IDIndex
	Staff map[string]bool
}

// homeSeries returns the id of the standalone series a record nests its cast
// under, or "" for a franchise (which has several series, so there is no single
// default).
func homeSeries(rec model.Record) string {
	if rec.Series != nil {
		return rec.Series.ID
	}
	return ""
}

// defaultAppearances fills in the enclosing series for a character nested under
// a standalone series: a character with no appearances gets one in its home
// series, and an appearance that omits seriesId defaults to it. A character
// under a franchise must name its series explicitly (home is "").
func defaultAppearances(c *model.Character, home string) {
	if home == "" {
		return
	}
	if len(c.Appearances) == 0 {
		c.Appearances = []model.CharacterAppearance{{SeriesID: home}}
		return
	}
	for i := range c.Appearances {
		if c.Appearances[i].SeriesID == "" {
			c.Appearances[i].SeriesID = home
		}
	}
}

// BuildStaff fills staff names from Wikidata.
func (b *Builder) BuildStaff(o overrides.StaffOverride) (model.StaffRecord, *Report, error) {
	report := &Report{}
	rec := model.StaffRecord{Staff: o.Staff}
	for i := range rec.Staff {
		s := &rec.Staff[i]
		b.fillNames(personName, "staff "+s.ID, &s.Names, s.ExternalIDs.WikidataID, report)
	}
	report.Sort()
	return rec, report, nil
}

// fillNames merges Wikidata labels into a node's names per field (an authored
// name always wins): the Japanese label becomes ja, the English label becomes
// en, and whichever of the two is the name as its owner writes it becomes the
// original.
//
// That last part is the whole difficulty. For 竈門炭治郎 the Japanese label is
// the name and the English one is a transliteration of it; for Zach Aguilar it
// is the other way round, and ザック・アギラール is a rendering of his name for
// Japanese readers rather than his name. Filing the Japanese label as the
// original regardless — which this did — recorded 30 of the 51 English voice
// actors under a katakana spelling of a name written in Latin script.
//
// The script decides, for a real person: a Japanese label containing kanji or
// hiragana is a Japanese name, and one written only in katakana is a foreign
// name transliterated. The case that breaks the rule is a Japanese stage name
// written in katakana, so that one is reported — see nameSeparator for how it
// is told apart. No Japanese staff member has a katakana-only label today.
//
// A *character* is different, and the same rule applied to one would be wrong:
// セイバー is not a rendering of "Saber" for Japanese readers, it is the name
// the character has in the work, and English-language releases render it back
// the other way. So a character keeps the Japanese label as its original
// whatever script it is written in.
func (b *Builder) fillNames(kind nameKind, entity string, dst *model.Title, qid string, report *Report) {
	if qid == "" {
		report.add(entity, "names", "no externalIds.wikidataId; names not auto-filled")
		return
	}
	if b.sources.Wikidata == nil {
		return
	}
	ent, ok := b.sources.Wikidata.Lookup(qid)
	if !ok {
		report.add(entity, "names", fmt.Sprintf("Wikidata %s not in cache; names not filled (run `builder init`)", qid))
		return
	}
	ja, en := ent.Labels["ja"], ent.Labels["en"]
	addTranslation(dst, "ja", ja)
	addTranslation(dst, "en", en)
	if dst.Original == "" {
		dst.Original = kind.original(ja, en, entity, report)
	}
	if dst.Original == "" {
		report.add(entity, "names", fmt.Sprintf("Wikidata %s has neither a Japanese nor an English label; original left empty", qid))
	}
}

// nameKind says whose name is being filled, because a character and a person
// answer "which label is the name itself?" differently.
type nameKind int

const (
	// characterName is a name as it appears in the work. The Japanese label is
	// it, katakana included.
	characterName nameKind = iota
	// personName is a real person's name, which belongs to whichever language
	// they write it in.
	personName
)

// original picks the label that is the name itself rather than a rendering of
// it, and reports the reasoning where it had to choose.
func (k nameKind) original(ja, en, entity string, report *Report) string {
	if k == characterName {
		if ja != "" {
			return ja
		}
		return en
	}
	switch {
	case ja == "":
		// Nothing Japanese to prefer, so the English label is all there is —
		// and for a name written in Latin script that is the right answer
		// anyway, which is why this no longer reports an empty original.
		return en
	case model.IsKatakanaOnly(ja):
		// Katakana spells a foreign name — and it also spells a Japanese stage
		// name, which is the case this gets wrong. A rendering of a foreign
		// name separates its parts with "・" (ザック・アギラール); a one-part
		// Japanese stage name has no reason to carry one, so its absence is
		// what gets reported.
		//
		// A multi-part Japanese name written in katakana with a separator is
		// therefore taken for a foreign one in silence. It is the residual
		// risk of the whole rule, and reporting every katakana label instead
		// would mean 30 lines a build about the case that is nearly always
		// right — which is how a report stops being read. Authoring the name
		// in the override is the answer when it happens.
		//
		// At most one line either way: two reports about one name is noise
		// dressed as thoroughness.
		switch {
		case en == "":
			report.add(entity, "names", fmt.Sprintf(
				"kept the katakana %q as the original because there is no English label; it is how Japanese writes the name rather than the name itself%s",
				ja, suffixIfUnseparated(ja)))
			return ja
		case !strings.Contains(ja, nameSeparator):
			report.add(entity, "names", fmt.Sprintf(
				"used the English label as the original because the Japanese one, %q, is katakana — but it carries no %q separating a given name from a family one, so it may be a Japanese stage name rather than a foreign name written for Japanese readers",
				ja, nameSeparator))
		}
		return en
	default:
		return ja
	}
}

// suffixIfUnseparated adds the stage-name caveat to a report that is already
// being written, so one name never produces two lines.
func suffixIfUnseparated(ja string) string {
	if strings.Contains(ja, nameSeparator) {
		return ""
	}
	return fmt.Sprintf(", and with no %q in it, it may be a Japanese stage name rather than a foreign name", nameSeparator)
}

// nameSeparator is the interpunct Japanese puts between the parts of a foreign
// name written in katakana: ザック・アギラール. A Japanese name, written in kanji,
// needs no separator, so its presence is evidence the name is not Japanese.
const nameSeparator = "・"

// addTranslation sets a translation only when the value is non-empty and the
// override did not already provide that language.
func addTranslation(dst *model.Title, code, val string) {
	if val == "" {
		return
	}
	if _, ok := dst.Translations[code]; ok {
		return
	}
	if dst.Translations == nil {
		dst.Translations = make(map[string]string)
	}
	dst.Translations[code] = val
}

// ValidateCharacters enforces referential integrity of a record's cast: every
// appearance resolves to a known R1 series/scope node and every voice actor to
// a declared staff id. It runs after all R1 ids are known.
func ValidateCharacters(characters []model.Character, ctx CharacterContext) error {
	for i := range characters {
		c := &characters[i]
		if c.ID == "" {
			return fmt.Errorf("a character has no id")
		}
		if len(c.Appearances) == 0 {
			return fmt.Errorf("character %q: has no appearances", c.ID)
		}
		for _, va := range c.VoiceActors {
			if err := validateVoiceActor(c.ID, va, ctx); err != nil {
				return err
			}
		}
		for _, ap := range c.Appearances {
			if ap.SeriesID == "" {
				return fmt.Errorf("character %q: appearance has no seriesId (only a character nested under a standalone series defaults it)", c.ID)
			}
			if !ctx.R1.Series[ap.SeriesID] {
				return fmt.Errorf("character %q: appearance references unknown series %q", c.ID, ap.SeriesID)
			}
			for _, sc := range ap.Scope {
				if err := validateScope(c.ID, sc, ctx.R1); err != nil {
					return err
				}
			}
			for _, va := range ap.VoiceActors {
				if err := validateVoiceActor(c.ID, va, ctx); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateVoiceActor checks a voice-actor link resolves to a known staff id.
func validateVoiceActor(charID string, va model.VoiceActor, ctx CharacterContext) error {
	if va.StaffID == "" {
		return fmt.Errorf("character %q: a voiceActor has no staffId", charID)
	}
	if !ctx.Staff[va.StaffID] {
		return fmt.Errorf("character %q: voiceActor references unknown staff %q", charID, va.StaffID)
	}
	if va.Language == "" {
		return fmt.Errorf("character %q: voiceActor %q has no language", charID, va.StaffID)
	}
	return nil
}

// validateScope checks a scope ref sets exactly one node id and that it exists.
func validateScope(charID string, sc model.ScopeRef, idx IDIndex) error {
	set := 0
	if sc.SeasonID != "" {
		set++
		if !idx.Season[sc.SeasonID] {
			return fmt.Errorf("character %q: scope references unknown season %q", charID, sc.SeasonID)
		}
	}
	if sc.MovieID != "" {
		set++
		if !idx.Movie[sc.MovieID] {
			return fmt.Errorf("character %q: scope references unknown movie %q", charID, sc.MovieID)
		}
	}
	if sc.SpecialID != "" {
		set++
		if !idx.Special[sc.SpecialID] {
			return fmt.Errorf("character %q: scope references unknown special %q", charID, sc.SpecialID)
		}
	}
	if set != 1 {
		return fmt.Errorf("character %q: each scope entry must set exactly one of seasonId/movieId/specialId", charID)
	}
	return nil
}
