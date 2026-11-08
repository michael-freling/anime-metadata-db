package builder

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/testsupport"
)

// importFixture is an offline database shaped for the import cases: one entry
// the catalogue already has, one that is new, one linked to a known work, one
// whose slug collides with an existing id, and one out of scope.
const importFixture = `{
  "data": [
    {
      "sources": ["https://anilist.co/anime/112151"],
      "title": "Kimetsu no Yaiba: Mugen Ressha-hen",
      "type": "MOVIE", "episodes": 1, "status": "FINISHED",
      "animeSeason": { "season": "FALL", "year": 2020 }
    },
    {
      "sources": ["https://anilist.co/anime/900001"],
      "title": "Brand New Show",
      "type": "TV", "episodes": 12, "status": "FINISHED",
      "animeSeason": { "season": "SPRING", "year": 2020 },
      "synonyms": ["ぶらんどにゅう"]
    },
    {
      "sources": ["https://anilist.co/anime/900002"],
      "title": "Kimetsu no Yaiba: Some New Arc",
      "type": "TV", "episodes": 12, "status": "FINISHED",
      "animeSeason": { "season": "SUMMER", "year": 2020 },
      "relatedAnime": ["https://anilist.co/anime/101922"]
    },
    {
      "sources": ["https://anilist.co/anime/900003"],
      "title": "DS Infinity",
      "type": "TV", "episodes": 12, "status": "FINISHED",
      "animeSeason": { "season": "WINTER", "year": 2020 }
    },
    {
      "sources": ["https://anilist.co/anime/900004"],
      "title": "A Promo Clip",
      "type": "ONA", "episodes": 1, "status": "FINISHED",
      "animeSeason": { "season": "SPRING", "year": 2020 }
    }
  ]
}`

