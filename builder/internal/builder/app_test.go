package builder

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/config"
	"github.com/michael-freling/anime-metadata-db/builder/internal/testsupport"
)

// newRepo creates a temp repo with the Demon Slayer override and returns its dir.
func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config", "overrides", "series", "demon-slayer.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(testsupport.DemonSlayerOverride), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func newApp(t *testing.T, dir string, f Fetcher) (*App, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	return New(dir, f, &out), &out
}

func TestInitThenBuild(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()

	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out.String(), "pinned offlineDatabase") {
		t.Errorf("init output missing pin line: %q", out.String())
	}
	// Sources and config exist now.
	if _, err := os.Stat(filepath.Join(dir, ".sources", "anime-offline-database.json")); err != nil {
		t.Errorf("source not cached: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Errorf("config not written: %v", err)
	}

	out.Reset()
	if err := a.Build(ctx); err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "data", "series", "demon-slayer.yaml"))
	if err != nil {
		t.Fatalf("data not written: %v", err)
	}
	if !strings.Contains(string(data), "absoluteNumber: 34") {
		t.Errorf("expected Infinity film numbered 34:\n%s", data)
	}

	// Re-build is idempotent: nothing changes.
	out.Reset()
	if err := a.Build(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0 file(s) updated") {
		t.Errorf("expected no changes on rebuild: %q", out.String())
	}
}

func TestInitVerifiesOnSecondRun(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "verified offlineDatabase") {
		t.Errorf("second init should verify, got: %q", out.String())
	}
}

func TestInitChecksumMismatch(t *testing.T) {
	dir := newRepo(t)
	// Pre-write a config pinning a wrong checksum.
	cfgYAML := "sources:\n" +
		"  offlineDatabase: { url: \"http://x/offline\", filename: db.json, sha256: deadbeef }\n" +
		"  animeList: { url: \"http://x/anime-list\", filename: al.xml }\n" +
		"  movieSetList: { url: \"http://x/movieset\", filename: ms.xml }\n" +
		"settings: { sourcesDir: .sources, overridesDir: overrides, dataDir: data }\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	if err := a.Init(context.Background()); err == nil {
		t.Error("expected checksum mismatch error")
	}
}

func TestInitRepinsRollingSource(t *testing.T) {
	dir := newRepo(t)
	// Pin offlineDatabase to a wrong checksum but keep its rolling "latest"
	// version: init must re-pin (with a warning), not fail.
	cfg := config.Default()
	s := cfg.Sources[config.SourceOfflineDatabase]
	s.SHA256 = "deadbeef"
	cfg.Sources[config.SourceOfflineDatabase] = s
	if err := cfg.Save(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}

	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("init should re-pin a rolling source, not fail: %v", err)
	}
	if !strings.Contains(out.String(), "re-pinned offlineDatabase") {
		t.Errorf("expected a re-pin message, got: %q", out.String())
	}
	got, err := config.Load(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources[config.SourceOfflineDatabase].SHA256 == "deadbeef" {
		t.Error("rolling source checksum should have been updated")
	}
}

func TestInitReverifiesAfterCacheCleared(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	// Drop the cache but keep the freshly pinned config; re-init must
	// re-download and verify against the recorded pin without re-pinning.
	if err := os.RemoveAll(filepath.Join(dir, ".sources")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "verified offlineDatabase") {
		t.Errorf("expected verified after re-download, got: %q", out.String())
	}
}

func TestInitFetchError(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{Err: errors.New("boom")})
	if err := a.Init(context.Background()); err == nil {
		t.Error("expected fetch error")
	}
}

func TestBuildWithoutSources(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	err := a.Build(context.Background())
	if err == nil || !strings.Contains(err.Error(), "builder init") {
		t.Errorf("expected a 'run builder init' error, got %v", err)
	}
}

func TestBuildFilterAndUnknownID(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx, "demon-slayer"); err != nil {
		t.Fatalf("filtered build: %v", err)
	}
	if err := a.Build(ctx, "ghost"); err == nil {
		t.Error("expected error for unknown id")
	}
}

func TestBuildNoOverridesDir(t *testing.T) {
	dir := t.TempDir() // no overrides/
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := a.Build(ctx); err != nil {
		t.Fatalf("build with no overrides should succeed: %v", err)
	}
	if !strings.Contains(out.String(), "0 file(s) updated") {
		t.Errorf("unexpected output: %q", out.String())
	}
}

func TestBuildError(t *testing.T) {
	dir := t.TempDir()
	// Override references an AniList id absent from the fixtures.
	path := filepath.Join(dir, "config", "overrides", "bad.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "series:\n  id: bad\n  seasons:\n    - id: x\n      number: 1\n      externalIds: { anilistId: 1 }\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx); err == nil {
		t.Error("expected build error for unknown id")
	}
}

