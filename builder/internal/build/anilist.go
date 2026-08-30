package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// relatedWalkCap bounds the relatedAnime walk. Upstream's graph is tight — the
// widest component any series in this catalogue sits in is nine entries, and
// finding every sibling never expanded more than 35 — so the cap is never
// reached by real data. It exists so that a wrong id, whose walk has no reason
// to stop early, cannot turn a build into a traversal of the whole database.
const relatedWalkCap = 500

// checkAnilistIDs reports an authored anilistId that does not look like it
// belongs where it sits.
//
// The id is the join key the whole build hangs off — titles, release season,
// episode counts and the AniDB/TVDB cross-map all come from the entry it names
// — so an id naming the wrong entry does not fail, it silently fills a series
// with another show's facts. `lookup` catches an id that exists nowhere; these
// catch an id that exists and is wrong.
//
// They run over derived and authored ids alike. A derived id is not exempt: the
// resolution pairs installments with entries by airing order, so an upstream
// catalogue that gains or loses an entry under a series title can shift the
// pairing without anything else looking wrong. These two checks read the graph
// and the dates instead, which is why they caught that shift when the title
// resolution did not.
//
// Two independent checks, because they catch different mistakes. The graph one
// asks "is this the right work?" and cannot see a season pointed at its own
// sibling; the chronology one asks "is this the right installment of it?" and
// cannot see an id from an unrelated show.
//
// Both report rather than fail. Which installments constitute a series is this
// project's editorial judgement — the thing it exists to provide — and upstream
// relatedAnime is a rolling source that moves under a pinned build. A hard
// error would let either overrule the author; a note puts it in front of them.
// The missing id in lookup does fail, and the difference is that one has no
// answer to fall back on while these have a defensible one they merely doubt.
func (b *Builder) checkAnilistIDs(s *model.Series, report *Report) {
	b.checkSameWork(s, report)
	checkSeasonChronology(s, report)
}

// checkSameWork reports an installment upstream does not connect to the rest of
// the series, walking relatedAnime outward from each in turn.
//
// Reachability rather than a direct link, because upstream chains sequels
// neighbour to neighbour: Tensei Shitara Slime Datta Ken's five installments
// are linked S1–S2–S3–…, so its first and last are four hops apart and a
// direct-link test would report three false positives on it alone.
//
// Asked once per id rather than once per series, which costs a handful of tiny
// walks and buys the right answer about which id is wrong. Seeding a single
// walk from one of them means that when the odd one out happens to be the seed,
// every id it cannot reach is reported and the wrong one is not — the first
// version of this named Frieren's season 1 when season 2 held the bad id.
func (b *Builder) checkSameWork(s *model.Series, report *Report) {
	if b.sources.Offline == nil {
		return
	}
	ids := anilistIDs(s)
	report.Coverage.Considered += len(ids)

	// Reported before anything else, and before the lone-installment exit: a
	// series whose only two installments share an id has one distinct id, and
	// would otherwise leave here counted as unverifiable and unmentioned.
	dupes := duplicates(ids)
	for _, id := range sortedKeysOf(dupes) {
		report.add("series "+s.ID, "externalIds", fmt.Sprintf(
			"anilistId %d is authored on more than one installment (%s); two installments are two different works, so one of them is wrong",
			id, strings.Join(dupes[id], ", ")))
	}

	distinct := map[int]string{}
	for _, x := range ids {
		distinct[x.id] = x.entity
	}
	if len(distinct) < 2 {
		// A lone installment has no sibling to be consistent with. Silent
		// rather than noted: most of the catalogue's series have exactly one,
		// and a line each saying "not checked" would drown the ones that found
		// something. It is counted instead, so the build can say how much of
		// the join-key surface went unchecked rather than implying it passed.
		// Counted per installment, not per distinct id, so a duplicate does not
		// shrink the total the coverage line is a fraction of.
		report.Coverage.Alone += len(ids)
		return
	}
	for _, id := range sortedKeysOf(distinct) {
		reached, capped := b.reachesSibling(id, distinct)
		if reached {
			report.Coverage.Corroborated++
			continue
		}
		// Not counted as corroborated: it was checked and failed, which the
		// note says. Counting it either way would make the summary disagree
		// with the line above it.
		why := "is not linked to any other installment of %s in the offline database's relatedAnime graph; check it names the right entry"
		if capped {
			why = "could not be linked to another installment of %s within the walk limit, so this is inconclusive rather than wrong"
		}
		report.add(distinct[id], "externalIds", fmt.Sprintf("anilistId %d "+why, id, s.ID))
	}
}

