package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/config"
	"github.com/michael-freling/anime-metadata-db/internal/testsupport"
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
