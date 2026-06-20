// Package overrides loads the authored override layer: hand-edited YAML files
// declaring the Franchise/Series structure and the judgement calls the open
// sources cannot express. The builder treats these files as read-only input.
package overrides

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// Override is one authored file. Exactly one of Franchise or Series is set: a
// multi-storyline brand is a Franchise, a single storyline is a standalone
// Series.
type Override struct {
	// Path is the file's path relative to the overrides directory, used to
	// mirror the layout into the data directory. It is not serialised.
	Path string `yaml:"-"`

	Franchise *model.Franchise `yaml:"franchise,omitempty"`
	Series    *model.Series    `yaml:"series,omitempty"`

	// Numbered lists the ids of Series that form a single linear continuity, so
	// the builder assigns a continuous absoluteNumber across them. It is an
	// authoring directive consumed by the build; the generated data model
	// carries the decision implicitly via the presence of absoluteNumber.
	Numbered []string `yaml:"numbered,omitempty"`
}

// IsNumbered reports whether seriesID was marked as a linear/numbered series.
func (o Override) IsNumbered(seriesID string) bool {
	for _, id := range o.Numbered {
		if id == seriesID {
			return true
		}
	}
	return false
}

// ID returns the franchise or series id of the override.
func (o Override) ID() string {
	switch {
	case o.Franchise != nil:
		return o.Franchise.ID
	case o.Series != nil:
		return o.Series.ID
	default:
		return ""
	}
}

// Validate checks the override declares exactly one well-formed top-level
// entity.
func (o Override) Validate() error {
	switch {
	case o.Franchise != nil && o.Series != nil:
		return fmt.Errorf("override %q: declares both franchise and series", o.Path)
	case o.Franchise != nil:
		if o.Franchise.ID == "" {
			return fmt.Errorf("override %q: franchise has no id", o.Path)
		}
		if len(o.Franchise.Series) == 0 {
			return fmt.Errorf("override %q: franchise %q has no series", o.Path, o.Franchise.ID)
		}
	case o.Series != nil:
		if o.Series.ID == "" {
			return fmt.Errorf("override %q: series has no id", o.Path)
		}
	default:
		return fmt.Errorf("override %q: declares neither franchise nor series", o.Path)
	}
	return nil
}

// Parse decodes a single override from raw YAML, recording relPath for layout
// mirroring.
func Parse(raw []byte, relPath string) (Override, error) {
	var o Override
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&o); err != nil {
		return Override{}, fmt.Errorf("parse override %q: %w", relPath, err)
	}
	o.Path = relPath
	if err := o.Validate(); err != nil {
		return Override{}, err
	}
	return o, nil
}

// Load reads a single override file. relPath is its path relative to the
// overrides root.
func Load(absPath, relPath string) (Override, error) {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Override{}, fmt.Errorf("read override: %w", err)
	}
	return Parse(raw, relPath)
}

// LoadDir loads every *.yaml / *.yml file under dir (recursively), sorted by
// relative path for deterministic ordering. A missing directory yields no
// overrides and no error.
func LoadDir(dir string) ([]Override, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(path)); ext != ".yaml" && ext != ".yml" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("walk overrides dir: %w", err)
	}
	sort.Strings(paths)

	overrides := make([]Override, 0, len(paths))
	seen := make(map[string]string, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return nil, fmt.Errorf("relativise override path: %w", err)
		}
		o, err := Load(p, filepath.ToSlash(rel))
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[o.ID()]; dup {
			return nil, fmt.Errorf("override %q: duplicate id %q (also in %q)", o.Path, o.ID(), prev)
		}
		seen[o.ID()] = o.Path
		overrides = append(overrides, o)
	}
	return overrides, nil
}
