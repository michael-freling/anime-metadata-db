//go:build e2e

package app

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michael-freling/anime-metadata-db/internal/config"
	"github.com/michael-freling/anime-metadata-db/internal/fetch"
)

// e2eOverride is a minimal numbered series referencing a stable, popular AniList
// id (Demon Slayer season 1) that is present in the real offline database.
const e2eOverride = `series:
  id: demon-slayer
  seasons:
    - id: demon-slayer-s1
      number: 1
      externalIds: { anilistId: 101922 }
numbered: [demon-slayer]
`

// e2eCharacters references the series above plus real Wikidata QIDs (Tanjirō
// Kamado and his voice actor), exercising the live Wikidata fetch + R2 build.
const e2eCharacters = `staff:
  - id: natsuki-hanae
    externalIds: { wikidataId: Q2596113 }
characters:
  - id: tanjiro-kamado
    externalIds: { wikidataId: Q85805158 }
    voiceActors:
      - { staffId: natsuki-hanae, language: ja }
    appearances:
      - seriesId: demon-slayer
        scope:
          - { seasonId: demon-slayer-s1 }
`

// TestE2EInitAndBuild downloads the real, pinned open-data sources over the
// network (no mocks) and runs the full init -> build pipeline against them.
//
// It is the regression guard for stale source URLs: a moved or renamed upstream
// file — like the 404 when the offline database moved from the repo tree to
// GitHub release assets — fails this test at Init. The mock-based unit tests
// can't catch that because they never resolve the real URLs.
//
// Build-tagged `e2e` so it stays out of the default `go test ./...` run (and the
// coverage gate); invoke it explicitly with `go test -tags e2e ./...`.
func TestE2EInitAndBuild(t *testing.T) {
	dir := t.TempDir()
	writeFileE2E(t, filepath.Join(dir, "config", "overrides", "series", "demon-slayer.yaml"), e2eOverride)
	writeFileE2E(t, filepath.Join(dir, "config", "overrides", "characters", "demon-slayer.yaml"), e2eCharacters)

	var out bytes.Buffer
	// A real HTTP client, with a timeout so a stall fails instead of hanging.
	client := fetch.NewClient(&http.Client{Timeout: 10 * time.Minute})
	a := New(dir, client, &out)
	ctx := context.Background()

	// Init downloads the real sources from the default (committed) URLs.
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init failed (is a source URL stale?): %v", err)
	}
	cfg := config.Default()
	for _, name := range config.SourceNames() {
		path := filepath.Join(dir, cfg.Settings.SourcesDir, cfg.Sources[name].Filename)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("source %q was not cached: %v", name, err)
		}
		if info.Size() < 1024 {
			t.Errorf("source %q looks too small (%d bytes) — download may be broken", name, info.Size())
		}
	}

	// Build resolves the override against the real data.
	if err := a.Build(ctx); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, cfg.Settings.DataDir, "series", "demon-slayer.yaml"))
	if err != nil {
		t.Fatalf("data not written: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "id: demon-slayer") {
		t.Errorf("generated data is missing the series id:\n%s", got)
	}
	if !strings.Contains(got, "absoluteNumber: 1") {
		t.Errorf("generated data is missing a computed absoluteNumber:\n%s", got)
	}

	// R2: the characters file must have names filled live from Wikidata.
	chars, err := os.ReadFile(filepath.Join(dir, cfg.Settings.DataDir, "characters", "demon-slayer.yaml"))
	if err != nil {
		t.Fatalf("characters data not written: %v", err)
	}
	cs := string(chars)
	for _, want := range []string{"id: tanjiro-kamado", "竈門炭治郎", "staffId: natsuki-hanae", "花江夏樹"} {
		if !strings.Contains(cs, want) {
			t.Errorf("characters data missing %q (Wikidata fetch/build broken?):\n%s", want, cs)
		}
	}
}

func writeFileE2E(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
