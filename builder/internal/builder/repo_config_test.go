package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/config"
	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
)

// moduleRoot is the builder module's directory — where `builder --dir` points
// by default and what every path in config.yaml is resolved against.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The checked-in config.yaml has to describe the repository it actually lives
// in. Every other test here builds in a synthetic temp directory, so nothing
// looked at the real one — and that gap cost the dataset once: after the Go
// module split, sourcesDir and dataDir still pointed at a repository root that
// no longer held the builder, and a full build deleted every record in data/ as
// orphaned while exiting 0.
//
// The builder now refuses that prune, and the e2e workflow rebuilds data/ and
// diffs it. This is the cheap half: it needs no network, runs in the ordinary
// test job, and fails the moment a path stops resolving — which is the form the
// regression actually takes.
func TestCommittedConfigResolvesOntoThisRepository(t *testing.T) {
	root := moduleRoot(t)
	cfg, err := config.Load(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("load the committed config.yaml: %v", err)
	}

	// The overrides are the input. If this resolves to an empty or missing
	// directory, a full build treats the entire dataset as orphaned — the
	// single most destructive misconfiguration available here.
	overridesDir := filepath.Join(root, cfg.Settings.OverridesDir)
	series, err := filepath.Glob(filepath.Join(overridesDir, "series", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(series) == 0 {
		t.Errorf("overridesDir %q resolves to %s, which holds no series overrides; "+
			"a full build would prune every record in data/",
			cfg.Settings.OverridesDir, overridesDir)
	}

	// The dataset is the output, and it is committed, so it must already be
	// there to be rewritten in place.
	dataDir := filepath.Join(root, cfg.Settings.DataDir)
	records, err := filepath.Glob(filepath.Join(dataDir, "series", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 {
		t.Errorf("dataDir %q resolves to %s, which holds no records", cfg.Settings.DataDir, dataDir)
	}

	// Overrides and data must not be the same tree: the builder writes the one
	// and reads the other, and pointing both at one directory would have it
	// overwrite its own inputs.
	if absEqual(t, overridesDir, dataDir) {
		t.Errorf("overridesDir and dataDir both resolve to %s", dataDir)
	}

	// The source cache is gitignored, so its contents cannot be asserted — but
	// the directory it would be created in has to exist, or `builder init`
	// writes the cache somewhere nothing will look for it.
	sourcesParent := filepath.Dir(filepath.Join(root, cfg.Settings.SourcesDir))
	if info, err := os.Stat(sourcesParent); err != nil || !info.IsDir() {
		t.Errorf("sourcesDir %q resolves under %s, which is not a directory (%v)",
			cfg.Settings.SourcesDir, sourcesParent, err)
	}
}

// absEqual reports whether two paths name the same location.
func absEqual(t *testing.T, a, b string) bool {
	t.Helper()
	pa, err := filepath.Abs(a)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := filepath.Abs(b)
	if err != nil {
		t.Fatal(err)
	}
	return pa == pb
}

// The committed overrides must also be loadable, not merely present: a parse
// error would leave the bundle empty, which is the same input state as a wrong
// path and would reach the same prune.
func TestCommittedOverridesLoad(t *testing.T) {
	root := moduleRoot(t)
	cfg, err := config.Load(filepath.Join(root, "config.yaml"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	bundle, err := overrides.LoadDir(filepath.Join(root, cfg.Settings.OverridesDir))
	if err != nil {
		t.Fatalf("load the committed overrides: %v", err)
	}
	if len(bundle.Series) == 0 {
		t.Error("the committed overrides parsed to no series")
	}
	if len(bundle.Staff) == 0 {
		t.Error("the committed overrides parsed to no staff files")
	}
}