func TestBuildPrunesOrphans(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx); err != nil {
		t.Fatal(err)
	}

	// Simulate a stale generated file left behind by a moved/deleted override
	// (e.g. data/franchises/fate.yaml after fate moved to data/series/).
	orphan := filepath.Join(dir, "data", "franchises", "fate.yaml")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("franchise: {id: stale}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := a.Build(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphaned data file should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(orphan)); !os.IsNotExist(err) {
		t.Errorf("emptied orphan directory should have been removed")
	}
	if !strings.Contains(out.String(), "removed orphaned franchises/fate.yaml") {
		t.Errorf("expected removal to be reported, got: %q", out.String())
	}
	// The real data file is still present.
	if _, err := os.Stat(filepath.Join(dir, "data", "series", "demon-slayer.yaml")); err != nil {
		t.Errorf("live data file should remain: %v", err)
	}

	// A filtered build must NOT prune.
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("franchise: {id: stale}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx, "demon-slayer"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("filtered build must not prune orphans: %v", err)
	}
}

// writeFileAt writes content to a path under the repo's config/overrides.
func writeFileAt(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, "config", "overrides", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newCharacterRepo writes a merged series file (structure + cast) plus a global
// staff file.
func newCharacterRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFileAt(t, dir, "series/demon-slayer.yaml", testsupport.DemonSlayerMerged)
	writeFileAt(t, dir, "staff/voice-actors.yaml", testsupport.StaffOverride)
	return dir
}

func TestInitBuildCharacters(t *testing.T) {
	dir := newCharacterRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()

	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out.String(), "fetched wikidata") {
		t.Errorf("init should fetch wikidata: %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".sources", "wikidata-entities.json")); err != nil {
		t.Errorf("wikidata cache not written: %v", err)
	}

	if err := a.Build(ctx); err != nil {
		t.Fatalf("build: %v", err)
	}
	// Cast is co-located in the series file; staff in its own file.
	series, err := os.ReadFile(filepath.Join(dir, "data", "series", "demon-slayer.yaml"))
	if err != nil {
		t.Fatalf("series data not written: %v", err)
	}
	// The default appearance (the enclosing series) is filled in.
	for _, want := range []string{"id: tanjiro-kamado", "竈門炭治郎", "staffId: natsuki-hanae", "characters:", "seriesId: demon-slayer"} {
		if !strings.Contains(string(series), want) {
			t.Errorf("series data missing %q:\n%s", want, series)
		}
	}
	staff, err := os.ReadFile(filepath.Join(dir, "data", "staff", "voice-actors.yaml"))
	if err != nil {
		t.Fatalf("staff data not written: %v", err)
	}
	for _, want := range []string{"id: natsuki-hanae", "花江夏樹"} {
		if !strings.Contains(string(staff), want) {
			t.Errorf("staff data missing %q:\n%s", want, staff)
		}
	}
}

func TestBuildFilterWithCharacters(t *testing.T) {
	dir := newCharacterRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	seriesData := filepath.Join(dir, "data", "series", "demon-slayer.yaml")
	staffData := filepath.Join(dir, "data", "staff", "voice-actors.yaml")

	// Filter by a character id -> its series file is built (cast is co-located).
	if err := a.Build(ctx, "tanjiro-kamado"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(seriesData); err != nil {
		t.Errorf("filtering by character id should build its series file: %v", err)
	}

	// Filter by a staff id -> only the staff file is built.
	if err := os.Remove(seriesData); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx, "natsuki-hanae"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staffData); err != nil {
		t.Errorf("filtering by staff id should build the staff file: %v", err)
	}
	if _, err := os.Stat(seriesData); !os.IsNotExist(err) {
		t.Error("staff filter should not rebuild the series file")
	}

	// Unknown id across all layers -> error.
	if err := a.Build(ctx, "ghost"); err == nil {
		t.Error("expected unknown-id error")
	}
}

func TestBuildCharactersUnknownSeries(t *testing.T) {
	dir := newRepo(t)
	// A character (nested in a valid series) whose appearance references a series
	// that doesn't exist.
	merged := `series:
  id: demon-slayer
  seasons:
    - id: ds-s1
      number: 1
      externalIds: { anilistId: 101922 }
  characters:
    - id: ghost-char
      appearances:
        - seriesId: no-such-series
`
	writeFileAt(t, dir, "series/demon-slayer.yaml", merged)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx); err == nil {
		t.Error("expected build error for unknown series reference")
	}
}

func TestRefresh(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Refresh(ctx); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if !strings.Contains(out.String(), "refreshed offlineDatabase") {
		t.Errorf("refresh output missing: %q", out.String())
	}
	// Refresh rebuilds, so data exists.
	if _, err := os.Stat(filepath.Join(dir, "data", "series", "demon-slayer.yaml")); err != nil {
		t.Errorf("refresh did not build data: %v", err)
	}
}

func TestRefreshFetchError(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{FailURL: "anime-list"})
	if err := a.Refresh(context.Background()); err == nil {
		t.Error("expected refresh fetch error")
	}
}

func TestBadConfig(t *testing.T) {
	dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("sources: [oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err == nil {
		t.Error("Init should fail on bad config")
	}
	if err := a.Build(ctx); err == nil {
		t.Error("Build should fail on bad config")
	}
	if err := a.Refresh(ctx); err == nil {
		t.Error("Refresh should fail on bad config")
	}
}

