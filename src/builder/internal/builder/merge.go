package builder

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/importer"
	"github.com/michael-freling/anime-metadata-db/src/builder/internal/sources/offlinedb"
)

// Merge attaches the works `builder import` reported as belonging to a series
// the catalogue already has.
//
// The import refuses to write these, and is right to: creating a second series
// for an installment of an existing one is the only import mistake nothing
// downstream can detect. But refusing them is not the same as resolving them,
// and 2,134 accumulated — the largest gap in the catalogue.
//
// What it writes is bounded by what the evidence settles, not by what could be
// guessed:
//
//   - The work must carry an AniList id. An installment upstream does not list
//     under the series' own title can only be reached by an authored id, and
//     there is nothing to author without one.
//   - Upstream's links must name exactly one series the catalogue has. A work
//     linked to two is a question about which franchise owns it, and that is a
//     person's call.
//   - A film is appended as a film. A film node carries no number, so the only
//     claim being made is that the work belongs to the series — which is what
//     upstream's own link says.
//   - A season is appended only when the work aired after every installment the
//     series already has. Then the claim is "a later installment", which is
//     what a trailing season is. Anything that lands inside or before the
//     existing run would be asserting a position in the middle of it, so it is
//     reported instead.
//
// Nothing here decides whether a work is a recap, a compilation or a side
// story — upstream types all of those as TV or MOVIE when it has no better
// box, and the catalogue's scope rule already admits them. What it does
// guarantee is that an applied merge makes no claim beyond membership and
// order, and the build's own gates then check the membership: a work upstream
// does not link to the rest of the series fails `anilist-unlinked`, and one
// that another series already accounts for fails `anilist-claimed-elsewhere`.
func (a *App) Merge(dryRun bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	sources, err := a.loadSources(cfg)
	if err != nil {
		return err
	}
	overridesDir := filepath.Join(a.Dir, cfg.Settings.OverridesDir)
	dataDir := filepath.Join(a.Dir, cfg.Settings.DataDir, "series")

	known, err := knownAnilistIDs(dataDir)
	if err != nil {
		return err
	}
	if len(known) == 0 {
		return fmt.Errorf("no anilistIds found under %s: refusing to merge into an empty catalogue "+
			"(is dataDir right?)", dataDir)
	}
	authored, err := declaredAnilistIDs(overridesDir)
	if err != nil {
		return err
	}
	for id := range authored {
		known[id] = true
	}
	owner, err := seriesByAnilistID(dataDir)
	if err != nil {
		return err
	}
	titlesOf, err := seriesTitles(dataDir)
	if err != nil {
		return err
	}
	taken, err := existingIDs(overridesDir)
	if err != nil {
		return err
	}
	claimedTitles, err := declaredTitles(overridesDir)
	if err != nil {
		return err
	}

	// Collected across every year before anything is written, because two
	// candidates can land on one series and the second has to see what the
	// first added — otherwise both take the same node id and the build refuses
	// the pair as duplicates.
	type action struct {
		m      importer.Merge
		series string
	}
	var apply []action
	refused := map[string]int{}
	seen := map[int]bool{}
	years := importYears(owner, sources.Offline)
	for _, y := range years {
		for _, m := range importer.Build(sources.Offline, y, known, claimedTitles).Merges {
			if m.AnilistID == 0 {
				refused["carries no AniList id, so there is nothing to author"]++
				continue
			}
			if seen[m.AnilistID] {
				continue
			}
			seen[m.AnilistID] = true
			if !m.Linked {
				// Found by sharing a title with something the catalogue has,
				// which is also how two unrelated works of the same name are
				// found. The build rejects the pairing — it did, for "Tokyo"
				// (2017) attached beside "Tokyo" (1999) — so it is not written.
				refused["upstream does not link it to the series; it only shares a title, which is also what a homonym does"]++
				continue
			}
			// Upstream's links say the two are related; they do not say the
			// work is an installment of this series. relatedAnime is not a
			// grouping primitive — one hop from a 2014 series reaches films
			// from 1996 belonging to an earlier part of a franchise the
			// catalogue does not model, and attaching them would claim they
			// are films of that series.
			//
			// So the catalogue's own boundary decides which of the linked
			// series it belongs to, and it is the same question the build
			// asks: does the series' title account for this work? Grouping is
			// by the title the build resolves against. A work whose name is a
			// linked series' name carried further is an installment of it; one
			// that matches none of them, or several, is a judgement call.
			var into []string
			for _, id := range m.Into {
				s, ok := owner[id]
				if !ok {
					continue
				}
				if !slices.Contains(into, s) {
					into = append(into, s)
				}
			}
			if len(into) == 0 {
				refused["upstream's links name no series the catalogue has"]++
				continue
			}
			var named []string
			for _, s := range into {
				if titleExtends(titlesOf[s], m, sources.Offline) {
					named = append(named, s)
				}
			}
			switch len(named) {
			case 1:
			case 0:
				refused["its title is not the title of any series upstream links it to, so whether it is an installment or just related is a judgement call"]++
				continue
			default:
				refused["its title extends more than one linked series' title, so which one owns it is a judgement call"]++
				continue
			}
			apply = append(apply, action{m: m, series: named[0]})
		}
	}
	sort.Slice(apply, func(i, j int) bool {
		if apply[i].series != apply[j].series {
			return apply[i].series < apply[j].series
		}
		return apply[i].m.AnilistID < apply[j].m.AnilistID
	})

	merged, byKind := 0, map[string]int{}
	// Held in memory so a dry run sees what a real one would: two works can
	// land on the same series, and the second has to read what the first
	// added. Reading from disk in dry run reported 250 attachments where the
	// run made 125, because every second attachment re-planned against an
	// unmodified file.
	edited := map[string]string{}
	for _, act := range apply {
		path := filepath.Join(overridesDir, "series", act.series+".yaml")
		body, ok := editedOrRead(edited, path)
		if !ok {
			refused["the series has no override file"]++
			continue
		}
		// The resolved record, not the override: the years a season has to be
		// compared against are where the resolution landed, and an override
		// states them only where they were authored. Reading the override for
		// them found none at all, so the ordering guard passed everything.
		record, err := os.ReadFile(filepath.Join(dataDir, act.series+".yaml"))
		if err != nil {
			refused["the series has no generated record to order against"]++
			continue
		}
		next, why := plan(body, string(record), act.series, act.m, taken)
		if why != "" {
			refused[why]++
			continue
		}
		edited[path] = next
		if !dryRun {
			if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
				return err
			}
		}
		merged++
		byKind[kindOf(act.m)]++
	}

	fmt.Fprintf(a.Out, "merge: %d attached (%d films, %d seasons), %d left for a person\n",
		merged, byKind["film"], byKind["season"], total(refused))
	for _, why := range sortedKeys(refused) {
		fmt.Fprintf(a.Out, "  %5d  %s\n", refused[why], why)
	}
	if dryRun {
		fmt.Fprintln(a.Out, "\n(dry run — nothing written)")
	} else {
		fmt.Fprintln(a.Out, "\nRun `builder build` and then the index; the build gates on whether upstream\nactually links each attached work to the rest of its series.")
	}
	return nil
}

