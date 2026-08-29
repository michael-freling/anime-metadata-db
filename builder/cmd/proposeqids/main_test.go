package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wikidataResponse is a wbgetentities reply covering the four shapes a review
// has to tell apart: a work with a P1476 title, one with only a label, a
// disambiguation page, and an entity with neither.
const wikidataResponse = `{"entities":{
  "Q98642652":{"id":"Q98642652",
    "sitelinks":{"jawiki":{"title":"葬送のフリーレン"}},
    "labels":{"en":{"value":"Frieren"}},
    "claims":{
      "P1476":[{"mainsnak":{"datavalue":{"value":{"text":"葬送のフリーレン","language":"ja"}}}},
               {"mainsnak":{"datavalue":{"value":{"text":"Frieren: Beyond Journey's End","language":"en"}}}}],
      "P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}},
  "Q116865404":{"id":"Q116865404",
    "sitelinks":{"jawiki":{"title":"青のミブロ"}},
    "labels":{"en":{"value":"The Blue Wolves of Mibu"}}},
  "Q4167410x":{"id":"Q4167410x",
    "sitelinks":{"jawiki":{"title":"曖昧さ回避"}},
    "labels":{"en":{"value":"Ambiguous"}},
    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q4167410"}}}}]}},
  "Q7":{"id":"Q7","sitelinks":{"jawiki":{"title":"無名"}}},
  "Q404":{"id":"Q404","missing":""},
  "-1":{"id":"-1","sitelinks":{"jawiki":{"title":"負"}}}
}}`

// writeOverrides lays out an overrides directory holding one series file.
func writeOverrides(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	seriesDir := filepath.Join(dir, "series")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seriesDir, "s.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fakeGet(body string) getter {
	return func(_ context.Context, _ string) ([]byte, error) { return []byte(body), nil }
}

// The whole point of the tool: one reviewable row per series, saying what was
// found and whether the build would take it.
func TestRunProposesReviewableRows(t *testing.T) {
	dir := writeOverrides(t, `franchise:
  id: f
  titles: { original: フランチャイズ }
  series:
    - id: sousou-no-frieren
      titles: { original: 葬送のフリーレン }
    - id: ao-no-miburo
      titles: { original: 青のミブロ }
    - id: nameless
      titles: { original: 無名 }
    - id: ambiguous
      titles: { original: 曖昧さ回避 }
    - id: black-clover
      titles: { original: ブラッククローバー }
    - id: already-known
      titles: { original: 既知 }
      externalIds: { wikidataId: Q1 }
`)
	var out, logOut bytes.Buffer
	if err := run(&out, &logOut, fakeGet(wikidataResponse), dir, 0); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		// P1476 beats the label, and the source column says which answered.
		"sousou-no-frieren\tQ98642652\t葬送のフリーレン\tFrieren: Beyond Journey's End\tP1476\tok",
		// No claim, so the label carries it.
		"ao-no-miburo\tQ116865404\t青のミブロ\tThe Blue Wolves of Mibu\tlabel\tok",
		// Matched, but nothing English to offer.
		"nameless\tQ7\t無名\t-\t-\tno English title",
		// The most common wrong match must be called out, not quietly listed.
		"ambiguous\tQ4167410x\t曖昧さ回避\tAmbiguous\tlabel\tREJECT: a disambiguation page",
		// No article by that exact title: a manual lookup, not a silent drop.
		"black-clover\t-\tブラッククローバー\t-\t-\tno jawiki article",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing row %q in:\n%s", want, got)
		}
	}
	// A series that already names a work is not re-proposed.
	if strings.Contains(got, "already-known") {
		t.Errorf("re-proposed a series that already has a qid:\n%s", got)
	}
	if !strings.Contains(logOut.String(), "4/5 series matched") {
		t.Errorf("summary should count matches: %q", logOut.String())
	}
}

// -limit keeps a trial run small; it must cut the work list, not the output.
func TestRunLimit(t *testing.T) {
	dir := writeOverrides(t, `series:
  id: sousou-no-frieren
  titles: { original: 葬送のフリーレン }
`)
	var out, logOut bytes.Buffer
	if err := run(&out, &logOut, fakeGet(wikidataResponse), dir, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logOut.String(), "1/1 series matched") {
		t.Errorf("summary = %q", logOut.String())
	}
}

// Nothing to do is a clean exit with a reason, not an empty table.
func TestRunNothingToPropose(t *testing.T) {
	dir := writeOverrides(t, `series:
  id: x
  titles: { original: 既知 }
  externalIds: { wikidataId: Q1 }
`)
	var out, logOut bytes.Buffer
	get := func(context.Context, string) ([]byte, error) {
		t.Fatal("must not fetch when there is nothing to propose")
		return nil, nil
	}
	if err := run(&out, &logOut, get, dir, 0); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || !strings.Contains(logOut.String(), "already names a Wikidata work") {
		t.Errorf("out=%q log=%q", out.String(), logOut.String())
	}
}

func TestRunErrors(t *testing.T) {
	dir := writeOverrides(t, `series:
  id: x
  titles: { original: 葬送のフリーレン }
`)
	var out, logOut bytes.Buffer

	failing := func(context.Context, string) ([]byte, error) { return nil, errors.New("boom") }
	if err := run(&out, &logOut, failing, dir, 0); err == nil {
		t.Error("expected a fetch error")
	}
	bad := func(context.Context, string) ([]byte, error) { return []byte("{nope"), nil }
	if err := run(&out, &logOut, bad, dir, 0); err == nil {
		t.Error("expected a decode error")
	}
	// An unparseable override fails before any fetch.
	broken := writeOverrides(t, "series: [this is not a series]\n")
	if err := run(&out, &logOut, fakeGet(wikidataResponse), broken, 0); err == nil {
		t.Error("expected an overrides load error")
	}
}

// The request must ask for exactly the three props the verdict needs, keyed by
// Japanese Wikipedia article title.
func TestLookupRequestShape(t *testing.T) {
	var seen string
	get := func(_ context.Context, url string) ([]byte, error) {
		seen = url
		return []byte(`{"entities":{}}`), nil
	}
	if _, err := lookup(context.Background(), get, []string{"葬送のフリーレン", "青のミブロ"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sites=jawiki", "sitelinks", "labels", "claims", "wbgetentities"} {
		if !strings.Contains(seen, want) {
			t.Errorf("request missing %q: %s", want, seen)
		}
	}
}

// A title claim in another language must not be read as the English one.
func TestLookupIgnoresNonEnglishTitleClaims(t *testing.T) {
	const jaOnly = `{"entities":{"Q1":{"id":"Q1","sitelinks":{"jawiki":{"title":"x"}},
	  "labels":{"en":{"value":"Fallback"}},
	  "claims":{"P1476":[{"mainsnak":{"datavalue":{"value":{"text":"日本語","language":"ja"}}}}]}}}}`
	ents, err := lookup(context.Background(), fakeGet(jaOnly), []string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Fatalf("got %d entities", len(ents))
	}
	if ents[0].title != "" || ents[0].candidate() != "Fallback" {
		t.Errorf("a ja title claim must not answer as English: %+v", ents[0])
	}
}

func TestChunk(t *testing.T) {
	got := chunk([]int{1, 2, 3, 4, 5}, 2)
	if len(got) != 3 || len(got[2]) != 1 {
		t.Errorf("chunk = %v", got)
	}
	if chunk([]int(nil), 2) != nil {
		t.Error("empty input should chunk to nothing")
	}
}