// builtRepo returns a repo whose data/ has been built, so the import has a
// catalogue to compare against.
func builtRepo(t *testing.T) (*App, string, func() string) {
	t.Helper()
	dir := newRepo(t)
	a, out := newApp(t, dir, testsupport.FakeFetcher{})
	ctx := context.Background()
	if err := a.Init(ctx); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := a.Build(ctx); err != nil {
		t.Fatalf("build: %v", err)
	}
	// Swap the cached source for the import fixture. Import reads the file and
	// does not verify the pin, which is what makes this possible.
	src := filepath.Join(dir, ".sources", "anime-offline-database.json")
	if err := os.WriteFile(src, []byte(importFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	return a, dir, out.String
}

func TestImportWritesNewSeriesAndRefusesTheRest(t *testing.T) {
	a, dir, output := builtRepo(t)

	if err := a.Import(2020, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := output()

	// Written: the one series that is new, resolvable and free of collisions.
	path := filepath.Join(dir, "config", "overrides", "series", "brand-new-show.yaml")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected an override for the new series: %v\n%s", err, got)
	}
	for _, want := range []string{"id: brand-new-show", "original: ぶらんどにゅう", "ja-Latn: Brand New Show", "- id: brand-new-show-s1"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("override missing %q:\n%s", want, body)
		}
	}

	// Already in the catalogue: skipped, and no second record for it.
	if _, err := os.Stat(filepath.Join(dir, "config", "overrides", "series", "kimetsu-no-yaiba-mugen-ressha-hen.yaml")); err == nil {
		t.Error("wrote an override for a work the catalogue already has")
	}

	// Linked to a known work: reported for a hand merge, never written.
	if !strings.Contains(got, "belong to a series the catalogue already has") {
		t.Errorf("no merge report:\n%s", got)
	}
	if !strings.Contains(got, "Kimetsu no Yaiba: Some New Arc") {
		t.Errorf("merge report does not name the linked work:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "overrides", "series", "kimetsu-no-yaiba-some-new-arc.yaml")); err == nil {
		t.Error("wrote an override for an installment of an existing series")
	}

	// "DS Infinity" slugs to ds-infinity, which the committed Demon Slayer
	// override already uses for a FILM. Every id in the dataset shares one
	// namespace, so a series slug has to be free against works too — and only
	// the id check can see that, since no file is named for it.
	if !strings.Contains(got, "id is already taken") {
		t.Errorf("no collision report:\n%s", got)
	}
	if !strings.Contains(got, `"ds-infinity"`) {
		t.Errorf("collision report does not name the id:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "overrides", "series", "ds-infinity.yaml")); err == nil {
		t.Error("wrote a series whose id is already in use")
	}

	// The out-of-scope ONA is counted, not written.
	if !strings.Contains(got, "1 out of scope") {
		t.Errorf("out-of-scope count missing:\n%s", got)
	}
}

func TestImportDryRunWritesNothing(t *testing.T) {
	a, dir, output := builtRepo(t)

	if err := a.Import(2020, true); err != nil {
		t.Fatalf("import: %v", err)
	}
	if !strings.Contains(output(), "would write 1 override") {
		t.Errorf("dry run did not say what it would do:\n%s", output())
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "overrides", "series", "brand-new-show.yaml")); !os.IsNotExist(err) {
		t.Error("a dry run wrote a file")
	}
}

func TestImportLeavesAnExistingFileAlone(t *testing.T) {
	a, dir, output := builtRepo(t)
	path := filepath.Join(dir, "config", "overrides", "series", "brand-new-show.yaml")
	if err := os.WriteFile(path, []byte("# authored by hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Import(2020, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Authored work is never clobbered: a collision is a judgement call, not
	// something to resolve by overwriting.
	if string(body) != "# authored by hand\n" {
		t.Errorf("overwrote an existing override:\n%s", body)
	}
	if !strings.Contains(output(), "exists, left alone") {
		t.Errorf("no note that the file was left alone:\n%s", output())
	}
}

// TestImportRefusesWithNoCatalogueToCompareAgainst covers the guard that
// matters most. With no anilistIds to read, every series the catalogue already
// has looks new and the import would write a duplicate of each — and an empty
// data/ is a misconfigured path far more often than an empty catalogue.
func TestImportRefusesWithNoCatalogueToCompareAgainst(t *testing.T) {
	dir := newRepo(t)
	a, _ := newApp(t, dir, testsupport.FakeFetcher{})
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Deliberately not built, so data/ holds nothing.
	err := a.Import(2020, false)
	if err == nil {
		t.Fatal("import succeeded with an empty catalogue")
	}
	if !strings.Contains(err.Error(), "every existing series would be duplicated") {
		t.Errorf("err = %v, want it to explain the refusal", err)
	}
}

func TestImportReportsASeriesTheBuildCouldNotFill(t *testing.T) {
	a, dir, output := builtRepo(t)
	// Two films under one title: a film carries no number, so the build
	// declines the kind and nothing resolves.
	src := filepath.Join(dir, ".sources", "anime-offline-database.json")
	// Related to each other on purpose: they are two parts of one work, so the
	// refusal under test is the one about ordering films, not the one about
	// unrelated works sharing a name.
	if err := os.WriteFile(src, []byte(`{"data":[
      {"sources":["https://anilist.co/anime/910001"],"title":"Twin Bill",
       "type":"MOVIE","episodes":1,"status":"FINISHED",
       "animeSeason":{"season":"WINTER","year":2020},
       "relatedAnime":["https://anilist.co/anime/910002"]},
      {"sources":["https://anilist.co/anime/910002"],"title":"Twin Bill Part Two",
       "type":"MOVIE","episodes":1,"status":"FINISHED",
       "animeSeason":{"season":"SUMMER","year":2020},"synonyms":["Twin Bill"],
       "relatedAnime":["https://anilist.co/anime/910001"]}
    ]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.Import(2020, false); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := output()
	if !strings.Contains(got, "could not have filled these") {
		t.Errorf("no unresolvable report:\n%s", got)
	}
	if !strings.Contains(got, "no number") {
		t.Errorf("report does not say why:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config", "overrides", "series", "twin-bill.yaml")); err == nil {
		t.Error("wrote a series the build could not have filled")
	}
}