// kindOf names the node a merge becomes.
func kindOf(m importer.Merge) string {
	if m.Type == offlinedb.TypeMovie {
		return "film"
	}
	return "season"
}

var (
	// The block runs to the next key at two-space indent. Comments count as
	// part of it: jujutsu-kaisen opens its films with one, an earlier version
	// of this regex therefore did not see the key, and the node was appended
	// as a second `movies:` that YAML rejects outright.
	seasonsBlock = regexp.MustCompile(`(?m)^  seasons:\n((?:    [-#].*\n|      .*\n|\s*\n)+)`)
	moviesBlock  = regexp.MustCompile(`(?m)^  movies:\n((?:    [-#].*\n|      .*\n|\s*\n)+)`)
	moviesKey    = regexp.MustCompile(`(?m)^  movies:\s*$`)
	seasonsKey   = regexp.MustCompile(`(?m)^  (?:seasons|movies|specials):\s*$`)
	latinLine    = regexp.MustCompile(`(?m)^\s*ja-Latn:\s*(.+?)\s*$`)
	nodeStart    = regexp.MustCompile(`(?m)^    - id: \S+\s*$`)
	nodeIDName   = regexp.MustCompile(`(?m)^\s*(?:-\s+)?id: (\S+)\s*$`)
	nextKey      = regexp.MustCompile(`(?m)^  \S`)
	numberLine   = regexp.MustCompile(`(?m)^      number: (\d+)\s*$`)
	yearLine     = regexp.MustCompile(`(?m)^      releaseYear: (\d+)\s*$`)
)

