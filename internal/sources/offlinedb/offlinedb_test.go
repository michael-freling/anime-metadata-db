package offlinedb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `{
  "data": [
    {
      "sources": ["https://anilist.co/anime/101922", "https://anidb.net/anime/14353", "https://myanimelist.net/anime/38000", "https://kitsu.app/anime/41370"],
      "title": "Kimetsu no Yaiba",
      "type": "TV",
      "episodes": 26,
      "animeSeason": { "season": "SPRING", "year": 2019 },
      "synonyms": ["鬼滅の刃"]
    },
    {
      "sources": ["https://myanimelist.net/anime/99999"],
      "title": "No AniList"
    },
    {
      "sources": ["https://anilist.co/anime/not-a-number"],
      "title": "Malformed id"
    }
  ]
}`

func TestParseAndLookup(t *testing.T) {
	db, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if db.Len() != 1 {
		t.Fatalf("expected 1 indexed entry, got %d", db.Len())
	}
	a, ok := db.Lookup(101922)
	if !ok {
		t.Fatal("expected to find 101922")
	}
	if a.Title != "Kimetsu no Yaiba" || a.Episodes != 26 || a.Type != TypeTV {
		t.Errorf("unexpected entry: %+v", a)
	}
	if a.AnilistID() != 101922 {
		t.Errorf("AnilistID = %d", a.AnilistID())
	}
	if a.AnidbID() != 14353 {
		t.Errorf("AnidbID = %d", a.AnidbID())
	}
	if a.MyAnimeListID() != 38000 {
		t.Errorf("MyAnimeListID = %d", a.MyAnimeListID())
	}
	if a.KitsuID() != 41370 {
		t.Errorf("KitsuID = %d", a.KitsuID())
	}
	if _, ok := db.Lookup(404); ok {
		t.Error("did not expect to find 404")
	}
}

func TestProviderIDAbsent(t *testing.T) {
	a := Anime{Sources: []string{"https://example.com/anime/1"}}
	if a.AnilistID() != 0 {
		t.Errorf("expected 0 for missing provider, got %d", a.AnilistID())
	}
}

func TestParseError(t *testing.T) {
	if _, err := Parse(strings.NewReader("{not json")); err == nil {
		t.Error("expected decode error")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "db.json")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if db.Len() != 1 {
		t.Errorf("Len = %d", db.Len())
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("expected error for missing file")
	}
}
