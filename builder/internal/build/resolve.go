package build

import (
	"fmt"
	"math"
	"sort"

	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// resolveInstallments works out which upstream entry each installment is, from
// the series' own title, filling the anilistId where no override names one and
// checking it where an override does.
//
// A series has no AniList id — it spans several, one per installment — so there
// is no id to look the installments up by. There is a name: upstream lists the
// series' native title among the synonyms of each entry belonging to it, so
// finding those entries enumerates the family and the airing dates order it.
//
// The entry is the result, and the id is a fact about it rather than the point.
// Half of upstream's entries have no AniList id at all, and for those this is
// the only thing that can find them: they are returned in the resolution so the
// fills can read an entry that no id names. An installment resolved that way
// carries no externalIds.anilistId, which the model has always allowed.
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
func (b *Builder) resolveInstallments(s *model.Series, report *Report) resolution {
	resolved := resolution{}
	if b.sources.Offline == nil {
		return resolved
	}
	pool := b.candidates(s)
	if len(pool) == 0 {
		return resolved
	}
	// Upstream's AniList-bearing records first, and the rest of the pool only
	// where they cannot account for a kind's installments.
	//
	// The two halves are not equally good evidence. An entry carrying an
	// AniList id is upstream's merged record of a work — the cross-links
	// between providers reached it — and the pool of those is the one this
	// resolution was measured against. The other half is the residue the
	// cross-links have not reached, where a work already in the merged half
	// routinely appears a second time under its English title with a vaguer
	// date: "Arne no Jikenbo" carries nine providers, "The Case Book of Arne"
	// carries Anime News Network alone, and both are winter 2026 TV entries
	// listing the series' own title. Pooling the two halves outright makes a
	// one-season series look like a two-season one, and the count check then
	// declines on a series that resolved yesterday.
	//
	// Consulted second rather than not at all, because "the merged records
	// cannot account for these installments" is exactly the shape of a work
	// AniList does not carry, and never the shape of one it does. So a series
	// whose installments the merged half already explains resolves exactly as
	// it did before any of this was reachable, which is what keeps the
	// catalogue still.
	linked := onAnilist(pool)
	for _, kind := range []installmentKind{seasonKind, movieKind, specialKind} {
		if !resolveKind(s, kind, linked, resolved, report) {
			resolveKind(s, kind, pool, resolved, report)
		}
	}
	return resolved
}

// onAnilist returns the candidates upstream gives an AniList id.
func onAnilist(pool map[offlinedb.Key]offlinedb.Anime) map[offlinedb.Key]offlinedb.Anime {
	out := make(map[offlinedb.Key]offlinedb.Anime, len(pool))
	for k, a := range pool {
		if a.AnilistID() != 0 {
			out[k] = a
		}
	}
	return out
}

// resolution records the upstream entry the title resolution paired with an
// installment, keyed by the installment's own externalIds block.
//
// It carries the installments upstream lists on no AniList page. Every other
// one is looked back up through the id the resolution wrote onto it, and needs
// nothing here; these have no id to write, so the entry itself is the only
// handle on them and it has to reach the fills somehow.
//
// Per build rather than stored on the node, because the handle is a position in
// a rolling file (see offlinedb.Key) and nothing that leaves the process may
// reference an entry by one. Keyed by pointer for the same reason: an
// installment has no stable identity here beyond the block being filled.
type resolution map[*model.ExternalIDs]offlinedb.Anime

// candidates gathers the upstream entries carrying any of the series' own
// titles, de-duplicated by entry.
//
// By entry and not by AniList id, which is what this did until the entries
// without one were indexed: they all answer 0, so an id-keyed map collapsed
// every one of them into a single bucket and kept whichever came last.
//
// The romanization is a second key rather than the only one: a title written in
// Latin script is what upstream indexes some series under, and the native form
// what it indexes others under. Both are the series' own name, so both are
// asked; anything beyond them would be guessing at a resemblance.
func (b *Builder) candidates(s *model.Series) map[offlinedb.Key]offlinedb.Anime {
	out := map[offlinedb.Key]offlinedb.Anime{}
	for _, name := range append([]string{s.Titles.Original}, latinForms(s.Titles)...) {
		if name == "" {
			continue
		}
		for _, a := range b.sources.Offline.Titled(name) {
			out[a.Key()] = a
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
// installments of the matching kind in airing order, and compares. It reports
// whether the kind is settled — either resolved, or deliberately left alone —
// so a caller holding better evidence knows there is nothing more to try.
//
// All or nothing per kind, and only when the counts agree. A partial match
// would have to decide which installment the spare candidate belongs to, and a
// wrong answer there does not fail — it would claim a season holds another
// installment's id. Saying nothing is the honest result when the evidence does
// not line up, and the count that did not line up is silent too: upstream
// carries spin-offs and shorts under the same title for many series, so
// reporting every one would be a line per series saying only that the title is
// popular.
func resolveKind(s *model.Series, kind installmentKind, pool map[offlinedb.Key]offlinedb.Anime, resolved resolution, report *Report) bool {
	slots := kind.slots(s)
	if len(slots) == 0 {
		return true // nothing of this kind to fill, whatever the candidates are
	}
	if !kind.ordered && len(slots) > 1 {
		return true // refused on principle; a wider pool would not make it safer
	}
	matching := make([]offlinedb.Anime, 0, len(pool))
	for _, a := range pool {
		if kind.types[a.Type] {
			matching = append(matching, a)
		}
	}
	if len(matching) != len(slots) {
		return false
	}
	sort.Slice(matching, func(i, j int) bool { return airedEarlier(matching[i], matching[j]) })

	for i, sl := range slots {
		entry := matching[i]
		want := entry.AnilistID()
		switch {
		case want == 0 && sl.ids.AnilistID == 0:
			// Upstream lists this installment on no AniList page, so there is
			// no id to write and the entry itself is the answer. The fills read
			// it exactly as they read any other, and the node keeps no
			// anilistId — the honest record of a work AniList does not carry.
			resolved[sl.ids] = entry
			report.Coverage.Unlisted++
		case want == 0:
			// An authored id and a record that has none. Silent, and not a
			// disagreement: the other cases compare two claims about which
			// AniList entry this is, and a record with no AniList id makes no
			// such claim. What it usually is instead is upstream's second,
			// un-cross-linked record of the very work the authored id names,
			// and reporting that on every build would be a line per series
			// saying only that upstream has not finished merging.
		case sl.ids.AnilistID == 0:
			// Nothing authored, so the resolution is the only answer there is.
			// A series may omit its ids and take these, accepting that a title
			// upstream stops carrying takes the build with it.
			sl.ids.AnilistID = want
			report.Coverage.Derived++
		case sl.ids.AnilistID == want:
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
	return true
}

// airedEarlier orders two upstream entries by airing window, falling back to
// the id and then to upstream's own order so entries sharing a window keep a
// fixed order between runs.
//
// The second fallback is what keeps that promise for entries with no AniList
// id: they all answer 0, so the id alone left every pair of them equal and the
// unstable sort was free to return them in either order — the same override
// resolving differently between two runs, which is churn in data/.
func airedEarlier(a, b offlinedb.Anime) bool {
	if ya, yb := airedYear(a), airedYear(b); ya != yb {
		return ya < yb
	}
	if qa, qb := quarterOf(a), quarterOf(b); qa != qb {
		return qa < qb
	}
	if ia, ib := a.AnilistID(), b.AnilistID(); ia != ib {
		return ia < ib
	}
	return a.Key() < b.Key()
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
