//go:build e2e

package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestE2EEveryCommittedOverrideParsesAsStrings reads the real authored layer and
// asserts each file is YAML whose titles come back as strings.
//
// The unit tests cover the renderer against a corpus I thought of; this covers
// it against the corpus that exists. Three rendering bugs survived the former
// and died to the latter — a title ending "ka? Bloom" broke the flow mapping it
// was written into, and nothing synthetic had a mid-value "?".
func TestE2EEveryCommittedOverrideParsesAsStrings(t *testing.T) {
	dir := "../../../../dataset/overrides/series"
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no override files under %s (err=%v)", dir, err)
	}

	type titles struct {
		Original     string            `yaml:"original"`
		Translations map[string]string `yaml:"translations"`
	}
	checked := 0
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		var doc struct {
			Series    *struct{ Titles titles } `yaml:"series"`
			Franchise *struct{ Titles titles } `yaml:"franchise"`
		}
		// Decoded into typed strings on purpose: a title of "null" or "yes"
		// parses as a non-string and fails here, which an `any` would swallow.
		if err := yaml.Unmarshal(body, &doc); err != nil {
			t.Errorf("%s: %v", filepath.Base(f), err)
			continue
		}
		node := doc.Series
		if node == nil {
			node = doc.Franchise // franchise files carry the same title shape
		}
		if node == nil {
			t.Errorf("%s: neither a series nor a franchise", filepath.Base(f))
			continue
		}
		if node.Titles.Original == "" {
			t.Errorf("%s: original title is empty or not a string", filepath.Base(f))
		}
		checked++
	}
	t.Logf("%d override files parsed, titles intact", checked)
}