func TestNewDefaults(t *testing.T) {
	a := New("/tmp/x", nil, nil)
	if a.Fetcher == nil {
		t.Error("nil fetcher should default to a real client")
	}
	if a.Out != os.Stdout {
		t.Error("nil writer should default to os.Stdout")
	}
}

// A full build owns data/, so a misconfiguration that resolves the overrides to
// an empty tree makes every committed record look orphaned. That is not
// hypothetical: after the Go module split, config.yaml's overridesDir pointed at
// a directory that no longer held the overrides, and `builder build` reported
// "removed orphaned ..." 151 times, deleted the whole dataset and exited 0.
//
// Nothing to build must never mean delete everything.
func TestBuildRefusesToPruneEverythingWhenNoOverridesResolve(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Build(ctx); err != nil {
		t.Fatalf("first build: %v", err)
	}
	built, err := filepath.Glob(filepath.Join(dir, "data", "series", "*.yaml"))
	if err != nil || len(built) == 0 {
		t.Fatalf("the first build produced no records to protect (%v)", err)
	}

	// The overrides resolve to nothing — a wrong overridesDir, a moved tree, a
	// bad --dir. The records in data/ are now unaccounted for.
	if err := os.RemoveAll(filepath.Join(dir, "config", "overrides")); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	err = a.Build(ctx)
	if err == nil {
		t.Fatal("build succeeded with no overrides; it should refuse rather than prune")
	}
	if !strings.Contains(err.Error(), "yielded no overrides at all") {
		t.Errorf("error = %v, want it to name the empty overrides directory", err)
	}
	if strings.Contains(out.String(), "removed orphaned") {
		t.Errorf("records were pruned before the refusal:\n%s", out.String())
	}
	for _, path := range built {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Errorf("%s was deleted despite the refusal: %v", filepath.Base(path), statErr)
		}
	}
}

// The guard must not fire on the legitimate case it resembles: a repository
// with no overrides and no data yet. There is nothing to lose, so the build
// should simply do nothing.
func TestBuildWithNoOverridesAndNoDataSucceeds(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "config", "overrides")); err != nil {
		t.Fatal(err)
	}
	if err := a.Build(ctx); err != nil {
		t.Errorf("build with nothing to build and nothing to lose: %v", err)
	}
}

// The dangerous case is not only an empty overrides directory. A path that
// resolves to some *other* tree — an old snapshot, a sibling with a couple of
// files, a half-finished move — yields a non-empty bundle, so a guard that only
// checks for zero overrides waves it through and the prune still eats most of
// the dataset.
func TestBuildRefusesWhenItWouldDeleteMoreThanItWrites(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Build(ctx); err != nil {
		t.Fatalf("first build: %v", err)
	}
	// Records the real overrides do not account for — as if data/ had been
	// built from a larger tree than the one now being read.
	for _, name := range []string{"ghost-a", "ghost-b", "ghost-c"} {
		mustWrite(t, filepath.Join(dir, "data", "series", name+".yaml"), "series:\n  id: "+name+"\n")
	}
	out.Reset()

	err := a.Build(ctx)
	if err == nil {
		t.Fatal("build pruned more than it wrote without refusing")
	}
	if !strings.Contains(err.Error(), "--allow-prune") {
		t.Errorf("error = %v, want it to point at the escape hatch", err)
	}
	for _, name := range []string{"ghost-a", "ghost-b", "ghost-c"} {
		if _, statErr := os.Stat(filepath.Join(dir, "data", "series", name+".yaml")); statErr != nil {
			t.Errorf("%s was deleted despite the refusal", name)
		}
	}

	// And the escape hatch works, because deliberately removing a lot of series
	// is a real thing to want.
	a.AllowPrune = true
	if err := a.Build(ctx); err != nil {
		t.Fatalf("build with --allow-prune: %v", err)
	}
	for _, name := range []string{"ghost-a", "ghost-b", "ghost-c"} {
		if _, statErr := os.Stat(filepath.Join(dir, "data", "series", name+".yaml")); statErr == nil {
			t.Errorf("%s survived a build that was explicitly allowed to prune", name)
		}
	}
}

// Pruning a few records because their overrides were removed is ordinary and
// must not need a flag.
func TestBuildPrunesASingleOrphanWithoutComplaint(t *testing.T) {
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Build(ctx); err != nil {
		t.Fatalf("first build: %v", err)
	}
	mustWrite(t, filepath.Join(dir, "data", "series", "ghost.yaml"), "series:\n  id: ghost\n")
	out.Reset()

	if err := a.Build(ctx); err != nil {
		t.Fatalf("pruning one orphan should not need a flag: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "series", "ghost.yaml")); err == nil {
		t.Error("the orphan was not pruned")
	}
	if !strings.Contains(out.String(), "removed orphaned series/ghost.yaml") {
		t.Errorf("the prune was not reported:\n%s", out.String())
	}
}

// mustWrite creates a file and its parents, failing the test on error.
func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
