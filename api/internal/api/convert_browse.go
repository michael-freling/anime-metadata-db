package api

import (
	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
	browsev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/browse/v1"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// Converters for the browse API's own shapes.
//
// They live apart from convert.go so the split is visible in the file list: a
// shape here exists because anime-metadata-web wanted it, and may change with
// the site. Anything the public API also serves — a series, a character, an
// episode — is converted once in convert.go and reused, so the site and the
// public API cannot render the same node differently.

// toEntryKind maps the internal entry kind onto the wire enum.
func toEntryKind(k EntryKind) browsev1.EntryKind {
	if k == EntryFranchise {
		return browsev1.EntryKind_FRANCHISE
	}
	return browsev1.EntryKind_SERIES
}

// fromEntryKind maps the wire enum back onto the internal entry kind. It
// returns nil for UNSPECIFIED, which is the filter's "match both kinds".
func fromEntryKind(k browsev1.EntryKind) *EntryKind {
	var e EntryKind
	switch k {
	case browsev1.EntryKind_FRANCHISE:
		e = EntryFranchise
	case browsev1.EntryKind_SERIES:
		e = EntrySeries
	default:
		return nil
	}
	return &e
}

// toStaff converts one staff member, resolving their name via loc.
func toStaff(loc localizer, st *model.Staff) *browsev1.Staff {
	name, full := loc.title(st.Names)
	return &browsev1.Staff{
		Id:            st.ID,
		Name:          name,
		LocalizedName: full,
		ExternalIds:   toExternalIDs(st.ExternalIDs),
	}
}

// toStaffList converts a slice of staff, returning nil for an empty input.
func toStaffList(loc localizer, in []*model.Staff) []*browsev1.Staff {
	if len(in) == 0 {
		return nil
	}
	out := make([]*browsev1.Staff, len(in))
	for i, st := range in {
		out[i] = toStaff(loc, st)
	}
	return out
}

// toStaffCredits converts a staff member's roles, resolving each credited
// series' title so a page can name it without a call per credit.
func toStaffCredits(loc localizer, store *Store, in []StaffCredit) []*browsev1.StaffCredit {
	if len(in) == 0 {
		return nil
	}
	out := make([]*browsev1.StaffCredit, len(in))
	for i, c := range in {
		name, _ := loc.title(c.CharacterNames)
		out[i] = &browsev1.StaffCredit{
			CharacterId:   c.CharacterID,
			CharacterName: name,
			Language:      c.Language,
			SeriesIds:     c.SeriesIDs,
			SeriesTitles:  seriesTitles(loc, store, c.SeriesIDs),
		}
	}
	return out
}

// toFranchise converts one franchise, embedding each of its series whole and
// its watch orders. The cast is carried by each nested series.
func toFranchise(loc localizer, store *Store, f *model.Franchise) (*browsev1.Franchise, error) {
	title, full := loc.title(f.Titles)
	out := &browsev1.Franchise{Id: f.ID, Title: title, LocalizedTitle: full}
	if len(f.Series) > 0 {
		out.Series = make([]*animev1.Series, len(f.Series))
		for i := range f.Series {
			s, err := toSeries(loc, store, &f.Series[i])
			if err != nil {
				return nil, err
			}
			out.Series[i] = s
		}
	}
	if len(f.WatchOrders) > 0 {
		out.WatchOrders = make([]*browsev1.WatchOrder, len(f.WatchOrders))
		for i, wo := range f.WatchOrders {
			entries := make([]*browsev1.WatchOrderEntry, len(wo.Entries))
			for j, e := range wo.Entries {
				entries[j] = &browsev1.WatchOrderEntry{Ref: e.Ref, Note: e.Note}
			}
			out.WatchOrders[i] = &browsev1.WatchOrder{Name: wo.Name, Entries: entries}
		}
	}
	return out, nil
}

// toSearchResult converts a catalog entry to a search result, resolving its
// title via loc.
func toSearchResult(loc localizer, e CatalogEntry) *browsev1.SearchResult {
	title, full := loc.title(e.Titles)
	return &browsev1.SearchResult{
		Kind:           toEntryKind(e.Kind),
		Id:             e.ID,
		Title:          title,
		LocalizedTitle: full,
		FranchiseId:    e.FranchiseID,
	}
}

// toCatalogEntry converts a catalog entry to a browse row, resolving its title
// via loc and carrying the aggregates the row displays.
func toCatalogEntry(loc localizer, e CatalogEntry) *browsev1.CatalogEntry {
	title, full := loc.title(e.Titles)
	return &browsev1.CatalogEntry{
		Kind:              toEntryKind(e.Kind),
		Id:                e.ID,
		Title:             title,
		LocalizedTitle:    full,
		FranchiseId:       e.FranchiseID,
		FirstReleaseYear:  int32(e.FirstReleaseYear),
		LatestReleaseYear: int32(e.LatestReleaseYear),
		Works:             int32(e.Works),
		Episodes:          int32(e.Episodes),
	}
}
