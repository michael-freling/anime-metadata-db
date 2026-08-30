package build

import (
	"fmt"
	"math"
	"sort"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// resolveAnilistIDs works out which upstream entry each installment is, from the
// series' own title, filling the id where no override names one and checking it
// where an override does.
//
// A series has no AniList id — it spans several, one per installment — so there
// is no id to look the installments up by. There is a name: upstream lists the
// series' native title among the synonyms of each entry belonging to it, so
// finding those entries enumerates the family and the airing dates order it.
//
// This supplies 150 of the catalogue's 225 ids, and the dataset it produces is
// byte-identical to the one the authored ids produced: over the whole
// catalogue the resolution never once named a different entry from the editor.
// That is what makes deriving them safe rather than merely convenient — the
// measurement is a build with every id deleted, compared against the committed
// tree, not an argument that the matching looks sound.
//
// The remaining 75 sit in 24 series and stay authored, because upstream does
// not list them under the series title at all. They are the residue this cannot
// reach, not a second opinion about ids it can.
//
// The cost is that a required field now comes from a rolling source. When
// upstream stops listing a series under its own title the build fails outright,
// because every other fact about an installment is read through this id — see
// lookup, which says so and names the id to author.
//
// That is not hypothetical: between the pinned database and the one upstream
// published today, Kanojo, Okarishimasu's five seasons and Fate/strange Fake
// stopped resolving. Their six ids are authored for exactly that reason, and
// under the pinned database the resolution still reproduces them, which is what
// the "authored and independently reproduced" count reports. Expect a couple
// more per upstream release, each one a line to add.
//
// Six ids to author is the price of not hand-authoring 150, and a build that
// stops is the right response to a join key that can no longer be computed: the
// alternative is a dataset that quietly re-points at whatever upstream now
// offers.
//
// A disagreement with an authored id is reported but not enforced, because it
// can be upstream's mistake rather than the author's: MF Ghost's third season
// is not listed under the series title while its announced fourth is, so the
// pool and the overrides agree on count and differ by a member, and position
// three compares wrong against a perfectly good id.
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
	// slots returns each node in the order the resolution should be paired
	// against, so a resolution can be written back and named.
	slots func(*model.Series) []slot
	// ordered reports whether the nodes of this kind carry something that puts
	// them in airing order. Where they do not, pairing more than one against
	// date-sorted candidates would be guesswork.
	ordered bool
}

// slot is one node a resolution can be paired with: its id to fill or compare,
// and the name a report line uses for it.
type slot struct {
	ids  *model.ExternalIDs
	name string
}

var (
	seasonKind = installmentKind{
		name:    "season",
		types:   map[offlinedb.MediaType]bool{offlinedb.TypeTV: true},
		ordered: true,
		slots: func(s *model.Series) []slot {
			idx := orderedSeasons(s)
			out := make([]slot, len(idx))
			for i, at := range idx {
				out[i] = slot{&s.Seasons[at].ExternalIDs, "season " + s.Seasons[at].ID}
			}
			return out
		},
	}
	// Movies and specials carry no number, so the override's file order is the
	// only order there is — and that is the author's, not upstream's airing
	// order the candidates are sorted by. Pairing two of them would be lining
	// up two lists that agree only by luck, so only a single one is paired,
	// where there is nothing to get wrong.
	movieKind = installmentKind{
		name:  "movie",
		types: map[offlinedb.MediaType]bool{offlinedb.TypeMovie: true},
		slots: func(s *model.Series) []slot {
			out := make([]slot, len(s.Movies))
			for i := range s.Movies {
				out[i] = slot{&s.Movies[i].ExternalIDs, "movie " + s.Movies[i].ID}
			}
			return out
		},
	}
	specialKind = installmentKind{
		name: "special",
		types: map[offlinedb.MediaType]bool{
			offlinedb.TypeOVA: true, offlinedb.TypeONA: true, offlinedb.TypeSpecial: true,
		},
		slots: func(s *model.Series) []slot {
			out := make([]slot, len(s.Specials))
			for i := range s.Specials {
				out[i] = slot{&s.Specials[i].ExternalIDs, "special " + s.Specials[i].ID}
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
	slots := kind.slots(s)
	if len(slots) == 0 {
		return
	}
	if !kind.ordered && len(slots) > 1 {
		return
	}
	matching := make([]offlinedb.Anime, 0, len(pool))
	for _, a := range pool {
		if kind.types[a.Type] {
			matching = append(matching, a)
		}
	}
	if len(matching) != len(slots) {
		return
	}
	sort.Slice(matching, func(i, j int) bool { return airedEarlier(matching[i], matching[j]) })

	for i, sl := range slots {
		want := matching[i].AnilistID()
		switch sl.ids.AnilistID {
		case 0:
			// Nothing authored, so the resolution is the only answer there is.
			// A series may omit its ids and take these, accepting that a title
			// upstream stops carrying takes the build with it.
			sl.ids.AnilistID = want
			report.Coverage.Derived++
		case want:
			report.Coverage.Agreed++
		default:
			// Named by the node, not by its position in the ordering: a split
			// cour sits in the third slot while its own number is 2, and
			// "season 3" would send a reader looking for a season that is not
			// there.
			report.addCoded(CodeTitleDisagrees, sl.name, "externalIds", fmt.Sprintf(
				"resolves to anilistId %d from the series' title, but %d is authored; check which names the right entry, or upstream may simply not carry this title on both",
				want, sl.ids.AnilistID))
		}
	}
}

// airedEarlier orders two upstream entries by airing window, falling back to
// the id so entries sharing a window keep a fixed order between runs.
func airedEarlier(a, b offlinedb.Anime) bool {
	if ya, yb := airedYear(a), airedYear(b); ya != yb {
		return ya < yb
	}
	if qa, qb := quarterOf(a), quarterOf(b); qa != qb {
		return qa < qb
	}
	return a.AnilistID() < b.AnilistID()
}

// airedYear is an entry's airing year, with an unknown one sorted last rather
// than first.
//
// Upstream carries an announced-but-unscheduled installment with no
// animeSeason, and a zero year would place it before everything — which is how
// "MF Ghost Final Season" appearing upstream shifted that series' three real
// seasons by one and made every id look wrong. An installment with no date yet
// is the newest thing there is, not the oldest.
func airedYear(a offlinedb.Anime) int {
	if a.AnimeSeason.Year == 0 {
		return math.MaxInt
	}
	return a.AnimeSeason.Year
}

// quarterOf orders an upstream airing quarter within its year.
func quarterOf(a offlinedb.Anime) int {
	return quarterIndex(model.ReleaseSeason(a.AnimeSeason.Season))
}
