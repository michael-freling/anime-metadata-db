package api

import (
	animev1 "github.com/michael-freling/anime-metadata-db/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// dateLayout is the canonical wire form for dates (matches model.Date).
const dateLayout = "2006-01-02"

// toTitle converts a model.Title, returning nil for a zero title so the field
// is omitted on the wire.
func toTitle(t model.Title) *animev1.Title {
	if t.IsZero() {
		return nil
	}
	return &animev1.Title{Original: t.Original, Translations: t.Translations}
}

// toExternalIDs converts cross-database ids, returning nil when none are set.
func toExternalIDs(e model.ExternalIDs) *animev1.ExternalIds {
	if e.IsZero() {
		return nil
	}
	return &animev1.ExternalIds{
		AnilistId:  int32(e.AnilistID),
		AnidbId:    int32(e.AnidbID),
		TmdbId:     int32(e.TmdbID),
		TvdbId:     int32(e.TvdbID),
		WikidataId: e.WikidataID,
	}
}

// toReleaseSeason maps a model release quarter to its proto enum.
func toReleaseSeason(s model.ReleaseSeason) animev1.ReleaseSeason {
	switch s {
	case model.SeasonWinter:
		return animev1.ReleaseSeason_RELEASE_SEASON_WINTER
	case model.SeasonSpring:
		return animev1.ReleaseSeason_RELEASE_SEASON_SPRING
	case model.SeasonSummer:
		return animev1.ReleaseSeason_RELEASE_SEASON_SUMMER
	case model.SeasonFall:
		return animev1.ReleaseSeason_RELEASE_SEASON_FALL
	default:
		return animev1.ReleaseSeason_RELEASE_SEASON_UNSPECIFIED
	}
}

// toSpecialFormat maps a model special format to its proto enum.
func toSpecialFormat(f model.SpecialFormat) animev1.SpecialFormat {
	switch f {
	case model.FormatOVA:
		return animev1.SpecialFormat_SPECIAL_FORMAT_OVA
	case model.FormatONA:
		return animev1.SpecialFormat_SPECIAL_FORMAT_ONA
	case model.FormatSpecial:
		return animev1.SpecialFormat_SPECIAL_FORMAT_SPECIAL
	default:
		return animev1.SpecialFormat_SPECIAL_FORMAT_UNSPECIFIED
	}
}

// toEntryKind maps a store catalog kind to its proto enum.
func toEntryKind(k EntryKind) animev1.EntryKind {
	if k == EntryFranchise {
		return animev1.EntryKind_ENTRY_KIND_FRANCHISE
	}
	return animev1.EntryKind_ENTRY_KIND_SERIES
}

// toInt32Ptr converts an optional int, preserving nil.
func toInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// toDate formats an optional model.Date as YYYY-MM-DD, returning "" for nil.
func toDate(d *model.Date) string {
	if d == nil {
		return ""
	}
	return d.Format(dateLayout)
}

// toEpisode converts one episode.
func toEpisode(e model.Episode) *animev1.Episode {
	return &animev1.Episode{
		AbsoluteNumber: toInt32Ptr(e.AbsoluteNumber),
		AiredNumber:    int32(e.AiredNumber),
		ReleaseDate:    toDate(e.ReleaseDate),
		Title:          e.Title,
	}
}

// toEpisodes converts a slice of episodes, returning nil for an empty input.
func toEpisodes(in []model.Episode) []*animev1.Episode {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.Episode, len(in))
	for i := range in {
		out[i] = toEpisode(in[i])
	}
	return out
}

// toSeason converts one season.
func toSeason(s model.Season) *animev1.Season {
	return &animev1.Season{
		Id:            s.ID,
		Titles:        toTitle(s.Titles),
		Number:        int32(s.Number),
		Part:          toInt32Ptr(s.Part),
		ReleaseDate:   toDate(s.ReleaseDate),
		ReleaseYear:   int32(s.ReleaseYear),
		ReleaseSeason: toReleaseSeason(s.ReleaseSeason),
		ExternalIds:   toExternalIDs(s.ExternalIDs),
		Episodes:      toEpisodes(s.Episodes),
	}
}

// toMovie converts one movie.
func toMovie(m model.Movie) *animev1.Movie {
	var alt *animev1.AlternateCutOf
	if m.AlternateCutOf != nil {
		alt = &animev1.AlternateCutOf{SeasonId: m.AlternateCutOf.SeasonID, Episodes: m.AlternateCutOf.Episodes}
	}
	return &animev1.Movie{
		Id:             m.ID,
		Titles:         toTitle(m.Titles),
		ReleaseDate:    toDate(m.ReleaseDate),
		ReleaseYear:    int32(m.ReleaseYear),
		ExternalIds:    toExternalIDs(m.ExternalIDs),
		AbsoluteNumber: toInt32Ptr(m.AbsoluteNumber),
		AlternateCutOf: alt,
	}
}

// toSpecial converts one special.
func toSpecial(sp model.Special) *animev1.Special {
	return &animev1.Special{
		Id:             sp.ID,
		Titles:         toTitle(sp.Titles),
		Format:         toSpecialFormat(sp.Format),
		ReleaseDate:    toDate(sp.ReleaseDate),
		ReleaseYear:    int32(sp.ReleaseYear),
		ExternalIds:    toExternalIDs(sp.ExternalIDs),
		Episodes:       toEpisodes(sp.Episodes),
		AbsoluteNumber: toInt32Ptr(sp.AbsoluteNumber),
	}
}

// toSeries converts one series and its installments (cast is not exposed yet).
func toSeries(s *model.Series) *animev1.Series {
	out := &animev1.Series{Id: s.ID, Titles: toTitle(s.Titles)}
	if len(s.Seasons) > 0 {
		out.Seasons = make([]*animev1.Season, len(s.Seasons))
		for i := range s.Seasons {
			out.Seasons[i] = toSeason(s.Seasons[i])
		}
	}
	if len(s.Movies) > 0 {
		out.Movies = make([]*animev1.Movie, len(s.Movies))
		for i := range s.Movies {
			out.Movies[i] = toMovie(s.Movies[i])
		}
	}
	if len(s.Specials) > 0 {
		out.Specials = make([]*animev1.Special, len(s.Specials))
		for i := range s.Specials {
			out.Specials[i] = toSpecial(s.Specials[i])
		}
	}
	return out
}

// toFranchise converts one franchise and its nested series and watch orders.
func toFranchise(f *model.Franchise) *animev1.Franchise {
	out := &animev1.Franchise{Id: f.ID, Titles: toTitle(f.Titles)}
	if len(f.Series) > 0 {
		out.Series = make([]*animev1.Series, len(f.Series))
		for i := range f.Series {
			out.Series[i] = toSeries(&f.Series[i])
		}
	}
	if len(f.WatchOrders) > 0 {
		out.WatchOrders = make([]*animev1.WatchOrder, len(f.WatchOrders))
		for i, wo := range f.WatchOrders {
			entries := make([]*animev1.WatchOrderEntry, len(wo.Entries))
			for j, e := range wo.Entries {
				entries[j] = &animev1.WatchOrderEntry{Ref: e.Ref, Note: e.Note}
			}
			out.WatchOrders[i] = &animev1.WatchOrder{Name: wo.Name, Entries: entries}
		}
	}
	return out
}

// toSearchResult converts a catalog entry to a search result.
func toSearchResult(e CatalogEntry) *animev1.SearchResult {
	return &animev1.SearchResult{
		Kind:        toEntryKind(e.Kind),
		Id:          e.ID,
		Titles:      toTitle(e.Titles),
		FranchiseId: e.FranchiseID,
	}
}
