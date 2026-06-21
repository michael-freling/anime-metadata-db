// Package builder wires the config, sources, build pipeline and writer into the
// three high-level operations the builder CLI exposes: init, build and refresh.
// It is kept free of cobra so the operations are unit-testable with a fake
// fetcher, and is deliberately separate from internal/api (the read-only
// Connect service that serves the dataset this package produces).
package builder

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/michael-freling/anime-metadata-db/internal/build"
	"github.com/michael-freling/anime-metadata-db/internal/config"
	"github.com/michael-freling/anime-metadata-db/internal/fetch"
	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/sources/animelists"
	"github.com/michael-freling/anime-metadata-db/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/sources/wikidata"
	"github.com/michael-freling/anime-metadata-db/internal/writer"
)

// Fetcher downloads a source by URL. *fetch.Client satisfies it; tests inject a
// fake.
type Fetcher interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// App runs the builder operations against a working directory (the repo root).
type App struct {
	Dir     string
	Fetcher Fetcher
	Out     io.Writer
}

// New returns an App rooted at dir. A nil fetcher defaults to a real HTTP
// client; a nil writer defaults to os.Stdout.
func New(dir string, fetcher Fetcher, out io.Writer) *App {
	if fetcher == nil {
		fetcher = fetch.NewClient(nil)
	}
	if out == nil {
		out = os.Stdout
	}
	return &App{Dir: dir, Fetcher: fetcher, Out: out}
}

// configPath is the path to the repo's config.yaml.
func (a *App) configPath() string { return filepath.Join(a.Dir, "config.yaml") }

// loadConfig loads config.yaml, falling back to the built-in defaults when the
// file does not exist yet.
func (a *App) loadConfig() (config.Config, error) {
	if _, err := os.Stat(a.configPath()); os.IsNotExist(err) {
		return config.Default(), nil
	}
	return config.Load(a.configPath())
}

// Init downloads the pinned sources into the cache, recording checksums for any
// source that was not yet pinned, and writes config.yaml.
func (a *App) Init(ctx context.Context) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	sourcesDir := filepath.Join(a.Dir, cfg.Settings.SourcesDir)
	for _, name := range config.SourceNames() {
		src := cfg.Sources[name]
		status, err := a.ensureSource(ctx, sourcesDir, &src)
		if err != nil {
			return err
		}
		cfg.Sources[name] = src
		switch status {
		case sourcePinned:
			fmt.Fprintf(a.Out, "pinned %s @ %s\n", name, shortSHA(src.SHA256))
		case sourceRepinned:
			fmt.Fprintf(a.Out, "re-pinned %s @ %s (rolling source %q changed upstream)\n", name, shortSHA(src.SHA256), src.Version)
		default:
			fmt.Fprintf(a.Out, "verified %s\n", name)
		}
	}
	if err := a.ensureWikidata(ctx, cfg); err != nil {
		return err
	}
	if err := cfg.Save(a.configPath()); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "init complete")
	return nil
}

// ensureWikidata fetches Wikidata labels for every QID referenced by the
// character overrides into the source cache. It is a no-op when the source is
// unconfigured or no character override references a QID.
func (a *App) ensureWikidata(ctx context.Context, cfg config.Config) error {
	src, ok := cfg.Sources[config.SourceWikidata]
	if !ok || src.URL == "" {
		return nil
	}
	bundle, err := overrides.LoadDir(filepath.Join(a.Dir, cfg.Settings.OverridesDir))
	if err != nil {
		return err
	}
	qids := collectQIDs(bundle)
	if len(qids) == 0 {
		return nil
	}
	raw, entities, err := wikidata.FetchLabels(ctx, a.Fetcher.Get, src.URL, qids)
	if err != nil {
		return err
	}
	if err := writeFile(a.wikidataCachePath(cfg), raw); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "fetched wikidata: %d/%d entities\n", entities.Len(), len(qids))
	return nil
}

