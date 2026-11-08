package builder

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/importer"
)

// Import writes override files for one year of the upstream catalogue.
//
// It writes the authored layer only and never runs the build, so the result is
// reviewable as a diff of overrides before any record is generated from it:
// run `builder build` afterwards. With dryRun it reports what it would write
// and touches nothing.
func (a *App) Import(year int, dryRun bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	sources, err := a.loadSources(cfg)
	if err != nil {
		return err
	}
	overridesDir := filepath.Join(a.Dir, cfg.Settings.OverridesDir, "series")
	dataDir := filepath.Join(a.Dir, cfg.Settings.DataDir, "series")

	known, err := knownAnilistIDs(dataDir)
	if err != nil {
		return err
	}
	// An empty set would make every series the catalogue already has look new,
	// so the import would write a duplicate of each. Refuse rather than
	// produce that: an empty data/ is a misconfigured path far more often than
	// it is a genuinely empty catalogue.
	if len(known) == 0 {
		return fmt.Errorf("no anilistIds found under %s: refusing to import, "+
			"since every existing series would be duplicated (is dataDir right?)", dataDir)
	}

	// Every id in the dataset shares one namespace — a series, a season and a
	// film cannot collide — so a new slug has to be checked against all of
	// them, not just against the override filenames. The build refuses a
	// duplicate outright, and it is right to: two records answering to one id
	// is an ambiguity the API cannot resolve.
	takenIDs, err := existingIDs(filepath.Join(a.Dir, cfg.Settings.OverridesDir))
	if err != nil {
		return err
	}

	plan := importer.Build(sources.Offline, year, known)

	fmt.Fprintf(a.Out, "import %d: %d new series, %d to merge by hand, %d unresolvable, %d already present, %d out of scope\n",
		plan.Year, len(plan.New), len(plan.Merges), len(plan.Unresolvable), len(plan.Present), plan.Skipped)

	if len(plan.Unresolvable) > 0 {
		fmt.Fprintf(a.Out, "\nNOT written — the build could not have filled these. Author by hand:\n")
		for _, u := range plan.Unresolvable {
			fmt.Fprintf(a.Out, "  %-46s %s\n", trim(u.Title, 46), u.Reason)
		}
		fmt.Fprintln(a.Out)
	}

	if len(plan.Merges) > 0 {
		fmt.Fprintf(a.Out, "\nNOT written — these belong to a series the catalogue already has.\n")
		fmt.Fprintf(a.Out, "Author the merge by adding the installment to the existing override:\n")
		for _, m := range plan.Merges {
			fmt.Fprintf(a.Out, "  %-62s anilist=%d -> related to %v\n", trim(m.Title, 62), m.AnilistID, m.Into)
		}
		fmt.Fprintln(a.Out)
	}

	var wrote, skipped int
	var collisions []string
	for _, s := range plan.New {
		path := filepath.Join(overridesDir, s.ID+".yaml")
		// Never overwrite: an existing file is authored work, and a collision
		// means two series slugged the same, which is a judgement call rather
		// than something to resolve by clobbering.
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(a.Out, "exists, left alone: %s\n", s.ID)
			skipped++
			continue
		}
		if clash := firstTaken(takenIDs, s); clash != "" {
			collisions = append(collisions, fmt.Sprintf("%-46s id %q is already used", trim(s.Romanized, 46), clash))
			skipped++
			continue
		}
		if dryRun {
			wrote++
			continue
		}
		if err := os.MkdirAll(overridesDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(importer.Render(s, year)), 0o644); err != nil {
			return err
		}
		wrote++
	}
	if len(collisions) > 0 {
		fmt.Fprintf(a.Out, "\nNOT written — the id is already taken elsewhere in the dataset.\n")
		fmt.Fprintf(a.Out, "Give the series a distinguishing id by hand, or merge it if it is the same work:\n")
		for _, c := range collisions {
			fmt.Fprintf(a.Out, "  %s\n", c)
		}
		fmt.Fprintln(a.Out)
	}

	verb := "wrote"
	if dryRun {
		verb = "would write"
	}
	fmt.Fprintf(a.Out, "%s %d override files under %s", verb, wrote, overridesDir)
	if skipped > 0 {
		fmt.Fprintf(a.Out, " (%d left alone, already present)", skipped)
	}
	fmt.Fprintln(a.Out)
	if !dryRun && wrote > 0 {
		fmt.Fprintln(a.Out, "next: `builder build` to fill the facts, then `make index`")
	}
	return nil
}

var (
	anilistLine = regexp.MustCompile(`(?m)^\s*anilistId:\s*(\d+)\s*$`)
	idLine      = regexp.MustCompile(`(?m)^\s*(?:-\s+)?id:\s*(\S+)\s*$`)
)

// existingIDs collects every id the authored layer declares, at any level.
//
// Read from the overrides rather than data/ because the overrides are where ids
// are authored: data/ holds the same ids, but a record whose override was just
// deleted would still be there and would block an id that is genuinely free.
func existingIDs(overridesDir string) (map[string]bool, error) {
	taken := map[string]bool{}
	err := filepath.WalkDir(overridesDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range idLine.FindAllSubmatch(body, -1) {
			taken[string(m[1])] = true
		}
		return nil
	})
	if os.IsNotExist(err) {
		return taken, nil
	}
	return taken, err
}

// firstTaken returns the first id of s that is already in use, or "".
func firstTaken(taken map[string]bool, s importer.Series) string {
	ids := append([]string{s.ID}, s.Seasons...)
	ids = append(ids, s.Movies...)
	for _, id := range ids {
		if taken[id] {
			return id
		}
	}
	return ""
}

// knownAnilistIDs collects every anilistId in the generated records.
//
// Read from data/ rather than from the overrides because the ids are resolved
// by the build, not authored: an override usually carries none at all, so the
// authored layer cannot answer what the catalogue already covers.
func knownAnilistIDs(dataDir string) (map[int]bool, error) {
	known := map[int]bool{}
	entries, err := os.ReadDir(dataDir)
	if os.IsNotExist(err) {
		return known, nil
	}
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
		// Matched by line rather than decoded: the ids are all that is wanted,
		// and parsing every record costs seconds per run for nothing else.
		for _, m := range anilistLine.FindAllSubmatch(body, -1) {
			var id int
			if _, err := fmt.Sscanf(string(m[1]), "%d", &id); err == nil && id > 0 {
				known[id] = true
			}
		}
	}
	return known, nil
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
