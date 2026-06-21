package animelists

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const animeListSample = `<?xml version="1.0"?>
<anime-list>
  <anime anidbid="14353" tvdbid="361069" defaulttvdbseason="1" episodeoffset="0"/>
  <anime anidbid="16182" tvdbid="movie" defaulttvdbseason="a" episodeoffset="26"/>
  <anime anidbid="0" tvdbid="123"/>
</anime-list>`

const movieSetSample = `<?xml version="1.0"?>
<anime-movieset-list>
  <set name="Set A">
    <anime anidbid="15183"/>
    <anime anidbid="18000"/>
    <anime anidbid="0"/>
  </set>
  <set name="Empty"/>
</anime-movieset-list>`

func TestParseAnimeList(t *testing.T) {
	al, err := ParseAnimeList(strings.NewReader(animeListSample))
	if err != nil {
		t.Fatal(err)
	}
	if al.Len() != 2 {
		t.Fatalf("expected 2 mappings, got %d", al.Len())
	}
	m, ok := al.Offset(14353)
	if !ok || m.TvdbID != 361069 || m.DefaultTvdbSeason != 1 || m.EpisodeOffset != 0 {
		t.Errorf("unexpected mapping: %+v ok=%v", m, ok)
	}
	// Non-numeric tvdbid/season degrade to 0.
	m2, ok := al.Offset(16182)
	if !ok || m2.TvdbID != 0 || m2.DefaultTvdbSeason != 0 || m2.EpisodeOffset != 26 {
		t.Errorf("unexpected placeholder mapping: %+v", m2)
	}
	if _, ok := al.Offset(999); ok {
		t.Error("did not expect 999")
	}
}

func TestParseAnimeListError(t *testing.T) {
	if _, err := ParseAnimeList(strings.NewReader("<not-closed")); err == nil {
		t.Error("expected decode error")
	}
}

func TestLoadAnimeList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anime-list.xml")
	if err := os.WriteFile(path, []byte(animeListSample), 0o644); err != nil {
		t.Fatal(err)
	}
	al, err := LoadAnimeList(path)
	if err != nil {
		t.Fatal(err)
	}
	if al.Len() != 2 {
		t.Errorf("Len = %d", al.Len())
	}
	if _, err := LoadAnimeList(filepath.Join(dir, "missing.xml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseMovieSetList(t *testing.T) {
	msl, err := ParseMovieSetList(strings.NewReader(movieSetSample))
	if err != nil {
		t.Fatal(err)
	}
	if msl.Len() != 1 {
		t.Fatalf("expected 1 non-empty set, got %d", msl.Len())
	}
	set, ok := msl.SetFor(15183)
	if !ok || set.Name != "Set A" || len(set.AnidbIDs) != 2 {
		t.Errorf("unexpected set: %+v ok=%v", set, ok)
	}
	if _, ok := msl.SetFor(404); ok {
		t.Error("did not expect 404 in any set")
	}
}

func TestParseMovieSetListError(t *testing.T) {
	if _, err := ParseMovieSetList(strings.NewReader("<bad")); err == nil {
		t.Error("expected decode error")
	}
}

func TestLoadMovieSetList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movieset.xml")
	if err := os.WriteFile(path, []byte(movieSetSample), 0o644); err != nil {
		t.Fatal(err)
	}
	msl, err := LoadMovieSetList(path)
	if err != nil {
		t.Fatal(err)
	}
	if msl.Len() != 1 {
		t.Errorf("Len = %d", msl.Len())
	}
	if _, err := LoadMovieSetList(filepath.Join(dir, "missing.xml")); err == nil {
		t.Error("expected error for missing file")
	}
}
