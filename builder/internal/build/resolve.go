package build

import (
	"fmt"
	"sort"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// resolveAnilistIDs fills the AniList id of every installment a series has not
// authored one for, by finding the upstream entries that carry the series' own
// title and dealing them out in airing order.
//
// This is the last join key that was typed by hand. A series has no AniList id
// — it spans several, one per installment — so there is no id to look the
// installments up by, but there is a name: upstream lists the series' native
// title among the synonyms of each entry that belongs to it. Matching on that
// enumerates the family, and the airing dates order it.
//
// Measured over the catalogue before it was written: the title finds exactly
// the authored set for 123 of 152 series, covering 156 of 225 ids, and assigns
// every one of them to the installment it was authored on. Not one is assigned
// wrongly — every failure is a count that does not line up, which is refused
// below rather than guessed at.
//
// Nothing here reaches the network. The offline database is already loaded for
// the build, so a resolved id is as reproducible as the sources it came from.
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

// resolveKind deals the candidates of one media type out to the installments of
// the matching kind, in airing order.
//
// All or nothing per kind, and only when the counts agree. A partial match
// would have to decide which installment the spare candidate belongs to, and
// getting that wrong does not fail — it fills a season with another
// installment's episode count and says nothing. Refusing costs an id that stays
// authored; guessing costs a wrong fact nobody sees.
func resolveKind(s *model.Series, kind installmentKind, pool map[int]offlinedb.Anime, report *Report) {
	ids := kind.ids(s)
	if len(ids) == 0 {
		return
	}
	for _, e := range ids {
		if e.AnilistID != 0 {
			// The author named one, so the author is naming them all for this
			// kind. Filling around an authored id would mean guessing which
			// candidates it already accounts for.
			return
		}
	}

	matching := make([]offlinedb.Anime, 0, len(pool))
	for _, a := range pool {
		if kind.types[a.Type] {
			matching = append(matching, a)
		}
	}
	if len(matching) != len(ids) {
		report.add("series "+s.ID, "externalIds", fmt.Sprintf(
			"%d %s(s) authored but %d upstream entr(y/ies) carry this title; anilistId not resolved — author them, or narrow the titles",
			len(ids), kind.name, len(matching)))
		return
	}
	sort.Slice(matching, func(i, j int) bool { return airedEarlier(matching[i], matching[j]) })

	for i, e := range ids {
		e.AnilistID = matching[i].AnilistID()
		report.Coverage.Derived++
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
