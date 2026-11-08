// Package importer turns one year of the upstream catalogue into authored
// override files.
//
// It writes the override layer only — the structure the open sources cannot
// express — and leaves every fact to `builder build`, which resolves titles,
// dates, episode counts and ids from the pinned sources. So an imported series
// is indistinguishable from a hand-authored one, and re-running the build over
// it is the same operation as for any other record.
//
// # What it refuses to decide
//
// Series boundaries are authored in this project, not derived, and the reason
// shows in the data: upstream's relatedAnime graph, followed transitively, puts
// 5,597 entries in one component — Evangelion, Oshi no Ko and brand-collab
// shorts together — because promo specials bridge unrelated franchises.
// Excluding out-of-scope types still leaves Fate in a 182-member blob.
//
// So the grouping here is deliberately timid. It joins two entries only when
// they are both in the year being imported and upstream links them directly,
// and it refuses outright to touch a series the catalogue already has: those
// are reported for a person to merge by hand. Importing a work into its own
// fresh series when it belongs to an existing one is the error that produced
// the orphaned Rascal record, and it is the one error a later import cannot
// detect, because by then the duplicate looks like an ordinary series.
package importer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/sources/offlinedb"
)

// Series is one override file to write.
type Series struct {
	ID        string   // slug, and the stem of the file
	Original  string   // native title, the handle the build resolves installments by
	Romanized string   // upstream's own romanisation
	Seasons   []string // work ids, in airing order
	Movies    []string
}

// Merge is a work that belongs to a series the catalogue already carries.
// Reported, never written: see the package comment.
type Merge struct {
	Title     string
	AnilistID int
	Into      []int // the already-known AniList ids it is linked to
}

// Unresolvable is a series this declines to write because the build could not
// fill it. Reported with the reason, so the fix is an authored file rather than
// a failed build somebody has to bisect.
type Unresolvable struct {
	Title  string
	Reason string
}

// Plan is what one year's import would do.
type Plan struct {
	Year         int
	New          []Series
	Merges       []Merge
	Unresolvable []Unresolvable
	Present      []string // titles already in the catalogue, skipped
	// Skipped counts entries excluded by the catalogue's scope rule, so a run
	// reports the size of what it declined rather than silently dropping it.
	Skipped int
}

// Build plans the import of one year.
//
// known is the set of AniList ids the catalogue already carries. It is what
// makes the refusal above possible, so passing an empty set does not merely
// lose a safety net — it turns every existing series into a duplicate.
func Build(db *offlinedb.Database, year int, known map[int]bool) Plan {
	plan := Plan{Year: year}

	var pool []offlinedb.Anime
	for a := range db.All() {
		if a.AnimeSeason.Year != year {
			continue
		}
		// The catalogue carries TV series and films only. Upstream lists music
		// videos, OP animations and promo clips as SPECIAL/ONA/OVA, and a
		// zero-episode entry is an announcement rather than a release.
		if a.Type != offlinedb.TypeTV && a.Type != offlinedb.TypeMovie || !released(a) {
			plan.Skipped++
			continue
		}
		pool = append(pool, a)
	}
	// Upstream order is its own file order, which moves between releases. Sort
	// so the same sources produce the same plan.
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].Title < pool[j].Title })

	// Earliest first, so when two installments of one series both seeded this
	// year the earlier one names it.
	sort.SliceStable(pool, func(i, j int) bool { return airedEarlier(pool[i], pool[j]) })

	// claimed holds every entry an already-planned series declares. A seed
	// inside one is an installment of it, not a new series: without this, the
	// second cour of a show that aired twice in one year becomes its own
	// record, which is the duplicate the whole merge report exists to prevent
	// — and here it would be a duplicate created within a single run.
	claimed := map[offlinedb.Key]bool{}
	for _, a := range pool {
		if claimed[a.Key()] {
			continue
		}
		switch {
		case known[a.AnilistID()]:
			plan.Present = append(plan.Present, a.Title)
			continue
		case len(linkedKnown(db, a, known)) > 0:
			plan.Merges = append(plan.Merges, Merge{
				Title:     a.Title,
				AnilistID: a.AnilistID(),
				Into:      linkedKnown(db, a, known),
			})
			continue
		}
		s, members, err := series(db, a, NativeTitle(a))
		if err != nil {
			plan.Unresolvable = append(plan.Unresolvable, Unresolvable{Title: a.Title, Reason: err.Error()})
			// Claimed even on refusal: the other installments of a series this
			// declined would otherwise each try again and each be refused, so
			// one hand-authoring note becomes several.
			for _, k := range members {
				claimed[k] = true
			}
			continue
		}
		for _, k := range members {
			claimed[k] = true
		}
		plan.New = append(plan.New, s)
	}
	sort.Slice(plan.New, func(i, j int) bool { return plan.New[i].ID < plan.New[j].ID })
	return plan
}

