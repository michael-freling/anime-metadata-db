// Package config loads builder configuration: the open-data source URLs, their
// pinned versions and checksums, plus the repo layout directories.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Source names used as keys in Config.Sources and as cache file basenames.
const (
	SourceOfflineDatabase = "offlineDatabase"
	SourceAnimeList       = "animeList"
	SourceMovieSetList    = "movieSetList"
)

// requiredSources are the sources every config must define, in canonical order.
var requiredSources = []string{SourceOfflineDatabase, SourceAnimeList, SourceMovieSetList}

// SourceNames returns the canonical source names in a stable order.
func SourceNames() []string {
	return append([]string(nil), requiredSources...)
}

// Source is a single pinned open-data input.
type Source struct {
	// URL is where init/refresh downloads the source from.
	URL string `yaml:"url"`
	// Version is the pinned upstream version (e.g. a release tag or date).
	Version string `yaml:"version"`
	// SHA256 is the hex checksum of the pinned file; empty means "not yet
	// pinned" and init will record it on first download.
	SHA256 string `yaml:"sha256,omitempty"`
	// Filename is the cache filename to write under the sources dir.
	Filename string `yaml:"filename"`
}

// Settings holds the repo layout directories.
type Settings struct {
	SourcesDir   string `yaml:"sourcesDir"`
	OverridesDir string `yaml:"overridesDir"`
	DataDir      string `yaml:"dataDir"`
}

// Config is the parsed config.yaml.
type Config struct {
	Sources  map[string]Source `yaml:"sources"`
	Settings Settings          `yaml:"settings"`
}

// Default returns a Config with the canonical open-data sources and the
// standard repo layout. It is the seed written by `builder init` when no
// config.yaml exists yet.
func Default() Config {
	return Config{
		Sources: map[string]Source{
			SourceOfflineDatabase: {
				// The dataset is published as a GitHub release asset (it is no
				// longer committed to the repo tree). The releases/latest alias
				// always resolves to the newest weekly release.
				URL:      "https://github.com/manami-project/anime-offline-database/releases/latest/download/anime-offline-database-minified.json",
				Version:  "latest",
				Filename: "anime-offline-database.json",
			},
			SourceAnimeList: {
				URL:      "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-list-master.xml",
				Version:  "master",
				Filename: "anime-list.xml",
			},
			SourceMovieSetList: {
				URL:      "https://raw.githubusercontent.com/Anime-Lists/anime-lists/master/anime-movieset-list.xml",
				Version:  "master",
				Filename: "anime-movieset-list.xml",
			},
		},
		Settings: Settings{
			SourcesDir:   ".sources",
			OverridesDir: "overrides",
			DataDir:      "data",
		},
	}
}

// Load reads and validates a config.yaml from path.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes the config to path as YAML.
func (c Config) Save(path string) error {
	raw, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// Validate checks that all required sources are present and well-formed and
// that the layout directories are set.
func (c Config) Validate() error {
	for _, name := range requiredSources {
		src, ok := c.Sources[name]
		if !ok {
			return fmt.Errorf("config: missing required source %q", name)
		}
		if src.URL == "" {
			return fmt.Errorf("config: source %q has no url", name)
		}
		if src.Filename == "" {
			return fmt.Errorf("config: source %q has no filename", name)
		}
	}
	if c.Settings.SourcesDir == "" {
		return fmt.Errorf("config: settings.sourcesDir is empty")
	}
	if c.Settings.OverridesDir == "" {
		return fmt.Errorf("config: settings.overridesDir is empty")
	}
	if c.Settings.DataDir == "" {
		return fmt.Errorf("config: settings.dataDir is empty")
	}
	return nil
}