// plan returns the override with the work attached, or the reason it was not.
//
// The file is edited as text rather than parsed and re-emitted, because the
// authored layer is written by hand as much as by this tool: the comments on a
// series are how its decisions are recorded, and a round trip through the
// model would drop every one of them.
func plan(body, record, series string, m importer.Merge, taken map[string]bool) (string, string) {
	// The file's own node ids count as taken whatever the caller knows. The
	// namespace check is global and comes from the overrides on disk, but
	// relying on that alone means a gap there becomes two nodes answering to
	// one id in the same file, which the build refuses.
	for _, g := range nodeIDName.FindAllStringSubmatch(body, -1) {
		taken[g[1]] = true
	}
	if m.Type == offlinedb.TypeMovie {
		return attachFilm(body, record, series, m, taken)
	}
	return attachSeason(body, record, series, m, taken)
}

// attachFilm appends a film node. A film carries no number, so its position
// makes no claim and the only question is what to call it.
func attachFilm(body, record, series string, m importer.Merge, taken map[string]bool) (string, string) {
	id, ok := freeID(series, "film", taken)
	if !ok {
		return "", "every film id this series could take is already in use"
	}
	node := fmt.Sprintf("    - id: %s\n      externalIds: { anilistId: %d } # %s (%d)\n",
		id, m.AnilistID, comment(m.Title), m.Year)
	if loc := moviesBlock.FindStringSubmatchIndex(body); loc != nil {
		pinned, why := pinIDs(body[loc[2]:loc[3]], resolvedIDs(record, "movies"))
		if why != "" {
			return "", why
		}
		taken[id] = true
		return body[:loc[2]] + pinned + node + body[loc[3]:], ""
	}
	// The key is there but the block did not match, so where it ends is not
	// understood. Refused rather than appended after the seasons, which is how
	// a file gets two `movies:` keys and stops parsing.
	if moviesKey.MatchString(body) {
		return "", "the film list is in a shape this does not know how to extend"
	}
	// No films yet. Added after the seasons where there are some, so the file
	// keeps the order the generated record uses.
	if loc := seasonsBlock.FindStringSubmatchIndex(body); loc != nil {
		taken[id] = true
		return body[:loc[3]] + "  movies:\n" + node + body[loc[3]:], ""
	}
	return "", "the series declares neither seasons nor films, so there is nowhere to put this"
}

// attachSeason appends a season, numbered after the last one, and only when the
// work aired after everything the series already has.
func attachSeason(body, record, series string, m importer.Merge, taken map[string]bool) (string, string) {
	loc := seasonsBlock.FindStringSubmatchIndex(body)
	if loc == nil {
		return "", "the series declares no seasons, so a trailing one cannot be numbered"
	}
	block := body[loc[2]:loc[3]]
	last := 0
	for _, g := range numberLine.FindAllStringSubmatch(block, -1) {
		if n, _ := strconv.Atoi(g[1]); n > last {
			last = n
		}
	}
	if last == 0 {
		return "", "the series' seasons carry no numbers to continue from"
	}
	if m.Year == 0 {
		return "", "upstream gives the work no release year, so it cannot be placed in order"
	}
	// Compared against the generated record's years, which is where the
	// resolution actually landed; the override states them only when authored.
	years := yearLine.FindAllStringSubmatch(record, -1)
	if len(years) == 0 {
		return "", "the generated record states no release years, so order cannot be checked"
	}
	for _, g := range years {
		if y, _ := strconv.Atoi(g[1]); y >= m.Year {
			return "", "it did not air after every installment the series already has, " +
				"so its position in the run is a judgement call"
		}
	}
	id, ok := freeID(series, fmt.Sprintf("s%d", last+1), taken)
	if !ok {
		return "", "the next season id is already in use"
	}
	pinned, why := pinIDs(block, resolvedIDs(record, "seasons"))
	if why != "" {
		return "", why
	}
	node := fmt.Sprintf("    - id: %s\n      number: %d\n      externalIds: { anilistId: %d } # %s (%d)\n",
		id, last+1, m.AnilistID, comment(m.Title), m.Year)
	taken[id] = true
	return body[:loc[2]] + pinned + node + body[loc[3]:], ""
}

