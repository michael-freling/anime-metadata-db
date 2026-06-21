// Package wikidata loads character/staff labels (names) from Wikidata, the one
// CC0 source the build may redistribute. It resolves QIDs to their multilingual
// labels via the wbgetentities API and caches the merged result for offline
// builds.
package wikidata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
)

// batchSize is the maximum number of entity ids per wbgetentities request.
const batchSize = 50

// languages restricts fetched labels to the ones we store.
const languages = "en|ja"

// label is one localized label value.
type label struct {
	Language string `json:"language"`
	Value    string `json:"value"`
}

// rawEntity is the wbgetentities shape of a single entity. Missing is present
// (as an empty string) when the entity id does not exist.
type rawEntity struct {
	ID      string           `json:"id"`
	Labels  map[string]label `json:"labels,omitempty"`
	Missing *string          `json:"missing,omitempty"`
}

// rawResponse is the wbgetentities response (and our cache file) shape.
type rawResponse struct {
	Entities map[string]rawEntity `json:"entities"`
}

// Entity is a resolved Wikidata entity: its QID and labels by language code.
type Entity struct {
	QID    string
	Labels map[string]string
}

// Entities is an indexed set of resolved entities.
type Entities struct {
	byQID map[string]Entity
}

// Parse reads a wbgetentities-shaped JSON document (a single
// {"entities": {...}} object) and indexes it by QID. Entities flagged
// "missing" are skipped.
func Parse(r io.Reader) (*Entities, error) {
	var raw rawResponse
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode wikidata entities: %w", err)
	}
	e := &Entities{byQID: make(map[string]Entity, len(raw.Entities))}
	for qid, re := range raw.Entities {
		if re.Missing != nil {
			continue
		}
		labels := make(map[string]string, len(re.Labels))
		for lang, l := range re.Labels {
			labels[lang] = l.Value
		}
		e.byQID[qid] = Entity{QID: qid, Labels: labels}
	}
	return e, nil
}

// Load reads and parses a cached wikidata entities file.
func Load(path string) (*Entities, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open wikidata cache: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	return Parse(f)
}

// Lookup returns the entity for a QID.
func (e *Entities) Lookup(qid string) (Entity, bool) {
	ent, ok := e.byQID[qid]
	return ent, ok
}

// Len reports the number of indexed entities.
func (e *Entities) Len() int { return len(e.byQID) }

// Getter fetches the body of a URL.
type Getter func(ctx context.Context, url string) ([]byte, error)

// FetchLabels resolves the given QIDs to their labels via the wbgetentities API
// (batched), and returns the merged cache bytes plus the parsed Entities. QIDs
// are de-duplicated and sorted for deterministic output.
func FetchLabels(ctx context.Context, get Getter, apiURL string, qids []string) ([]byte, *Entities, error) {
	unique := dedupeSorted(qids)
	merged := make(map[string]rawEntity, len(unique))
	for _, batch := range chunk(unique, batchSize) {
		reqURL, err := buildURL(apiURL, batch)
		if err != nil {
			return nil, nil, err
		}
		body, err := get(ctx, reqURL)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch wikidata labels: %w", err)
		}
		var raw rawResponse
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, nil, fmt.Errorf("decode wikidata response: %w", err)
		}
		for qid, re := range raw.Entities {
			merged[qid] = re
		}
	}
	out, err := json.MarshalIndent(rawResponse{Entities: merged}, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("encode wikidata cache: %w", err)
	}
	entities, err := Parse(strings.NewReader(string(out)))
	if err != nil {
		return nil, nil, err
	}
	return out, entities, nil
}

// buildURL constructs a wbgetentities request URL for a batch of QIDs.
func buildURL(apiURL string, ids []string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("parse wikidata api url: %w", err)
	}
	q := u.Query()
	q.Set("action", "wbgetentities")
	q.Set("props", "labels")
	q.Set("languages", languages)
	q.Set("format", "json")
	q.Set("ids", strings.Join(ids, "|"))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// dedupeSorted returns the unique non-empty ids, sorted.
func dedupeSorted(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// chunk splits ids into batches of at most size.
func chunk(ids []string, size int) [][]string {
	var out [][]string
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		out = append(out, ids[i:end])
	}
	return out
}
