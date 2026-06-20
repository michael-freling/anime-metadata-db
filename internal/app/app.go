// Package app wires the config, sources, build pipeline and writer into the
// three high-level operations the CLI exposes: init, build and refresh. It is
// kept free of cobra so the operations are unit-testable with a fake fetcher.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/michael-freling/anime-metadata-db/internal/build"
	"github.com/michael-freling/anime-metadata-db/internal/config"
	"github.com/michael-freling/anime-metadata-db/internal/fetch"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/sources/animelists"
	"github.com/michael-freling/anime-metadata-db/internal/sources/offlinedb"
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
		recorded, err := a.ensureSource(ctx, sourcesDir, &src)
		if err != nil {
			return err
		}
		cfg.Sources[name] = src
		if recorded {
			fmt.Fprintf(a.Out, "pinned %s @ %s\n", name, src.SHA256[:12])
		} else {
			fmt.Fprintf(a.Out, "verified %s\n", name)
		}
	}
	if err := cfg.Save(a.configPath()); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "init complete")
	return nil
}

// ensureSource makes the cache file present and consistent with its pin. It
// downloads when the file is missing or fails its pinned checksum, records the
// checksum the first time a source is pinned, and reports whether it did so.
func (a *App) ensureSource(ctx context.Context, dir string, src *config.Source) (bool, error) {
	path := filepath.Join(dir, src.Filename)
	if data, err := os.ReadFile(path); err == nil && src.SHA256 != "" {
		if fetch.Checksum(data) == src.SHA256 {
			return false, nil
		}
	}
	data, err := a.Fetcher.Get(ctx, src.URL)
	if err != nil {
		return false, err
	}
	sum := fetch.Checksum(data)
	if src.SHA256 != "" && sum != src.SHA256 {
		return false, fmt.Errorf("source %s: checksum mismatch (pinned %s, downloaded %s)", src.Filename, src.SHA256, sum)
	}
	recorded := src.SHA256 == ""
	src.SHA256 = sum
	if err := writeFile(path, data); err != nil {
		return false, err
	}
	return recorded, nil
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
		fmt.Fprintf(a.Out, "refreshed %s @ %s\n", name, src.SHA256[:12])
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

// build is the shared body of Build and Refresh.
func (a *App) build(cfg config.Config, ids []string) error {
	sources, err := a.loadSources(cfg)
	if err != nil {
		return err
	}
	ovs, err := overrides.LoadDir(filepath.Join(a.Dir, cfg.Settings.OverridesDir))
	if err != nil {
		return err
	}
	filter := make(map[string]bool, len(ids))
	for _, id := range ids {
		filter[id] = true
	}

	builder := build.New(sources)
	dataDir := filepath.Join(a.Dir, cfg.Settings.DataDir)
	updated := 0
	for _, o := range ovs {
		if len(filter) > 0 && !filter[o.ID()] {
			continue
		}
		rec, report, err := builder.Build(o)
		if err != nil {
			return fmt.Errorf("build %s: %w", o.ID(), err)
		}
		wrote, err := writer.WriteIfChanged(dataDir, o.Path, rec)
		if err != nil {
			return err
		}
		if wrote {
			updated++
			fmt.Fprintf(a.Out, "built %s\n", o.Path)
		}
		if !report.Empty() {
			fmt.Fprintf(a.Out, "report for %s (low-confidence guesses):\n%s", o.ID(), report.String())
		}
	}
	if len(filter) > 0 {
		for id := range filter {
			if !containsID(ovs, id) {
				return fmt.Errorf("build: no override found for id %q", id)
			}
		}
	}
	fmt.Fprintf(a.Out, "build complete: %d file(s) updated\n", updated)
	return nil
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
	return build.Sources{Offline: off, AnimeList: al, MovieSets: msl}, nil
}

// containsID reports whether any override has the given id.
func containsID(ovs []overrides.Override, id string) bool {
	for _, o := range ovs {
		if o.ID() == id {
			return true
		}
	}
	return false
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
