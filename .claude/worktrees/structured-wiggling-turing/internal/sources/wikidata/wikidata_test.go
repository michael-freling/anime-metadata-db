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

func TestFetchLabels(t *testing.T) {
	var calls int
	var seenIDs []string
	get := func(_ context.Context, url string) ([]byte, error) {
		calls++
		// Echo back only the ids that were requested so batching is observable.
		seenIDs = append(seenIDs, url)
		return []byte(sample), nil
	}
	raw, ents, err := FetchLabels(context.Background(), get, "https://www.wikidata.org/w/api.php", []string{"Q2", "Q1", "Q1"})
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

func TestFetchLabelsBatches(t *testing.T) {
	var calls int
	get := func(_ context.Context, _ string) ([]byte, error) {
		calls++
		return []byte(`{"entities":{}}`), nil
	}
	ids := make([]string, batchSize+1)
	for i := range ids {
		ids[i] = "Q" + string(rune('A'+i%26)) + string(rune('0'+i/26))
	}
	if _, _, err := FetchLabels(context.Background(), get, "https://x/api.php", ids); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 batches for %d ids, got %d calls", batchSize+1, calls)
	}
}

func TestFetchLabelsEmpty(t *testing.T) {
	get := func(_ context.Context, _ string) ([]byte, error) {
		t.Fatal("should not fetch for empty qids")
		return nil, nil
	}
	raw, ents, err := FetchLabels(context.Background(), get, "https://x/api.php", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ents.Len() != 0 || !strings.Contains(string(raw), "entities") {
		t.Errorf("unexpected empty result: %s", raw)
	}
}

func TestFetchLabelsErrors(t *testing.T) {
	ctx := context.Background()
	// Transport error.
	failGet := func(_ context.Context, _ string) ([]byte, error) { return nil, errors.New("boom") }
	if _, _, err := FetchLabels(ctx, failGet, "https://x/api.php", []string{"Q1"}); err == nil {
		t.Error("expected fetch error")
	}
	// Bad JSON response.
	badGet := func(_ context.Context, _ string) ([]byte, error) { return []byte("{nope"), nil }
	if _, _, err := FetchLabels(ctx, badGet, "https://x/api.php", []string{"Q1"}); err == nil {
		t.Error("expected decode error")
	}
	// Bad api URL.
	okGet := func(_ context.Context, _ string) ([]byte, error) { return []byte(sample), nil }
	if _, _, err := FetchLabels(ctx, okGet, "://bad-url", []string{"Q1"}); err == nil {
		t.Error("expected url parse error")
	}
}
