package config

import (
	"os"
	"testing"
)

// TestExampleConfigLoads guards the committed config.yaml in the repo root: it
// must always load and validate.
func TestExampleConfigLoads(t *testing.T) {
	const path = "../../config.yaml"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no example config.yaml")
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("example config failed to load: %v", err)
	}
	for _, name := range SourceNames() {
		if _, ok := cfg.Sources[name]; !ok {
			t.Errorf("example config missing source %q", name)
		}
	}
}
