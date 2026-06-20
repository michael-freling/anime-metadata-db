package build

import (
	"fmt"

	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
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

// BuildStaff fills staff names from Wikidata.
func (b *Builder) BuildStaff(o overrides.StaffOverride) (model.StaffRecord, *Report, error) {
	report := &Report{}
	rec := model.StaffRecord{Staff: o.Staff}
	for i := range rec.Staff {
		s := &rec.Staff[i]
		b.fillNames("staff "+s.ID, &s.Names, s.ExternalIDs.WikidataID, report)
	}
	report.Sort()
	return rec, report, nil
}

// fillNames merges Wikidata labels into a node's names per field (an authored
// name always wins): the Japanese label becomes original + ja, the English
// label becomes en.
func (b *Builder) fillNames(entity string, dst *model.Title, qid string, report *Report) {
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
	if dst.Original == "" && ja != "" {
		dst.Original = ja
	}
	addTranslation(dst, "ja", ja)
	addTranslation(dst, "en", en)
	if dst.Original == "" {
		report.add(entity, "names", fmt.Sprintf("Wikidata %s has no Japanese label; original left empty", qid))
	}
}

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
