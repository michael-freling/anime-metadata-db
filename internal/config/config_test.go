package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestSourceNames(t *testing.T) {
	names := SourceNames()
	if len(names) != 3 {
		t.Fatalf("expected 3 sources, got %d", len(names))
	}
	// Mutating the returned slice must not affect the canonical list.
	names[0] = "mutated"
	if SourceNames()[0] == "mutated" {
		t.Error("SourceNames should return a copy")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	want := Default()
	want.Sources[SourceOfflineDatabase] = Source{
		URL:      want.Sources[SourceOfflineDatabase].URL,
		Filename: want.Sources[SourceOfflineDatabase].Filename,
		SHA256:   "deadbeef",
	}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sources[SourceOfflineDatabase].SHA256 != "deadbeef" {
		t.Errorf("sha256 not persisted: %q", got.Sources[SourceOfflineDatabase].SHA256)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadBadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := writeFileForTest(path, "sources: [this is not a map"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// Valid YAML but missing required sources.
	if err := writeFileForTest(path, "settings:\n  sourcesDir: .sources\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("expected validation error")
	}
}

func TestSaveError(t *testing.T) {
	// Saving into a path whose parent is a file fails.
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := writeFileForTest(file, "x"); err != nil {
		t.Fatal(err)
	}
	if err := Default().Save(filepath.Join(file, "config.yaml")); err == nil {
		t.Error("expected save error when parent is a file")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"missing source", func(c *Config) { delete(c.Sources, SourceAnimeList) }},
		{"no url", func(c *Config) {
			s := c.Sources[SourceAnimeList]
			s.URL = ""
			c.Sources[SourceAnimeList] = s
		}},
		{"no filename", func(c *Config) {
			s := c.Sources[SourceAnimeList]
			s.Filename = ""
			c.Sources[SourceAnimeList] = s
		}},
		{"no sourcesDir", func(c *Config) { c.Settings.SourcesDir = "" }},
		{"no overridesDir", func(c *Config) { c.Settings.OverridesDir = "" }},
		{"no dataDir", func(c *Config) { c.Settings.DataDir = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}