// collectQIDs gathers every Wikidata QID referenced by the overrides: character
// and per-appearance ids in the series files, and staff ids in the staff files.
func collectQIDs(bundle overrides.Bundle) []string {
	var qids []string
	add := func(id string) {
		if id != "" {
			qids = append(qids, id)
		}
	}
	for _, o := range bundle.Series {
		for _, c := range o.Cast() {
			add(c.ExternalIDs.WikidataID)
			for _, ap := range c.Appearances {
				add(ap.ExternalIDs.WikidataID)
			}
		}
	}
	for _, o := range bundle.Staff {
		for _, s := range o.Staff {
			add(s.ExternalIDs.WikidataID)
		}
	}
	return qids
}

// sourceStatus reports what ensureSource did with a source.
type sourceStatus int

const (
	sourceVerified sourceStatus = iota // present and matching its pin
	sourcePinned                       // downloaded and pinned for the first time
	sourceRepinned                     // a rolling source changed upstream and was re-pinned
)

// rollingRefs are version strings that name a moving target rather than an
// immutable release, so a pinned checksum is advisory (it will legitimately
// change when upstream updates) instead of a hard integrity gate.
var rollingRefs = map[string]bool{"latest": true, "master": true, "main": true, "head": true}

// isRollingVersion reports whether a source version names a moving ref.
func isRollingVersion(version string) bool {
	return rollingRefs[strings.ToLower(strings.TrimSpace(version))]
}

// shortSHA truncates a hex checksum for display.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// ensureSource makes the cache file present and consistent with its pin. A
// cache hit against the pin is a no-op. Otherwise it downloads and, on a
// checksum mismatch, re-pins rolling sources (with a warning) but fails sources
// pinned to a fixed version so tampering is still caught.
func (a *App) ensureSource(ctx context.Context, dir string, src *config.Source) (sourceStatus, error) {
	path := filepath.Join(dir, src.Filename)
	if data, err := os.ReadFile(path); err == nil && src.SHA256 != "" {
		if fetch.Checksum(data) == src.SHA256 {
			return sourceVerified, nil
		}
	}
	data, err := a.Fetcher.Get(ctx, src.URL)
	if err != nil {
		return sourceVerified, err
	}
	sum := fetch.Checksum(data)
	var status sourceStatus
	switch {
	case src.SHA256 == "":
		status = sourcePinned
	case sum == src.SHA256:
		status = sourceVerified
	case isRollingVersion(src.Version):
		status = sourceRepinned
	default:
		return sourceVerified, fmt.Errorf("source %s: checksum mismatch (pinned %s, downloaded %s); run `builder refresh` to update the pin",
			src.Filename, shortSHA(src.SHA256), shortSHA(sum))
	}
	src.SHA256 = sum
	if err := writeFile(path, data); err != nil {
		return sourceVerified, err
	}
	return status, nil
}

// Refresh re-downloads every source to its latest version, bumps the pinned
// checksums, then rebuilds all of data/.
func (a *App) Refresh(ctx context.Context) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	sourcesDir := filepath.Join(a.Dir, cfg.Settings.SourcesDir)
	for _, name := range config.SourceNames() {
		src := cfg.Sources[name]
		data, err := a.Fetcher.Get(ctx, src.URL)
		if err != nil {
			return err
		}
		path := filepath.Join(sourcesDir, src.Filename)
		if err := writeFile(path, data); err != nil {
			return err
		}
		src.SHA256 = fetch.Checksum(data)
		cfg.Sources[name] = src
		fmt.Fprintf(a.Out, "refreshed %s @ %s\n", name, shortSHA(src.SHA256))
	}
	if err := a.ensureWikidata(ctx, cfg); err != nil {
		return err
	}
	if err := cfg.Save(a.configPath()); err != nil {
		return err
	}
	return a.build(cfg, nil)
}

// Build resolves the overrides into data/. With ids given, only those
// franchise/series ids are (re)built; otherwise all are. Files are written only
// when their content changes.
func (a *App) Build(_ context.Context, ids ...string) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	return a.build(cfg, ids)
}