// freeID returns the first unused id of the form <series>-<suffix>, suffixed
// again if it has to be. Every id in the dataset shares one namespace, so this
// is checked against all of them and not just this file.
func freeID(series, suffix string, taken map[string]bool) (string, bool) {
	base := series + "-" + suffix
	if !taken[base] {
		return base, true
	}
	for i := 2; i < 100; i++ {
		id := fmt.Sprintf("%s-%d", base, i)
		if !taken[id] {
			return id, true
		}
	}
	return "", false
}

// comment keeps an upstream title readable on one line and out of YAML's way.
func comment(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return trim(s, 64)
}

// seriesByAnilistID maps every AniList id the generated records account for to
// the series that accounts for it. One id cannot belong to two series — the
// build gates on that — so the last writer winning would be a bug rather than
// a choice, and it cannot happen on a dataset that builds.
func seriesByAnilistID(dataDir string) (map[int]string, error) {
	owner := map[int]string{}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dataDir, e.Name()))
		if err != nil {
			return nil, err
		}
		series := strings.TrimSuffix(e.Name(), ".yaml")
		for _, g := range anilistInline.FindAllSubmatch(body, -1) {
			if n, err := strconv.Atoi(string(g[1])); err == nil && n > 0 {
				owner[n] = series
			}
		}
	}
	return owner, nil
}

// importYears returns the years to ask for merge candidates in: every year the
// catalogue already covers, since a work linked to a series it has can only
// have been released in one of them or nearby. Derived rather than written
// down, so it grows with the dataset instead of freezing.
func importYears(owner map[int]string, db *offlinedb.Database) []int {
	seen := map[int]bool{}
	for id := range owner {
		if a, ok := db.Lookup(id); ok && a.AnimeSeason.Year > 0 {
			seen[a.AnimeSeason.Year] = true
		}
	}
	out := make([]int, 0, len(seen))
	for y := range seen {
		out = append(out, y)
	}
	sort.Ints(out)
	return out
}

func total(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if m[out[i]] != m[out[j]] {
			return m[out[i]] > m[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

// titleExtends reports whether the work's name is the series' name carried
// further — "Kimetsu no Yaiba: Hashira Geiko-hen" against "Kimetsu no Yaiba".
//
// Compared folded, for the reason importer.FoldTitle exists, and against every
// name either side has: a series states its native title and a romanisation,
// and upstream often lists an installment under only one of them, or under a
// synonym. Equality counts — a film sharing its series' name exactly is still
// that series' film.
func titleExtends(titles []string, m importer.Merge, db *offlinedb.Database) bool {
	names := []string{m.Title}
	if a, ok := db.Lookup(m.AnilistID); ok {
		names = append(names, a.Title)
		names = append(names, a.Synonyms...)
	}
	for _, t := range titles {
		folded := importer.FoldTitle(t)
		// A one- or two-character series title would match most of the
		// catalogue once folded, so it is not evidence of anything.
		if len([]rune(folded)) < 3 {
			continue
		}
		for _, n := range names {
			if strings.HasPrefix(importer.FoldTitle(n), folded) {
				return true
			}
		}
	}
	return false
}

// seriesTitles maps each generated record to the names the series answers to.
func seriesTitles(dataDir string) (map[string][]string, error) {
	out := map[string][]string{}
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dataDir, e.Name()))
		if err != nil {
			return nil, err
		}
		// The series' own block only: an installment's titles are not the
		// series' name, and matching against them would let any season's
		// subtitle pull in works that merely share it.
		head := body
		if i := seasonsKey.FindIndex(body); i != nil {
			head = body[:i[0]]
		}
		var names []string
		for _, g := range originalLine.FindAllSubmatch(head, -1) {
			if t := unquoteYAML(string(g[1])); t != "" {
				names = append(names, t)
			}
		}
		for _, g := range latinLine.FindAllSubmatch(head, -1) {
			if t := unquoteYAML(string(g[1])); t != "" {
				names = append(names, t)
			}
		}
		out[strings.TrimSuffix(e.Name(), ".yaml")] = names
	}
	return out, nil
}

