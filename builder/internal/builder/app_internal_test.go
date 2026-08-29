package builder

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/config"
	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/builder/internal/testsupport"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

func TestWriteFileError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Parent is a file, so MkdirAll fails.
	if err := writeFile(filepath.Join(blocker, "child"), []byte("y")); err == nil {
		t.Error("expected writeFile error under a file")
	}
}

func TestLoadSourcesMissingEach(t *testing.T) {
	dir := t.TempDir()
	sdir := filepath.Join(dir, ".sources")
	if err := os.MkdirAll(sdir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := &App{Dir: dir, Out: os.Stdout}
	cfg := config.Default()

	// Nothing present: offline db load fails.
	if _, err := a.loadSources(cfg); err == nil {
		t.Fatal("expected offline db error")
	}

	// Offline present, anime-list missing.
	write(t, filepath.Join(sdir, cfg.Sources[config.SourceOfflineDatabase].Filename), testsupport.OfflineDBJSON)
	if _, err := a.loadSources(cfg); err == nil {
		t.Fatal("expected anime-list error")
	}

	// Anime-list present, movieset missing.
	write(t, filepath.Join(sdir, cfg.Sources[config.SourceAnimeList].Filename), testsupport.AnimeListXML)
	if _, err := a.loadSources(cfg); err == nil {
		t.Fatal("expected movieset error")
	}

	// All present: success.
	write(t, filepath.Join(sdir, cfg.Sources[config.SourceMovieSetList].Filename), testsupport.MovieSetXML)
	if _, err := a.loadSources(cfg); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A series names a work in Wikidata, so its QID must reach both the fetch (or
// the entity is never downloaded) and the claims group (or it is downloaded
// without the title claim the build reads). A franchise file nests its series
// one level deeper, which is exactly where an id gets missed.
func TestCollectQIDsIncludesSeries(t *testing.T) {
	bundle := overrides.Bundle{Series: []overrides.Override{
		{Series: &model.Series{
			ID:          "sousou-no-frieren",
			ExternalIDs: model.ExternalIDs{WikidataID: "Q98642652"},
			Characters: []model.Character{{
				ID:          "aura",
				ExternalIDs: model.ExternalIDs{WikidataID: "Q123029593"},
			}},
		}},
		{Franchise: &model.Franchise{
			ID: "fate",
			Series: []model.Series{
				{ID: "fate-zero", ExternalIDs: model.ExternalIDs{WikidataID: "Q1"}},
				{ID: "fate-stay-night"}, // no id: contributes nothing, breaks nothing
			},
		}},
	}}

	all := collectQIDs(bundle)
	for _, want := range []string{"Q98642652", "Q123029593", "Q1"} {
		if !slices.Contains(all, want) {
			t.Errorf("collectQIDs missing %s: %v", want, all)
		}
	}

	series := collectSeriesQIDs(bundle)
	if !slices.Equal(series, []string{"Q98642652", "Q1"}) {
		t.Errorf("collectSeriesQIDs = %v, want the series ids only", series)
	}
}