// linkedKnown returns the already-known AniList ids a is directly related to.
//
// Resolved through the database rather than from a.RelatedAnilistIDs(), which
// reads only the relations upstream happened to record as AniList URLs. On the
// 2020 catalogue that difference is 8 candidates against 29 — so the cheaper
// version would have waved two thirds of the orphan risk through.
func linkedKnown(db *offlinedb.Database, a offlinedb.Anime, known map[int]bool) []int {
	seenID := map[int]bool{}
	add := func(id int) {
		if id != 0 && known[id] && !seenID[id] {
			seenID[id] = true
		}
	}
	// Both halves, because each reaches a link the other does not. Resolving
	// through the database reads relations recorded as any provider's URL, which
	// is most of them; reading the AniList ids straight off the relation also
	// catches a link to an entry upstream no longer carries, where there is
	// nothing to resolve to but the id is still the one the catalogue knows.
	for _, rel := range db.Related(a) {
		add(rel.AnilistID())
	}
	for _, id := range a.RelatedAnilistIDs() {
		add(id)
	}
	out := make([]int, 0, len(seenID))
	for id := range seenID {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// series builds the override for the series named by original.
//
// The installments are the upstream entries carrying the series' title, which
// is how `build` finds them too — deliberately the same rule, because the build
// refuses to resolve a kind unless the number of candidates equals the number
// of declared installments. Declaring only the installments that aired in the
// year being imported therefore produces a file that cannot build: a series
// with a second season next year has two TV candidates and one slot. So a
// series is written whole, and the year is the seed rather than a filter — the
// shape the 2025 import already left behind in the 89 series carrying a
// pre-2025 work.
func series(db *offlinedb.Database, seed offlinedb.Anime, original string) (Series, []offlinedb.Key, error) {
	id := Slug(seed.Title)
	s := Series{ID: id, Original: original, Romanized: seed.Title}

	// The same pool the build will assemble: both title forms, deduplicated by
	// the database's own handle.
	pool := map[offlinedb.Key]offlinedb.Anime{}
	for _, name := range []string{original, seed.Title} {
		for _, a := range db.Titled(name) {
			pool[a.Key()] = a
		}
	}
	members := make([]offlinedb.Key, 0, len(pool))
	for k := range pool {
		members = append(members, k)
	}
	var tv, films []offlinedb.Anime
	for _, a := range pool {
		switch a.Type {
		case offlinedb.TypeTV:
			tv = append(tv, a)
		case offlinedb.TypeMovie:
			films = append(films, a)
		}
	}
	// Announcements are not in the catalogue, and the two kinds have to drop
	// them differently because resolveKind short-circuits on zero slots but
	// refuses on a mismatched count.
	//
	// A season: declaring it would put an unreleased work in the catalogue, and
	// omitting it leaves more candidates than slots, which cannot resolve. So
	// the series is refused and left for a person.
	for _, a := range tv {
		if !released(a) {
			return Series{}, members, fmt.Errorf("upstream lists an unreleased season under this title (%q, %s), "+
				"so the installment count cannot be matched", a.Title, a.Status)
		}
	}
	// A film: dropping every film slot is resolvable, because a kind with no
	// slots is skipped whatever the candidates are. So an unreleased film means
	// the series carries no film yet, which is the truth.
	for _, a := range films {
		if !released(a) {
			films = nil
			break
		}
	}
	sort.SliceStable(tv, func(i, j int) bool { return airedEarlier(tv[i], tv[j]) })
	sort.SliceStable(films, func(i, j int) bool { return airedEarlier(films[i], films[j]) })

	for i := range tv {
		s.Seasons = append(s.Seasons, fmt.Sprintf("%s-s%d", id, i+1))
	}
	// A movie node carries no number, so the build will not pair more than one
	// against date-sorted candidates — it declines the whole kind rather than
	// guess, and the installments are then left with nothing resolved. Writing
	// two film slots therefore produces a file that cannot build, whatever
	// their ids are called.
	if len(films) > 1 {
		names := make([]string, 0, len(films))
		for _, f := range films {
			names = append(names, f.Title)
		}
		sort.Strings(names)
		return Series{}, members, fmt.Errorf("%d films share this title (%s), and a film carries no number "+
			"for the build to order them by", len(films), strings.Join(names, "; "))
	}
	if len(films) == 1 {
		s.Movies = append(s.Movies, id+"-film")
	}
	if len(s.Seasons) == 0 && len(s.Movies) == 0 {
		return Series{}, members, fmt.Errorf("no in-scope upstream entry carries this title")
	}
	return s, members, nil
}

// released reports whether upstream has an aired work here rather than an
// announcement.
//
// Three conditions because each catches a shape the others miss: no episodes is
// an entry with nothing in it, UPCOMING is upstream saying so outright, and a
// missing year is an entry nobody has dated — "Majo no Tabitabi Movie" carries
// one episode and the UPCOMING status with no year at all, and an episode count
// alone waved it into the catalogue as a work released in year zero.
func released(a offlinedb.Anime) bool {
	return a.Episodes > 0 && a.Status != "UPCOMING" && a.AnimeSeason.Year > 0
}

// airedEarlier orders two entries by airing year then quarter, so season
// numbers follow the story rather than upstream's file order.
func airedEarlier(a, b offlinedb.Anime) bool {
	if a.AnimeSeason.Year != b.AnimeSeason.Year {
		return a.AnimeSeason.Year < b.AnimeSeason.Year
	}
	if q1, q2 := quarter(a.AnimeSeason.Season), quarter(b.AnimeSeason.Season); q1 != q2 {
		return q1 < q2
	}
	return a.Title < b.Title
}

func quarter(s string) int {
	switch strings.ToUpper(s) {
	case "WINTER":
		return 1
	case "SPRING":
		return 2
	case "SUMMER":
		return 3
	case "FALL":
		return 4
	}
	return 9
}

// NativeTitle picks the series' own native title out of upstream's synonyms.
//
// The build resolves a series' installments by this title, so it has to be one
// upstream actually carries — a translation or a tidied-up version would
// resolve to nothing and leave every id to be authored by hand.
//
// Upstream's `title` is usually a romanisation and its synonyms mix Japanese
// with Simplified and Traditional Chinese. Kana is the discriminator: hiragana
// and katakana are Japanese-only, so a CJK synonym carrying them is not
// Chinese. Among those the longest wins, which passes over abbreviations
// ("青ブタ") and arc labels ("青春ブタ野郎 大学生編") in favour of the full title.
func NativeTitle(a offlinedb.Anime) string {
	best := ""
	for _, s := range a.Synonyms {
		if !hasKana(s) {
			continue
		}
		if len([]rune(s)) > len([]rune(best)) {
			best = s
		}
	}
	if best != "" {
		return best
	}
	// Nothing kana-bearing: an entry upstream lists only under romanisations,
	// or a non-Japanese work. Its own title is then the only native handle
	// there is, and it is what the build will match on.
	return a.Title
}

func hasKana(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

var (
	nonSlug  = regexp.MustCompile(`[^a-z0-9]+`)
	edgeDash = regexp.MustCompile(`^-+|-+$`)
)

// Slug derives a record id from a romanised title.
//
// Ids are permanent and appear in the public API, so this is deliberately
// plain: lowercase ASCII words joined by hyphens, with everything else
// dropped. Diacritics are folded rather than stripped so "Yarō" and "Yarou"
// do not become different ids.
func Slug(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch r {
		case 'ā', 'ă', 'â', 'à', 'á':
			b.WriteRune('a')
		case 'ē', 'ĕ', 'ê', 'è', 'é':
			b.WriteRune('e')
		case 'ī', 'ĭ', 'î', 'ì', 'í':
			b.WriteRune('i')
		case 'ō', 'ŏ', 'ô', 'ò', 'ó':
			b.WriteRune('o')
		case 'ū', 'ŭ', 'û', 'ù', 'ú':
			b.WriteRune('u')
		default:
			b.WriteRune(r)
		}
	}
	s := nonSlug.ReplaceAllString(b.String(), "-")
	return edgeDash.ReplaceAllString(s, "")
}
