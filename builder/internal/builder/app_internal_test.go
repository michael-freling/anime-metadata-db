package builder

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/builder/internal/build"
	"github.com/michael-freling/anime-metadata-db/builder/internal/config"
	"github.com/michael-freling/anime-metadata-db/builder/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/builder/internal/sources/wikidata"
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

// resolvingFetcher answers the two different Wikidata requests a full init
// makes: the title→work resolution, and the entity fetch for what it found.
type resolvingFetcher struct{ resolve, entities string }

func (f resolvingFetcher) Get(_ context.Context, url string) ([]byte, error) {
	if strings.Contains(url, "sites=jawiki") {
		return []byte(f.resolve), nil
	}
	return []byte(f.entities), nil
}

// A series names its work by resolution, not by an authored id, so init has to
// look every unresolved title up, carry what it found into the cache, and say
// what it could not resolve — a title matching no article and a title matching
// a disambiguation page are different problems with different fixes, and
// neither is visible from the dataset afterwards.
func TestEnsureWikidataResolvesSeriesWorks(t *testing.T) {
	dir := t.TempDir()
	seriesDir := filepath.Join(dir, "config", "overrides", "series")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(seriesDir, "s.yaml"), `series:
  id: sousou-no-frieren
  titles: { original: 葬送のフリーレン }
`)
	write(t, filepath.Join(seriesDir, "t.yaml"), `series:
  id: mao
  titles: { original: マオ }
`)
	write(t, filepath.Join(seriesDir, "u.yaml"), `series:
  id: black-clover
  titles: { original: ブラッククローバー }
`)
	// An authored id is not re-resolved: the override has already answered.
	write(t, filepath.Join(seriesDir, "v.yaml"), `series:
  id: authored
  titles: { original: 既知 }
  externalIds: { wikidataId: Q7 }
`)

	fetcher := resolvingFetcher{
		resolve: `{"entities":{
		  "Q98642652":{"sitelinks":{"jawiki":{"title":"葬送のフリーレン"}},
		    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}},
		  "Q11339901":{"sitelinks":{"jawiki":{"title":"マオ"}},
		    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q4167410"}}}}]}}}}`,
		entities: `{"entities":{"Q98642652":{"id":"Q98642652",
		  "labels":{"en":{"language":"en","value":"Frieren"}}}}}`,
	}
	var out bytes.Buffer
	a := &App{Dir: dir, Fetcher: fetcher, Out: &out}
	cfg := config.Default()
	if err := a.ensureWikidata(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	for _, want := range []string{
		"resolved wikidata works: 1/3 series titles",
		"1 unresolved: a disambiguation page, not a work",
		"1 unresolved: no Japanese Wikipedia article with this exact title",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// 既知 carries an authored id, so it was never a resolution target.
	if strings.Contains(got, "1/4") {
		t.Errorf("an authored id must not be re-resolved:\n%s", got)
	}

	// The resolution has to reach the cache, or an offline build could not use it.
	raw, err := os.ReadFile(a.wikidataCachePath(cfg))
	if err != nil {
		t.Fatal(err)
	}
	ents, err := wikidata.Parse(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if qid := ents.QIDForTitle("葬送のフリーレン"); qid != "Q98642652" {
		t.Errorf("cache lost the resolution: %q", qid)
	}
	if qid := ents.QIDForTitle("マオ"); qid != "" {
		t.Errorf("a disambiguation page must not be cached as a work: %q", qid)
	}
}

// Two series sharing a native title mean the title identifies neither, so
// neither is resolved. Resolving would hand both the same work and one of them
// the other's English title, with the tally still reading as if all had
// resolved — the failure mode worth refusing outright.
func TestResolveSeriesWorksRefusesSharedTitles(t *testing.T) {
	dir := t.TempDir()
	seriesDir := filepath.Join(dir, "config", "overrides", "series")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(seriesDir, "a.yaml"), "series:\n  id: remake\n  titles: { original: 同じ題 }\n")
	write(t, filepath.Join(seriesDir, "b.yaml"), "series:\n  id: original-run\n  titles: { original: 同じ題 }\n")
	write(t, filepath.Join(seriesDir, "c.yaml"), "series:\n  id: unique\n  titles: { original: 葬送のフリーレン }\n")

	fetcher := resolvingFetcher{
		resolve: `{"entities":{"Q98642652":{"sitelinks":{"jawiki":{"title":"葬送のフリーレン"}},
		  "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}},
		  "Q5":{"sitelinks":{"jawiki":{"title":"同じ題"}},
		  "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}}}}`,
		entities: `{"entities":{}}`,
	}
	var out bytes.Buffer
	a := &App{Dir: dir, Fetcher: fetcher, Out: &out}
	resolved, err := a.resolveSeriesWorks(context.Background(), "https://x/api.php", mustLoadOverrides(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := resolved["同じ題"]; ok {
		t.Error("a shared title must not resolve, even though the lookup would have answered")
	}
	if resolved["葬送のフリーレン"] != "Q98642652" {
		t.Errorf("the unique title must still resolve: %v", resolved)
	}
	got := out.String()
	// Counted against all three, so the figure cannot read as complete.
	if !strings.Contains(got, "resolved wikidata works: 1/3 series titles") {
		t.Errorf("tally must count the skipped titles:\n%s", got)
	}
	if !strings.Contains(got, "shared by 2 series (remake, original-run)") {
		t.Errorf("the refusal must name the colliding series:\n%s", got)
	}
}

// mustLoadOverrides reads the overrides a test just wrote.
func mustLoadOverrides(t *testing.T, dir string) overrides.Bundle {
	t.Helper()
	b, err := overrides.LoadDir(filepath.Join(dir, "config", "overrides"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The coverage line is the only thing that tells a reader whether an empty
// report means "clean" or "nothing was looked at", and nothing asserted its
// wording — the e2e gate greps for a prefix, which a rename silently breaks.
func TestReportCoverage(t *testing.T) {
	render := func(c build.Coverage) string {
		var out bytes.Buffer
		(&App{Out: &out}).reportCoverage(c)
		return out.String()
	}

	// Nothing considered: no line at all, rather than "0/0", which reads as a
	// result when it is an absence.
	if got := render(build.Coverage{}); got != "" {
		t.Errorf("no ids means no line, got %q", got)
	}

	got := render(build.Coverage{Authored: 10, Corroborated: 4, Alone: 5, Agreed: 7})
	for _, want := range []string{
		"anilistId corroboration: 10 authored",
		"7/10 agree with the entry the series' title resolves to",
		"4/10 linked to a sibling installment",
		"5 unverifiable: the only installment of their series",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Every fraction shares the denominator, so two independent checks are not
	// reported as if one were a subset of the other.
	if strings.Contains(got, "/9") || strings.Contains(got, "/7") {
		t.Errorf("a figure used a denominator of its own:\n%s", got)
	}

	// The derived line appears only when something was derived: on a catalogue
	// that authors every id it would otherwise print a permanent "0 resolved".
	if strings.Contains(got, "resolved from the title") {
		t.Errorf("nothing was derived, so no line:\n%s", got)
	}
	if d := render(build.Coverage{Authored: 3, Derived: 3}); !strings.Contains(d, "3 resolved from the title") {
		t.Errorf("a derived id must be reported:\n%s", d)
	}
}
