package build

import (
	"fmt"
	"sort"

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
// Every installment's id is authored by hand and nothing has ever checked one.
// It is the join key the whole build hangs off — titles, release season,
// episode counts and the AniDB/TVDB cross-map all come from the entry it names
// — so an id naming the wrong entry does not fail, it silently fills a series
// with another show's facts. `lookup` catches an id that exists nowhere; these
// catch an id that exists and is wrong.
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
	if len(ids) < 2 {
		// A lone installment has no sibling to be consistent with. Silent
		// rather than noted: most of the catalogue's series have exactly one,
		// and a line each saying "not checked" would drown the ones that found
		// something.
		return
	}
	for _, id := range sortedIntKeys(ids) {
		if b.reachesSibling(id, ids) {
			continue
		}
		report.add(ids[id], "externalIds", fmt.Sprintf(
			"anilistId %d is not linked to any other installment of %s in the offline database's relatedAnime graph; check it names the right entry",
			id, s.ID))
	}
}

// reachesSibling reports whether id reaches any other of the series' ids by
// walking relatedAnime. The walk stops the moment one is found, which for
// correct data is almost always the first hop.
func (b *Builder) reachesSibling(id int, ids map[int]string) bool {
	seen := map[int]bool{id: true}
	queue := []int{id}
	for len(queue) > 0 && len(seen) < relatedWalkCap {
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
				return true
			}
			seen[next] = true
			queue = append(queue, next)
		}
	}
	return false
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
		seasons = append(seasons, datedSeason{season.Number, season.ReleaseYear, season.ReleaseSeason, season.ID})
	}
	sort.Slice(seasons, func(i, j int) bool { return seasons[i].number < seasons[j].number })

	for i := 1; i < len(seasons); i++ {
		prev, cur := seasons[i-1], seasons[i]
		if cur.number == prev.number {
			continue // split cours of one season share a number and a year
		}
		if cur.airedBefore(prev) {
			report.add("season "+cur.id, "externalIds", fmt.Sprintf(
				"season %d aired %s %d, before season %d's %s %d; check anilistId %d names the right installment",
				cur.number, quarterName(cur.season), cur.year,
				prev.number, quarterName(prev.season), prev.year,
				anilistIDOf(s, cur.id)))
		}
	}
}

// datedSeason is a season reduced to what the chronology check compares.
type datedSeason struct {
	number int
	year   int
	season model.ReleaseSeason
	id     string
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

// anilistIDOf returns the id authored on the named season, for the report.
func anilistIDOf(s *model.Series, seasonID string) int {
	for _, season := range s.Seasons {
		if season.ID == seasonID {
			return season.ExternalIDs.AnilistID
		}
	}
	return 0
}

// anilistIDs collects every AniList id under a series, mapped to the entity
// name a report line would use.
func anilistIDs(s *model.Series) map[int]string {
	out := map[int]string{}
	for _, x := range s.Seasons {
		if x.ExternalIDs.AnilistID != 0 {
			out[x.ExternalIDs.AnilistID] = "season " + x.ID
		}
	}
	for _, x := range s.Movies {
		if x.ExternalIDs.AnilistID != 0 {
			out[x.ExternalIDs.AnilistID] = "movie " + x.ID
		}
	}
	for _, x := range s.Specials {
		if x.ExternalIDs.AnilistID != 0 {
			out[x.ExternalIDs.AnilistID] = "special " + x.ID
		}
	}
	return out
}

// sortedIntKeys returns a map's keys in ascending order.
func sortedIntKeys(m map[int]string) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