// pinIDs writes the resolved AniList id onto each slot of a kind that does not
// already state one.
//
// Required by the thing it sits next to rather than chosen. The build refuses a
// kind unless the number of upstream candidates equals the number of declared
// installments, so appending a slot to a kind whose existing ones resolve by
// title breaks every one of them: "ajin" declared one film, upstream lists one
// film under that title, and a second slot left the first with nothing
// resolved and the build stopping on it.
//
// The ids written are the ones the build already resolved, read from the
// generated record, so nothing about the dataset changes except that those
// joins stop depending on a count that the new slot has just altered.
//
// Refuses rather than guesses when the slots and the resolved nodes do not line
// up one to one, since pairing them by position is only sound when they are the
// same list.
func pinIDs(block string, resolved []int) (string, string) {
	slots := nodeStart.FindAllStringIndex(block, -1)
	if len(slots) != len(resolved) {
		return "", "the override's installments and the resolved record's do not line up, so ids cannot be pinned"
	}
	var b strings.Builder
	// Whatever sits before the first node — a comment recording why the kind
	// is shaped the way it is — belongs to the block and is kept.
	if len(slots) > 0 {
		b.WriteString(block[:slots[0][0]])
	}
	for i, at := range slots {
		end := len(block)
		if i+1 < len(slots) {
			end = slots[i+1][0]
		}
		node := block[at[0]:end]
		if strings.Contains(node, "anilistId") {
			b.WriteString(node)
			continue
		}
		if resolved[i] == 0 {
			// Nothing to pin: this installment resolves to an upstream record
			// that carries no AniList id, so it can only ever be reached by
			// the title, and the count a new slot changes is the only thing
			// holding it. "ajin" declares one film and upstream's record for
			// it has no id at all.
			return "", "an existing installment resolves to a record with no AniList id, " +
				"so adding a slot beside it would leave it unresolvable"
		}
		// Appended after the node's own lines, so an authored title or comment
		// on it is untouched.
		trimmed := strings.TrimRight(node, "\n")
		b.WriteString(trimmed)
		b.WriteString(fmt.Sprintf("\n      # Pinned when a later installment was attached: the build matches a\n"+
			"      # kind by candidate count, and an added slot changes it.\n"+
			"      externalIds: { anilistId: %d }\n", resolved[i]))
	}
	return b.String(), ""
}

// resolvedIDs reads the AniList ids a generated record carries for one kind,
// in the order it lists them.
func resolvedIDs(record, kind string) []int {
	loc := kindBlock(record, kind)
	if loc == nil {
		return nil
	}
	block := record[loc[0]:loc[1]]
	slots := nodeStart.FindAllStringIndex(block, -1)
	out := make([]int, len(slots))
	for i, at := range slots {
		end := len(block)
		if i+1 < len(slots) {
			end = slots[i+1][0]
		}
		if g := anilistInline.FindStringSubmatch(block[at[0]:end]); g != nil {
			out[i], _ = strconv.Atoi(g[1])
		}
	}
	return out
}

// kindBlock locates one kind's installments in a generated record.
func kindBlock(record, kind string) []int {
	re := regexp.MustCompile(`(?m)^  ` + kind + `:\n`)
	at := re.FindStringIndex(record)
	if at == nil {
		return nil
	}
	rest := record[at[1]:]
	if next := nextKey.FindStringIndex(rest); next != nil {
		return []int{at[1], at[1] + next[0]}
	}
	return []int{at[1], len(record)}
}

// editedOrRead returns the working copy of an override, which is whatever an
// earlier attachment left behind if there was one.
func editedOrRead(edited map[string]string, path string) (string, bool) {
	if body, ok := edited[path]; ok {
		return body, true
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	edited[path] = string(body)
	return string(body), true
}