// resolvedSeries pairs an override with its resolved record and report.
type resolvedSeries struct {
	o      overrides.Override
	rec    model.Record
	report *build.Report
}

// resolvedStaff pairs a staff override with its resolved record and report.
type resolvedStaff struct {
	o      overrides.StaffOverride
	rec    model.StaffRecord
	report *build.Report
}

// build is the shared body of Build and Refresh. It resolves staff and series
// records (the latter carry their co-located cast), validates every character's
// references against the full R1 id universe + declared staff, then writes the
// changed files. Everything is resolved even under a filter so references always
// validate against the whole tree; only matching files are written.
func (a *App) build(cfg config.Config, ids []string) error {
	sources, err := a.loadSources(cfg)
	if err != nil {
		return err
	}
	bundle, err := overrides.LoadDir(filepath.Join(a.Dir, cfg.Settings.OverridesDir))
	if err != nil {
		return err
	}
	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	builder := build.New(sources)
	dataDir := filepath.Join(a.Dir, cfg.Settings.DataDir)
	expected := make(map[string]bool, len(bundle.Series)+len(bundle.Staff))

	// Staff first: resolve names and collect the staff id universe.
	staffIDs := map[string]bool{}
	staffOut := make([]resolvedStaff, 0, len(bundle.Staff))
	for _, o := range bundle.Staff {
		expected[filepath.FromSlash(o.Path)] = true
		rec, report, err := builder.BuildStaff(o)
		if err != nil {
			return fmt.Errorf("build %s: %w", o.Path, err)
		}
		for _, s := range rec.Staff {
			staffIDs[s.ID] = true
		}
		staffOut = append(staffOut, resolvedStaff{o, rec, report})
	}

	// Series: resolve structure + cast names, collecting the R1 id universe.
	idx := build.NewIDIndex()
	seriesOut := make([]resolvedSeries, 0, len(bundle.Series))
	for _, o := range bundle.Series {
		expected[filepath.FromSlash(o.Path)] = true
		rec, report, err := builder.Build(o)
		if err != nil {
			return fmt.Errorf("build %s: %w", o.ID(), err)
		}
		idx.Collect(rec)
		seriesOut = append(seriesOut, resolvedSeries{o, rec, report})
	}

	// Validate every cast against the full id universe, then write what matches.
	ctx := build.CharacterContext{R1: idx, Staff: staffIDs}
	updated := 0
	for _, s := range seriesOut {
		if err := build.ValidateCharacters(s.rec.Cast(), ctx); err != nil {
			return fmt.Errorf("build %s: %w", s.o.ID(), err)
		}
		if matchesAny(filter, s.o.IDs()) {
			wrote, err := writer.WriteIfChanged(dataDir, s.o.Path, s.rec)
			if err != nil {
				return err
			}
			updated += a.reportBuilt(wrote, s.o.Path, s.o.ID(), s.report)
		}
	}
	for _, s := range staffOut {
		if matchesAny(filter, s.o.IDs()) {
			wrote, err := writer.WriteStaffIfChanged(dataDir, s.o.Path, s.rec)
			if err != nil {
				return err
			}
			updated += a.reportBuilt(wrote, s.o.Path, s.o.Path, s.report)
		}
	}

	if len(filter) > 0 {
		for id := range filter {
			if !knownID(bundle, id) {
				return fmt.Errorf("build: no override found for id %q", id)
			}
		}
	}

	// A full build owns the whole data tree: remove generated files (and now-empty
	// directories) whose override was deleted or moved, so data/ never keeps a
	// stale record. A filtered build only touches the requested ids.
	if len(filter) == 0 {
		removed, err := pruneData(dataDir, expected)
		if err != nil {
			return err
		}
		for _, rel := range removed {
			fmt.Fprintf(a.Out, "removed orphaned %s\n", rel)
		}
		updated += len(removed)
	}

	fmt.Fprintf(a.Out, "build complete: %d file(s) updated\n", updated)
	return nil
}

