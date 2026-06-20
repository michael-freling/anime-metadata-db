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

// Bundle is the result of loading an overrides directory: the R1 series /
// franchise files and the R2 character / staff files, routed by content.
type Bundle struct {
	Series     []Override
	Characters []CharactersOverride
}

// fileKind distinguishes the two override shapes.
type fileKind int

const (
	kindSeries fileKind = iota
	kindCharacters
)

// detectKind inspects a file's top-level keys to route it without strict
// field checking.
func detectKind(raw []byte, relPath string) (fileKind, error) {
	var top map[string]yaml.Node
	if err := yaml.Unmarshal(raw, &top); err != nil {
		return 0, fmt.Errorf("parse override %q: %w", relPath, err)
	}
	_, hasCharacters := top["characters"]
	_, hasStaff := top["staff"]
	_, hasFranchise := top["franchise"]
	_, hasSeries := top["series"]
	r2 := hasCharacters || hasStaff
	r1 := hasFranchise || hasSeries
	switch {
	case r1 && r2:
		return 0, fmt.Errorf("override %q: mixes series/franchise with characters/staff", relPath)
	case r2:
		return kindCharacters, nil
	case r1:
		return kindSeries, nil
	default:
		return 0, fmt.Errorf("override %q: no recognized top-level key (franchise, series, characters or staff)", relPath)
	}
}

// LoadDir loads every *.yaml / *.yml file under dir (recursively), routing each
// to the series or characters bundle by its content, sorted by relative path
// for deterministic ordering. Every id (series, franchise, character, staff)
// must be globally unique. A missing directory yields an empty bundle and no
// error.
func LoadDir(dir string) (Bundle, error) {
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
			return Bundle{}, nil
		}
		return Bundle{}, fmt.Errorf("walk overrides dir: %w", err)
	}
	sort.Strings(paths)

	var bundle Bundle
	seen := make(map[string]string, len(paths))
	register := func(id, path string) error {
		if id == "" {
			return nil
		}
		if prev, dup := seen[id]; dup {
			return fmt.Errorf("override %q: duplicate id %q (also in %q)", path, id, prev)
		}
		seen[id] = path
		return nil
	}

	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return Bundle{}, fmt.Errorf("relativise override path: %w", err)
		}
		relPath := filepath.ToSlash(rel)
		raw, err := os.ReadFile(p)
		if err != nil {
			return Bundle{}, fmt.Errorf("read override: %w", err)
		}
		kind, err := detectKind(raw, relPath)
		if err != nil {
			return Bundle{}, err
		}
		switch kind {
		case kindCharacters:
			co, err := ParseCharacters(raw, relPath)
			if err != nil {
				return Bundle{}, err
			}
			for _, id := range co.IDs() {
				if err := register(id, co.Path); err != nil {
					return Bundle{}, err
				}
			}
			bundle.Characters = append(bundle.Characters, co)
		default:
			o, err := Parse(raw, relPath)
			if err != nil {
				return Bundle{}, err
			}
			if err := register(o.ID(), o.Path); err != nil {
				return Bundle{}, err
			}
			bundle.Series = append(bundle.Series, o)
		}
	}
	return bundle, nil
}
