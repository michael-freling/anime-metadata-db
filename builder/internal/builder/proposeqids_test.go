package builder

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// wikidataProposalResponse is a wbgetentities reply covering the shapes a
// review has to tell apart: a work with a P1476 title, one carrying only a
// label, a source video game, a disambiguation page, and a kind of entity the
// allowlist does not name.
const wikidataProposalResponse = `{"entities":{
  "Q98642652":{
    "sitelinks":{"jawiki":{"title":"葬送のフリーレン"}},
    "labels":{"en":{"value":"Frieren"}},
    "claims":{
      "P1476":[{"mainsnak":{"datavalue":{"value":{"text":"葬送のフリーレン","language":"ja"}}}},
               {"mainsnak":{"datavalue":{"value":{"text":"Frieren: Beyond Journey's End","language":"en"}}}}],
      "P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}},
  "Q116865404":{
    "sitelinks":{"jawiki":{"title":"青のミブロ"}},
    "labels":{"en":{"value":"The Blue Wolves of Mibu"}},
    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q21198342"}}}}]}},
  "Q857823":{
    "sitelinks":{"jawiki":{"title":"Fate/stay night"}},
    "labels":{"en":{"value":"Fate/stay night"}},
    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q7889"}}}}]}},
  "Q11339901":{
    "sitelinks":{"jawiki":{"title":"マオ"}},
    "labels":{"en":{"value":"Mao"}},
    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q4167410"}}}}]}},
  "Q999":{
    "sitelinks":{"jawiki":{"title":"謎"}},
    "labels":{"en":{"value":"Mystery"}},
    "claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q123456789"}}}}]}},
  "Q404":{"missing":""},
  "-1":{"sitelinks":{"jawiki":{"title":"負"}}}
}}`

// stubFetcher answers every request with one canned body, or an error.
type stubFetcher struct {
	body string
	err  error
	urls []string
}

func (s *stubFetcher) Get(_ context.Context, url string) ([]byte, error) {
	s.urls = append(s.urls, url)
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.body), nil
}

