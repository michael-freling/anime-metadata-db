package wikidata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `{"entities":{
  "Q1":{"id":"Q1","labels":{"en":{"language":"en","value":"Saber"},"ja":{"language":"ja","value":"セイバー"}}},
  "Q2":{"id":"Q2","missing":""}
}}`

func TestParseAndLookup(t *testing.T) {
	e, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if e.Len() != 1 {
		t.Fatalf("expected 1 entity (missing skipped), got %d", e.Len())
	}
	ent, ok := e.Lookup("Q1")
	if !ok || ent.Labels["en"] != "Saber" || ent.Labels["ja"] != "セイバー" {
		t.Errorf("unexpected entity: %+v ok=%v", ent, ok)
	}
	if _, ok := e.Lookup("Q2"); ok {
		t.Error("missing entity should not be indexed")
	}
	if _, ok := e.Lookup("Q999"); ok {
		t.Error("unknown qid should not be found")
	}
}

func TestParseError(t *testing.T) {
	if _, err := Parse(strings.NewReader("{not json")); err == nil {
		t.Error("expected decode error")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wd.json")
	if err := os.WriteFile(path, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if e.Len() != 1 {
		t.Errorf("Len = %d", e.Len())
	}
	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestFetchEntities(t *testing.T) {
	var calls int
	var seenIDs []string
	get := func(_ context.Context, url string) ([]byte, error) {
		calls++
		// Echo back only the ids that were requested so batching is observable.
		seenIDs = append(seenIDs, url)
		return []byte(sample), nil
	}
	raw, ents, err := FetchEntities(context.Background(), get, "https://www.wikidata.org/w/api.php", []string{"Q2", "Q1", "Q1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 batched call, got %d", calls)
	}
	if ents.Len() != 1 {
		t.Errorf("entities Len = %d", ents.Len())
	}
	if !strings.Contains(string(raw), "Q1") {
		t.Errorf("merged cache missing Q1: %s", raw)
	}
	// The single request URL must carry both unique ids, sorted+encoded.
	if !strings.Contains(seenIDs[0], "Q1") || !strings.Contains(seenIDs[0], "Q2") {
		t.Errorf("request URL missing ids: %s", seenIDs[0])
	}
}

func TestFetchEntitiesBatches(t *testing.T) {
	var calls int
	get := func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return []byte(`{"entities":{}}`), nil
	}
	ids := make([]string, batchSize+1)
	for i := range ids {
		ids[i] = "Q" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	if _, _, err := FetchEntities(context.Background(), get, "https://x/api.php", ids, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 batches for %d ids, got %d calls", batchSize+1, calls)
	}
}

func TestFetchEntitiesEmpty(t *testing.T) {
	get := func(_ context.Context, _ string) ([]byte, error) {
		t.Fatal("should not fetch for empty qids")
		return nil, nil
	}
	raw, ents, err := FetchEntities(context.Background(), get, "https://x/api.php", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ents.Len() != 0 || !strings.Contains(string(raw), "entities") {
		t.Errorf("unexpected empty result: %s", raw)
	}
}

func TestFetchEntitiesErrors(t *testing.T) {
	ctx := context.Background()
	// Transport error.
	failGet := func(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("boom") }
	if _, _, err := FetchEntities(ctx, failGet, "https://x/api.php", []string{"Q1"}, nil); err == nil {
		t.Error("expected fetch error")
	}
	// Bad JSON response.
	badGet := func(_ context.Context, _ string) ([]byte, error) { return []byte("{nope"), nil }
	if _, _, err := FetchEntities(ctx, badGet, "https://x/api.php", []string{"Q1"}, nil); err == nil {
		t.Error("expected decode error")
	}
	// Bad api URL.
	okGet := func(_ context.Context, _ string) ([]byte, error) { return []byte(sample), nil }
	if _, _, err := FetchEntities(ctx, okGet, "://bad-url", []string{"Q1"}, nil); err == nil {
		t.Error("expected url parse error")
	}
}

// Wikidata is migrating names that do not vary by language onto a single "mul"
// label instead of a per-language duplicate. Read literally, those entities
// have no en or ja label and the name comes out empty — which is how a batch of
// English voice actors first arrived nameless.
func TestParseFillsMissingLanguagesFromMul(t *testing.T) {
	e, err := Parse(strings.NewReader(`{"entities":{
		"Q1":{"id":"Q1","labels":{"mul":{"language":"mul","value":"Sarah Roach"}}},
		"Q2":{"id":"Q2","labels":{
			"mul":{"language":"mul","value":"Latin Form"},
			"ja":{"language":"ja","value":"日本語名"}}},
		"Q3":{"id":"Q3","labels":{"en":{"language":"en","value":"Only English"}}}
	}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// A lone mul label fills en — and pointedly not ja: the build reads ja as
	// Title.Original, the native-script form, so a romanized value there would
	// be wrong and would also suppress the report that flags it as missing.
	one, _ := e.Lookup("Q1")
	if one.Labels["en"] != "Sarah Roach" {
		t.Errorf("Q1 en = %q, want the mul value", one.Labels["en"])
	}
	if one.Labels["ja"] != "" {
		t.Errorf("Q1 ja = %q, want it left empty so the build reports it", one.Labels["ja"])
	}
	// A real per-language label always wins over the fallback.
	two, _ := e.Lookup("Q2")
	if two.Labels["ja"] != "日本語名" || two.Labels["en"] != "Latin Form" {
		t.Errorf("Q2 labels = %v", two.Labels)
	}
	// mul is a source of values, not a language we store.
	if _, ok := one.Labels["mul"]; ok {
		t.Error("mul was kept as if it were a language")
	}
	// An entity without mul is untouched.
	three, _ := e.Lookup("Q3")
	if len(three.Labels) != 1 || three.Labels["en"] != "Only English" {
		t.Errorf("Q3 labels = %v", three.Labels)
	}
}

// Wikidata appends a parenthetical to a label that would collide with another
// entity's — "イアン・シンクレア (声優)", where 声優 means voice actor. It is an
// artifact of their index, not part of a name, and leaving it in did two kinds
// of damage: it stored a name nobody has, and its kanji made an otherwise
// all-katakana label stop looking like one, so the rule that picks a person's
// name over its Japanese rendering skipped him without a word.
func TestParseDropsWikidataDisambiguators(t *testing.T) {
	const raw = `{"entities":{
	  "Q1":{"id":"Q1","labels":{
	    "ja":{"language":"ja","value":"イアン・シンクレア (声優)"},
	    "en":{"language":"en","value":"Ian Sinclair"}}},
	  "Q2":{"id":"Q2","labels":{
	    "ja":{"language":"ja","value":"花江夏樹"},
	    "en":{"language":"en","value":"Kana Hanazawa （声優）"}}},
	  "Q3":{"id":"Q3","labels":{"ja":{"language":"ja","value":"(声優)"}}},
	  "Q4":{"id":"Q4","labels":{"ja":{"language":"ja","value":"イアン・シンクレア（声優)"}}},
	  "Q5":{"id":"Q5","labels":{"ja":{"language":"ja","value":"イアン・シンクレア (声優)　"}}},
	  "Q6":{"id":"Q6","labels":{"ja":{"language":"ja","value":"イアン・シンクレア (俳優) (声優)"}}}
	}}`
	got, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ qid, lang, want string }{
		{"Q1", "ja", "イアン・シンクレア"},    // the suffix goes
		{"Q1", "en", "Ian Sinclair"}, // a label without one is untouched
		{"Q2", "ja", "花江夏樹"},
		{"Q2", "en", "Kana Hanazawa"}, // full-width brackets too
		{"Q3", "ja", "(声優)"},          // nothing but the suffix: keep it
		{"Q4", "ja", "イアン・シンクレア"},     // brackets that do not match each other
		{"Q5", "ja", "イアン・シンクレア"},     // an ideographic space after the suffix
		{"Q6", "ja", "イアン・シンクレア"},     // two of them, stripped by the loop
	} {
		e, ok := got.Lookup(tc.qid)
		if !ok {
			t.Fatalf("%s missing", tc.qid)
		}
		if e.Labels[tc.lang] != tc.want {
			t.Errorf("%s %s = %q, want %q", tc.qid, tc.lang, e.Labels[tc.lang], tc.want)
		}
	}
}

// claimsSample is a wbgetentities response carrying P1476 the way the API
// returns it: a monolingual-text value per language, wrapped in a statement.
const claimsSample = `{"entities":{"Q98642652":{"id":"Q98642652",
  "labels":{"en":{"language":"en","value":"Frieren"},"ja":{"language":"ja","value":"葬送のフリーレン"}},
  "claims":{"P1476":[
    {"mainsnak":{"datavalue":{"value":{"text":"葬送のフリーレン","language":"ja"}}}},
    {"mainsnak":{"datavalue":{"value":{"text":"Frieren: Beyond Journey's End","language":"en"}}}}],
   "P31":[{"mainsnak":{"datavalue":{"value":{"text":"ignored","language":"en"}}}}]}}}}`

// A work's title claim is read alongside its label, and the two stay distinct:
// the label is the item's name, the claim is the work's title.
func TestParseTitleClaims(t *testing.T) {
	e, err := Parse(strings.NewReader(claimsSample))
	if err != nil {
		t.Fatal(err)
	}
	ent, ok := e.Lookup("Q98642652")
	if !ok {
		t.Fatal("entity not indexed")
	}
	if got := ent.Titles["en"]; got != "Frieren: Beyond Journey's End" {
		t.Errorf("en title = %q", got)
	}
	if got := ent.Titles["ja"]; got != "葬送のフリーレン" {
		t.Errorf("ja title = %q", got)
	}
	if got := ent.Labels["en"]; got != "Frieren" {
		t.Errorf("label must not be replaced by the claim: %q", got)
	}
}

// An entity with no P1476 has no titles, rather than an empty map that would
// read as "looked up and found nothing".
func TestParseWithoutTitleClaims(t *testing.T) {
	e, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	ent, _ := e.Lookup("Q1")
	if ent.Titles != nil {
		t.Errorf("expected no titles, got %v", ent.Titles)
	}
}

// The cache stores the distilled titles, not the claims block it came from: a
// full claims response is ~75x a labels one, nearly all of it properties this
// build never reads.
func TestFetchEntitiesReducesClaimsInCache(t *testing.T) {
	get := func(_ context.Context, _ string) ([]byte, error) { return []byte(claimsSample), nil }
	raw, ents, err := FetchEntities(context.Background(), get, "https://x/api.php", nil, []string{"Q98642652"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "mainsnak") || strings.Contains(string(raw), "P31") {
		t.Errorf("cache kept the raw claims block:\n%s", raw)
	}
	if !strings.Contains(string(raw), "Beyond Journey") {
		t.Errorf("cache lost the title: %s", raw)
	}
	// The reduced cache must parse back to the same entity, so a build reading
	// a cache and one reading a fresh response agree.
	reparsed, err := Parse(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := ents.Lookup("Q98642652")
	b, _ := reparsed.Lookup("Q98642652")
	if a.Titles["en"] != b.Titles["en"] {
		t.Errorf("cache round-trip changed the title: %q vs %q", a.Titles["en"], b.Titles["en"])
	}
}

// Claims are requested only for the works whose titles are read. Asking for
// them over every character and staff entity would multiply the download for a
// field those entities do not have.
func TestFetchEntitiesRequestsClaimsOnlyForTitleQIDs(t *testing.T) {
	byURL := map[string]bool{}
	get := func(_ context.Context, url string) ([]byte, error) {
		byURL[url] = true
		return []byte(`{"entities":{}}`), nil
	}
	if _, _, err := FetchEntities(context.Background(), get, "https://x/api.php",
		[]string{"Q1"}, []string{"Q2"}); err != nil {
		t.Fatal(err)
	}
	if len(byURL) != 2 {
		t.Fatalf("expected a labels batch and a claims batch, got %d requests", len(byURL))
	}
	for url := range byURL {
		wantClaims := strings.Contains(url, "Q2")
		if got := strings.Contains(url, "claims"); got != wantClaims {
			t.Errorf("url %s: claims=%v, want %v", url, got, wantClaims)
		}
	}
}

// A QID named as both a plain id and a title id is fetched once, with claims.
func TestFetchEntitiesDeduplicatesAcrossGroups(t *testing.T) {
	var calls int
	get := func(_ context.Context, url string) ([]byte, error) {
		calls++
		if !strings.Contains(url, "claims") {
			t.Errorf("the shared id must be fetched with claims: %s", url)
		}
		return []byte(`{"entities":{}}`), nil
	}
	if _, _, err := FetchEntities(context.Background(), get, "https://x/api.php",
		[]string{"Q1"}, []string{"Q1"}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected 1 request, got %d", calls)
	}
}