// reachesSibling reports whether id reaches any other of the series' ids by
// walking relatedAnime, and whether the walk stopped at the cap rather than
// running out of graph. The walk ends the moment a sibling is found, which for
// correct data is almost always the first hop.
//
// The second return is what keeps the cap honest. Without it a truncated walk
// is indistinguishable from an exhausted one, and a series in some enormous
// franchise component would be reported as "not linked — check it names the
// right entry" when the truth is that nobody looked far enough.
func (b *Builder) reachesSibling(id int, ids map[int]string) (reached, capped bool) {
	seen := map[int]bool{id: true}
	queue := []int{id}
	for len(queue) > 0 {
		if len(seen) >= relatedWalkCap {
			return false, true
		}
		cur := queue[0]
		queue = queue[1:]
		entry, ok := b.sources.Offline.Lookup(cur)
		if !ok {
			continue
		}
		for _, next := range entry.RelatedAnilistIDs() {
			if seen[next] {
				continue
			}
			if _, sibling := ids[next]; sibling {
				return true, false
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return false, false
}

// checkSeasonChronology reports a season that aired before the one numbered
// ahead of it. The release year and season are filled from the entry the
// anilistId names, so a number and a date that disagree mean one of them is
// wrong — most often an id pasted from the wrong installment, which the graph
// check cannot see because the right and wrong entries are siblings.
//
// A season whose release is unknown is skipped rather than assumed early: the
// upstream entry carries no animeSeason, which says nothing about the order.
func checkSeasonChronology(s *model.Series, report *Report) {
	var seasons []datedSeason
	for _, season := range s.Seasons {
		if season.ReleaseYear == 0 || season.ExternalIDs.AnilistID == 0 {
			continue
		}
		seasons = append(seasons, datedSeason{
			season.Number, season.ReleaseYear, season.ReleaseSeason,
			season.ID, season.ExternalIDs.AnilistID,
		})
	}
	// Ordered by number and then by id, never by number alone. Split cours
	// share a number, and an unstable sort would leave which of them sits
	// beside the neighbouring season up to the sort's internals — so the same
	// override could compare different pairs between runs, and report a finding
	// on one run and not the next.
	sort.Slice(seasons, func(i, j int) bool {
		if seasons[i].number != seasons[j].number {
			return seasons[i].number < seasons[j].number
		}
		return seasons[i].id < seasons[j].id
	})

	for i := 1; i < len(seasons); i++ {
		prev, cur := seasons[i-1], seasons[i]
		if cur.number == prev.number {
			continue // split cours of one season share a number and a year
		}
		// A year both share tells us nothing when either quarter is unknown:
		// upstream leaves animeSeason.season blank while carrying the year, and
		// reading blank as "earliest" turned that into a finding against a
		// season that may well have aired later.
		if cur.year == prev.year && (cur.season == "" || prev.season == "") {
			continue
		}
		if cur.airedBefore(prev) {
			// Both are named, and neither is accused. The pair is out of order;
			// which of the two carries the wrong id is not something this can
			// tell — the branch's own test puts season 3's id on season 1 and
			// the older wording blamed season 2 for it.
			report.add("series "+s.ID, "externalIds", fmt.Sprintf(
				"season %d (anilistId %d) aired %s %d, before season %d (anilistId %d) aired %s %d; one of the two names the wrong installment",
				cur.number, cur.anilistID, quarterName(cur.season), cur.year,
				prev.number, prev.anilistID, quarterName(prev.season), prev.year))
		}
	}
}

// datedSeason is a season reduced to what the chronology check compares.
type datedSeason struct {
	number    int
	year      int
	season    model.ReleaseSeason
	id        string
	anilistID int
}

// airedBefore compares two releases by year, then by quarter within it.
func (d datedSeason) airedBefore(other datedSeason) bool {
	if d.year != other.year {
		return d.year < other.year
	}
	return quarterIndex(d.season) < quarterIndex(other.season)
}

// quarterIndex orders the airing quarters within a year. An unset season sorts
// first, which cannot produce a false report on its own: a year that is equal
// and a quarter that is missing on the later season is the one case this would
// flag, and it is a real gap worth seeing.
func quarterIndex(s model.ReleaseSeason) int {
	switch s {
	case model.SeasonWinter:
		return 0
	case model.SeasonSpring:
		return 1
	case model.SeasonSummer:
		return 2
	case model.SeasonFall:
		return 3
	default:
		return -1
	}
}

// quarterName renders a release season for a report line, naming an unset one
// rather than printing an empty string into the middle of a sentence.
func quarterName(s model.ReleaseSeason) string {
	if s == "" {
		return "an unknown quarter of"
	}
	return string(s)
}

// installment is one node carrying an AniList id, with the name a report line
// would use for it.
type installment struct {
	id     int
	entity string
}

// anilistIDs collects every installment of a series that carries an AniList id,
// in a fixed order.
//
// A slice rather than a map keyed by id, because two nodes can carry the same
// one. A map silently merged them, and a series whose two seasons were given
// the same id by a copy-paste then looked like a single installment: the
// duplicate disappeared, the checks skipped the series as having nothing to
// compare, and the coverage line counted one unverifiable id where there were
// two.
func anilistIDs(s *model.Series) []installment {
	var out []installment
	for _, x := range s.Seasons {
		if x.ExternalIDs.AnilistID != 0 {
			out = append(out, installment{x.ExternalIDs.AnilistID, "season " + x.ID})
		}
	}
	for _, x := range s.Movies {
		if x.ExternalIDs.AnilistID != 0 {
			out = append(out, installment{x.ExternalIDs.AnilistID, "movie " + x.ID})
		}
	}
	for _, x := range s.Specials {
		if x.ExternalIDs.AnilistID != 0 {
			out = append(out, installment{x.ExternalIDs.AnilistID, "special " + x.ID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].id != out[j].id {
			return out[i].id < out[j].id
		}
		return out[i].entity < out[j].entity
	})
	return out
}

// duplicates reports the ids carried by more than one installment, naming every
// node that carries each. Two installments are two different works, so one id
// on both is always a mistake — and the one mistake the other checks cannot
// see, since a duplicate is trivially "linked to" and "in order with" itself.
func duplicates(ids []installment) map[int][]string {
	byID := map[int][]string{}
	for _, x := range ids {
		byID[x.id] = append(byID[x.id], x.entity)
	}
	for id, names := range byID {
		if len(names) < 2 {
			delete(byID, id)
		}
	}
	return byID
}

// sortedKeysOf returns a map's integer keys in ascending order, so a report
// lists its findings the same way on every run over the same inputs.
func sortedKeysOf[V any](m map[int]V) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
