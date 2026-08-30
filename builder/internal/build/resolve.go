package build

import (
	"fmt"
	"sort"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// resolveAnilistIDs works out which upstream entry each installment is, from the
// series' own title, and checks that against what the override says.
//
// A series has no AniList id — it spans several, one per installment — so there
// is no id to look the installments up by. There is a name: upstream lists the
// series' native title among the synonyms of each entry belonging to it, so
// finding those entries enumerates the family and the airing dates order it.
// Over the catalogue that reproduces the authored id for 123 of 152 series and
// never picks a different one, so a disagreement is worth reporting.
//
// It corroborates rather than replaces, which was not the first plan. The ids
// were briefly derived and deleted from the overrides — and upstream moved two
// days later, two series stopped resolving, and the build failed outright,
// because every other fact about an installment is read through this id and a
// missing one has nothing to fall back on. wikidataId can be derived the same
// way precisely because it is optional: a series that stops resolving loses its
// English title and the build carries on. This one is load-bearing, so the
// authored id stays and the resolution checks it.
//
// Nothing here reaches the network: the offline database is already loaded.
func (b *Builder) resolveAnilistIDs(s *model.Series, report *Report) {
	if b.sources.Offline == nil {
		return
	}
	pool := b.candidates(s)
	if len(pool) == 0 {
		return
	}
	for _, kind := range []installmentKind{seasonKind, movieKind, specialKind} {
		resolveKind(s, kind, pool, report)
	}
}

// candidates gathers the upstream entries carrying any of the series' own
// titles, de-duplicated by id.
//
// The romanization is a second key rather than the only one: a title written in
// Latin script is what upstream indexes some series under, and the native form
// what it indexes others under. Both are the series' own name, so both are
// asked; anything beyond them would be guessing at a resemblance.
func (b *Builder) candidates(s *model.Series) map[int]offlinedb.Anime {
	out := map[int]offlinedb.Anime{}
	for _, name := range append([]string{s.Titles.Original}, latinForms(s.Titles)...) {
		if name == "" {
			continue
		}
		for _, a := range b.sources.Offline.Titled(name) {
			if id := a.AnilistID(); id != 0 {
				out[id] = a
			}
		}
	}
	return out
}

// installmentKind pairs the nodes of one kind with the upstream media types
// that can fill them.
type installmentKind struct {
	name  string
	types map[offlinedb.MediaType]bool
	// ids returns a pointer to each node's id, in the order the installments
	// are numbered, so a resolution can be written back.
	ids func(*model.Series) []*model.ExternalIDs
}

var (
	seasonKind = installmentKind{
		name:  "season",
		types: map[offlinedb.MediaType]bool{offlinedb.TypeTV: true},
		ids: func(s *model.Series) []*model.ExternalIDs {
			ordered := orderedSeasons(s)
			out := make([]*model.ExternalIDs, len(ordered))
			for i, idx := range ordered {
				out[i] = &s.Seasons[idx].ExternalIDs
			}
			return out
		},
	}
	movieKind = installmentKind{
		name:  "movie",
		types: map[offlinedb.MediaType]bool{offlinedb.TypeMovie: true},
		ids: func(s *model.Series) []*model.ExternalIDs {
			out := make([]*model.ExternalIDs, len(s.Movies))
			for i := range s.Movies {
				out[i] = &s.Movies[i].ExternalIDs
			}
			return out
		},
	}
	specialKind = installmentKind{
		name: "special",
		types: map[offlinedb.MediaType]bool{
			offlinedb.TypeOVA: true, offlinedb.TypeONA: true, offlinedb.TypeSpecial: true,
		},
		ids: func(s *model.Series) []*model.ExternalIDs {
			out := make([]*model.ExternalIDs, len(s.Specials))
			for i := range s.Specials {
				out[i] = &s.Specials[i].ExternalIDs
			}
			return out
		},
	}
)

// orderedSeasons returns the indices of a series' seasons in the order they are
// numbered, breaking a tie on the id so split cours of one season keep a fixed
// order and two runs over the same override agree.
func orderedSeasons(s *model.Series) []int {
	idx := make([]int, len(s.Seasons))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		x, y := s.Seasons[idx[a]], s.Seasons[idx[b]]
		if x.Number != y.Number {
			return x.Number < y.Number
		}
		return x.ID < y.ID
	})
	return idx
}

// resolveKind works out the candidates of one media type, deals them out to the
// installments of the matching kind in airing order, and compares.
//
// All or nothing per kind, and only when the counts agree. A partial match
// would have to decide which installment the spare candidate belongs to, and a
// wrong answer there does not fail — it would claim a season holds another
// installment's id. Saying nothing is the honest result when the evidence does
// not line up, and the count that did not line up is silent too: upstream
// carries spin-offs and shorts under the same title for many series, so
// reporting every one would be a line per series saying only that the title is
// popular.
func resolveKind(s *model.Series, kind installmentKind, pool map[int]offlinedb.Anime, report *Report) {
	ids := kind.ids(s)
	if len(ids) == 0 {
		return
	}
	matching := make([]offlinedb.Anime, 0, len(pool))
	for _, a := range pool {
		if kind.types[a.Type] {
			matching = append(matching, a)
		}
	}
	if len(matching) != len(ids) {
		return
	}
	sort.Slice(matching, func(i, j int) bool { return airedEarlier(matching[i], matching[j]) })

	for i, e := range ids {
		want := matching[i].AnilistID()
		switch {
		case e.AnilistID == 0:
			// Nothing authored, so the resolution is the only answer there is.
			// A series may omit its ids and take these, accepting that a title
			// upstream stops carrying takes the build with it.
			e.AnilistID = want
			report.Coverage.Derived++
		case e.AnilistID == want:
			report.Coverage.Agreed++
		default:
			report.add("series "+s.ID, "externalIds", fmt.Sprintf(
				"%s %d of this series resolves to anilistId %d from the title, but %d is authored; one of them names the wrong entry",
				kind.name, i+1, want, e.AnilistID))
		}
	}
}

// airedEarlier orders two upstream entries by airing window, falling back to
// the id so entries sharing a window keep a fixed order between runs.
func airedEarlier(a, b offlinedb.Anime) bool {
	if a.AnimeSeason.Year != b.AnimeSeason.Year {
		return a.AnimeSeason.Year < b.AnimeSeason.Year
	}
	if qa, qb := quarterOf(a), quarterOf(b); qa != qb {
		return qa < qb
	}
	return a.AnilistID() < b.AnilistID()
}

// quarterOf orders an upstream airing quarter within its year.
func quarterOf(a offlinedb.Anime) int {
	return quarterIndex(model.ReleaseSeason(a.AnimeSeason.Season))
}
