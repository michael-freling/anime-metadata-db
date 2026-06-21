package build

import (
	"fmt"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// validate enforces schema and referential integrity on a resolved record. It
// aborts the build on the first violation (design Part 4, step 6).
func validate(rec model.Record) error {
	ids := newIDSet()

	if rec.Franchise != nil {
		if rec.Franchise.ID == "" {
			return fmt.Errorf("franchise has no id")
		}
		if err := ids.add("franchise", rec.Franchise.ID); err != nil {
			return err
		}
	}

	var verr error
	rec.EachSeries(func(s *model.Series) {
		if verr == nil {
			verr = validateSeries(s, ids)
		}
	})
	if verr != nil {
		return verr
	}

	if rec.Franchise != nil {
		for _, wo := range rec.Franchise.WatchOrders {
			if err := validateWatchOrder(rec.Franchise.ID, wo, ids); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateSeries checks one series and its nodes, registering every id and
// resolving alternateCutOf targets to seasons in the same series.
func validateSeries(s *model.Series, ids *idSet) error {
	if s.ID == "" {
		return fmt.Errorf("series has no id")
	}
	if err := ids.add("series", s.ID); err != nil {
		return err
	}

	seasonIDs := make(map[string]bool, len(s.Seasons))
	for i := range s.Seasons {
		sea := &s.Seasons[i]
		if sea.ID == "" {
			return fmt.Errorf("series %q: a season has no id", s.ID)
		}
		if err := ids.add("season", sea.ID); err != nil {
			return err
		}
		seasonIDs[sea.ID] = true
		if sea.Number < 1 {
			return fmt.Errorf("season %q: number must be >= 1, got %d", sea.ID, sea.Number)
		}
		if sea.ReleaseSeason != "" && !sea.ReleaseSeason.Valid() {
			return fmt.Errorf("season %q: invalid releaseSeason %q", sea.ID, sea.ReleaseSeason)
		}
	}

	for i := range s.Movies {
		mov := &s.Movies[i]
		if mov.ID == "" {
			return fmt.Errorf("series %q: a movie has no id", s.ID)
		}
		if err := ids.add("movie", mov.ID); err != nil {
			return err
		}
		if mov.AlternateCutOf != nil && !seasonIDs[mov.AlternateCutOf.SeasonID] {
			return fmt.Errorf("movie %q: alternateCutOf references unknown season %q",
				mov.ID, mov.AlternateCutOf.SeasonID)
		}
	}

	for i := range s.Specials {
		sp := &s.Specials[i]
		if sp.ID == "" {
			return fmt.Errorf("series %q: a special has no id", s.ID)
		}
		if err := ids.add("special", sp.ID); err != nil {
			return err
		}
		if sp.Format != "" && !sp.Format.Valid() {
			return fmt.Errorf("special %q: invalid format %q", sp.ID, sp.Format)
		}
	}
	return nil
}

// validateWatchOrder checks every entry of a curated order points at a known id
// within the franchise and that the order is named and non-empty.
func validateWatchOrder(franchiseID string, wo model.WatchOrder, ids *idSet) error {
	if wo.Name == "" {
		return fmt.Errorf("franchise %q: a watchOrder has no name", franchiseID)
	}
	if len(wo.Entries) == 0 {
		return fmt.Errorf("franchise %q: watchOrder %q has no entries", franchiseID, wo.Name)
	}
	for _, e := range wo.Entries {
		if !ids.has(e.Ref) {
			return fmt.Errorf("franchise %q: watchOrder %q references unknown id %q",
				franchiseID, wo.Name, e.Ref)
		}
	}
	return nil
}

// idSet tracks every declared id to enforce global uniqueness and resolve refs.
type idSet struct {
	kind map[string]string
}

// newIDSet returns an empty id set.
func newIDSet() *idSet { return &idSet{kind: make(map[string]string)} }

// add registers an id, failing if it was already declared.
func (s *idSet) add(kind, id string) error {
	if prev, ok := s.kind[id]; ok {
		return fmt.Errorf("duplicate id %q (declared as %s and %s)", id, prev, kind)
	}
	s.kind[id] = kind
	return nil
}

// has reports whether an id was declared.
func (s *idSet) has(id string) bool {
	_, ok := s.kind[id]
	return ok
}