// reportBuilt prints the built/report lines for one record and returns 1 if it
// was written (for the updated counter), else 0.
func (a *App) reportBuilt(wrote bool, path, label string, report *build.Report) int {
	if wrote {
		fmt.Fprintf(a.Out, "built %s\n", path)
	}
	if !report.Empty() {
		fmt.Fprintf(a.Out, "report for %s (low-confidence guesses):\n%s", label, report.String())
	}
	if wrote {
		return 1
	}
	return 0
}

// matchesAny reports whether any of ids is selected by the filter (no filter
// means all).
func matchesAny(filter map[string]bool, ids []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, id := range ids {
		if filter[id] {
			return true
		}
	}
	return false
}

// knownID reports whether id names any series/franchise, character or staff
// declared in the bundle.
func knownID(bundle overrides.Bundle, id string) bool {
	for _, o := range bundle.Series {
		for _, oid := range o.IDs() {
			if oid == id {
				return true
			}
		}
	}
	for _, o := range bundle.Staff {
		for _, sid := range o.IDs() {
			if sid == id {
				return true
			}
		}
	}
	return false
}

// pruneData deletes every *.yaml under dataDir whose relative path is not in
// expected, then removes any directories left empty. It returns the relative
// paths that were removed. A missing dataDir is a no-op.
func pruneData(dataDir string, expected map[string]bool) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(dataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return err
		}
		if expected[rel] {
			return nil
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove orphaned data file: %w", err)
		}
		removed = append(removed, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("prune data dir: %w", err)
	}
	if len(removed) > 0 {
		removeEmptyDirs(dataDir)
	}
	sort.Strings(removed)
	return removed, nil
}

// removeEmptyDirs removes empty subdirectories under root (deepest first),
// leaving root itself in place. Best-effort: errors are ignored.
func removeEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	// Deepest paths last lexically is not guaranteed, so remove by descending
	// length, which removes children before parents.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		_ = os.Remove(dir) // fails (and is skipped) when the directory is non-empty
	}
}

// loadSources loads the cached open-data sources, pointing the user at `init`
// when a source is missing.
func (a *App) loadSources(cfg config.Config) (build.Sources, error) {
	dir := filepath.Join(a.Dir, cfg.Settings.SourcesDir)
	offPath := filepath.Join(dir, cfg.Sources[config.SourceOfflineDatabase].Filename)
	off, err := offlinedb.Load(offPath)
	if err != nil {
		return build.Sources{}, fmt.Errorf("%w (run `builder init`)", err)
	}
	al, err := animelists.LoadAnimeList(filepath.Join(dir, cfg.Sources[config.SourceAnimeList].Filename))
	if err != nil {
		return build.Sources{}, fmt.Errorf("%w (run `builder init`)", err)
	}
	msl, err := animelists.LoadMovieSetList(filepath.Join(dir, cfg.Sources[config.SourceMovieSetList].Filename))
	if err != nil {
		return build.Sources{}, fmt.Errorf("%w (run `builder init`)", err)
	}
	sources := build.Sources{Offline: off, AnimeList: al, MovieSets: msl}

	// Wikidata (R2 names) is optional: load it when the cache exists, otherwise
	// leave it nil and let the character build report unfilled names.
	if wdPath := a.wikidataCachePath(cfg); wdPath != "" {
		if _, err := os.Stat(wdPath); err == nil {
			wd, err := wikidata.Load(wdPath)
			if err != nil {
				return build.Sources{}, err
			}
			sources.Wikidata = wd
		}
	}
	return sources, nil
}

// wikidataCachePath returns the cache path for the Wikidata source, or "" when
// the source is not configured.
func (a *App) wikidataCachePath(cfg config.Config) string {
	src, ok := cfg.Sources[config.SourceWikidata]
	if !ok || src.Filename == "" {
		return ""
	}
	return filepath.Join(a.Dir, cfg.Settings.SourcesDir, src.Filename)
}

// writeFile writes data to path, creating parent directories.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