// proposalRepo lays out a repo root holding one series override file.
func proposalRepo(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	seriesDir := filepath.Join(dir, "config", "overrides", "series")
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seriesDir, "s.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The point of the command: one reviewable row per series, saying what was
// found and what the build would make of it.
func TestProposeQIDsRows(t *testing.T) {
	dir := proposalRepo(t, `franchise:
  id: f
  titles: { original: フランチャイズ }
  series:
    - id: sousou-no-frieren
      titles: { original: 葬送のフリーレン }
    - id: ao-no-miburo
      titles: { original: 青のミブロ }
    - id: fate-stay-night
      titles: { original: Fate/stay night }
    - id: mao
      titles: { original: マオ }
    - id: nazo
      titles: { original: 謎 }
    - id: black-clover
      titles: { original: ブラッククローバー }
    - id: already-known
      titles: { original: 既知 }
      externalIds: { wikidataId: Q1 }
`)
	var out bytes.Buffer
	app := New(dir, &stubFetcher{body: wikidataProposalResponse}, &out)
	if err := app.ProposeQIDs(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	got := out.String()

	for _, want := range []string{
		// A title claim beats the label, and the source column says which answered.
		"sousou-no-frieren\tQ98642652\t葬送のフリーレン\tFrieren: Beyond Journey's End\tP1476\tok — manga series",
		// No claim, so the label carries it.
		"ao-no-miburo\tQ116865404\t青のミブロ\tThe Blue Wolves of Mibu\tlabel\tok — manga series",
		// A visual novel is a real source work, not a wrong match.
		"fate-stay-night\tQ857823\tFate/stay night\tFate/stay night\tlabel\tok — video game",
		// The most common wrong match must be called out, never quietly listed.
		"mao\tQ11339901\tマオ\tMao\tlabel\tREJECT: a disambiguation page",
		// An unrecognised kind is surfaced for a human, not assumed good.
		"nazo\tQ999\t謎\tMystery\tlabel\tCHECK: not a recognised kind of work (P31 Q123456789)",
		// No article by that exact title: a manual lookup, not a silent drop.
		"black-clover\t-\tブラッククローバー\t-\t-\tno jawiki article",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing row %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "already-known") {
		t.Errorf("re-proposed a series that already names a work:\n%s", got)
	}
	if !strings.Contains(got, "5/6 series matched") {
		t.Errorf("summary should count matches:\n%s", got)
	}
}

// The command reports; it must never edit an override.
func TestProposeQIDsNeverWrites(t *testing.T) {
	const body = `series:
  id: sousou-no-frieren
  titles: { original: 葬送のフリーレン }
`
	dir := proposalRepo(t, body)
	path := filepath.Join(dir, "config", "overrides", "series", "s.yaml")
	var out bytes.Buffer
	app := New(dir, &stubFetcher{body: wikidataProposalResponse}, &out)
	if err := app.ProposeQIDs(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != body {
		t.Errorf("override was modified:\n%s", after)
	}
}

// --limit keeps a trial run small by cutting the work list.
func TestProposeQIDsLimit(t *testing.T) {
	dir := proposalRepo(t, `series:
  id: sousou-no-frieren
  titles: { original: 葬送のフリーレン }
`)
	var out bytes.Buffer
	app := New(dir, &stubFetcher{body: wikidataProposalResponse}, &out)
	if err := app.ProposeQIDs(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1/1 series matched") {
		t.Errorf("out = %q", out.String())
	}
}

// Nothing to do is a clean exit with a reason, not an empty table.
func TestProposeQIDsNothingToDo(t *testing.T) {
	dir := proposalRepo(t, `series:
  id: x
  titles: { original: 既知 }
  externalIds: { wikidataId: Q1 }
`)
	var out bytes.Buffer
	fetcher := &stubFetcher{body: wikidataProposalResponse}
	app := New(dir, fetcher, &out)
	if err := app.ProposeQIDs(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if len(fetcher.urls) != 0 {
		t.Error("must not fetch when there is nothing to propose")
	}
	if !strings.Contains(out.String(), "already names a Wikidata work") {
		t.Errorf("out = %q", out.String())
	}
}

// The endpoint comes from config.yaml rather than a constant, and the request
// asks for exactly the three props the verdict needs.
func TestProposeQIDsRequestShape(t *testing.T) {
	dir := proposalRepo(t, `series:
  id: x
  titles: { original: 葬送のフリーレン }
`)
	fetcher := &stubFetcher{body: `{"entities":{}}`}
	var out bytes.Buffer
	if err := New(dir, fetcher, &out).ProposeQIDs(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if len(fetcher.urls) != 1 {
		t.Fatalf("expected 1 batched request, got %d", len(fetcher.urls))
	}
	for _, want := range []string{
		"https://www.wikidata.org/w/api.php", "wbgetentities",
		"sites=jawiki", "sitelinks", "labels", "claims",
	} {
		if !strings.Contains(fetcher.urls[0], want) {
			t.Errorf("request missing %q: %s", want, fetcher.urls[0])
		}
	}
}

func TestProposeQIDsErrors(t *testing.T) {
	dir := proposalRepo(t, `series:
  id: x
  titles: { original: 葬送のフリーレン }
`)
	var out bytes.Buffer

	if err := New(dir, &stubFetcher{err: errors.New("boom")}, &out).ProposeQIDs(context.Background(), 0); err == nil {
		t.Error("expected a fetch error")
	}
	if err := New(dir, &stubFetcher{body: "{nope"}, &out).ProposeQIDs(context.Background(), 0); err == nil {
		t.Error("expected a decode error")
	}
	broken := proposalRepo(t, "series: [not a series]\n")
	if err := New(broken, &stubFetcher{body: `{"entities":{}}`}, &out).ProposeQIDs(context.Background(), 0); err == nil {
		t.Error("expected an overrides load error")
	}
}

// A title claim in another language must not answer as the English one.
func TestLookupWorksIgnoresNonEnglishTitleClaims(t *testing.T) {
	const jaOnly = `{"entities":{"Q1":{"sitelinks":{"jawiki":{"title":"x"}},
	  "labels":{"en":{"value":"Fallback"}},
	  "claims":{"P1476":[{"mainsnak":{"datavalue":{"value":{"text":"日本語","language":"ja"}}}}]}}}}`
	app := New(t.TempDir(), &stubFetcher{body: jaOnly}, &bytes.Buffer{})
	got, err := app.lookupWorks(context.Background(), "https://x/api.php", []string{"x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d proposals", len(got))
	}
	if got[0].title != "" || got[0].candidate() != "Fallback" {
		t.Errorf("a ja claim must not answer as English: %+v", got[0])
	}
}

// A matched work with no English at all is still worth an id, and says so.
func TestProposalVerdictWithoutEnglish(t *testing.T) {
	p := proposal{qid: "Q1", instanceOf: []string{"Q21198342"}}
	if v := p.verdict(); !strings.Contains(v, "no English title or label") {
		t.Errorf("verdict = %q", v)
	}
	if p.source() != "-" {
		t.Errorf("source = %q", p.source())
	}
}
