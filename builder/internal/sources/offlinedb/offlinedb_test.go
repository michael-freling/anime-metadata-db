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

// relatedAnime mixes providers, and only AniList ids can be compared against
// this dataset's own. A url for another provider, or one with no numeric tail,
// is skipped rather than parsed into a wrong id.
func TestRelatedAnilistIDsSkipsOtherProviders(t *testing.T) {
	const mixed = `{"data":[{"sources":["https://anilist.co/anime/1"],"title":"x","type":"TV","episodes":1,
	  "relatedAnime":[
	    "https://anidb.net/anime/18886",
	    "https://anilist.co/anime/182255",
	    "https://myanimelist.net/anime/52991",
	    "https://anime-planet.com/anime/frieren-beyond-journeys-end",
	    "https://anilist.co/anime/169811"
	  ]}]}`
	db, err := Parse(strings.NewReader(mixed))
	if err != nil {
		t.Fatal(err)
	}
	a, ok := db.Lookup(1)
	if !ok {
		t.Fatal("entry not indexed")
	}
	got := a.RelatedAnilistIDs()
	want := []int{182255, 169811}
	if len(got) != len(want) {
		t.Fatalf("got %v, want only the AniList ids %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v (in url order)", got, want)
		}
	}
}

// An entry upstream links to nothing has no related ids, and must not produce
// an empty-but-non-nil slice that reads as "looked and found none" downstream.
func TestRelatedAnilistIDsWithoutRelatedAnime(t *testing.T) {
	db, err := Parse(strings.NewReader(`{"data":[{"sources":["https://anilist.co/anime/1"],"title":"x","type":"TV","episodes":1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := db.Lookup(1)
	if got := a.RelatedAnilistIDs(); len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

// Titled used to sort its result by AniList id and promise that order. It no
// longer does — the only caller merges several calls into a set and re-orders
// the survivors by airing date — so what it does promise is worth pinning:
// every entry carrying the name, in upstream's own order, which is fixed for a
// given database and therefore still reproducible.
func TestTitledReturnsEveryMatchInUpstreamOrder(t *testing.T) {
	const db = `{"data":[
	  {"sources":["https://anilist.co/anime/900"],"title":"Later Season","synonyms":["Shared Name"]},
	  {"sources":["https://anilist.co/anime/100"],"title":"Shared Name","synonyms":[]},
	  {"sources":["https://anilist.co/anime/500"],"title":"Other","synonyms":["Shared Name","Alias"]},
	  {"sources":["https://anidb.net/anime/7"],"title":"No AniList Id","synonyms":["Shared Name"]}
	]}`
	d, err := Parse(strings.NewReader(db))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// 900 first, then 100, then 500: the file's order, not ascending id. An
	// ascending sort would put 100 first, so this fails if the sort returns.
	got := d.Titled("Shared Name")
	want := []int{900, 100, 500}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].AnilistID() != id {
			t.Fatalf("entry %d: got id %d, want %d", i, got[i].AnilistID(), id)
		}
	}
	// Indexing by id must still return the whole entry, not an id-shaped hole.
	if got[2].Title != "Other" {
		t.Errorf("the entry itself comes back, got title %q", got[2].Title)
	}

	// An entry with no AniList id is not indexed at all, so it cannot be found
	// by a name it happens to share — there would be no id to join it on.
	for _, a := range got {
		if a.Title == "No AniList Id" {
			t.Error("an entry with no AniList id must not be indexed by title")
		}
	}

	// A title is indexed as well as the synonyms.
	if byTitle := d.Titled("Other"); len(byTitle) != 1 || byTitle[0].AnilistID() != 500 {
		t.Errorf("an entry is findable by its own title, got %+v", byTitle)
	}
	if none := d.Titled("Not A Title Anywhere"); none != nil {
		t.Errorf("an unknown name finds nothing, got %+v", none)
	}
}
