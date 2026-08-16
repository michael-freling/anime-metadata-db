// Package api is the read-only Connect service over the committed anime
// dataset. It is deliberately separate from internal/builder: the builder
// writes data/, this package serves it. The service implementation converts the
// internal/model records into the generated anime.v1 protobuf messages.
package api

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// seriesRef pairs a resolved series with the id of the franchise that owns it
// (empty for a standalone top-level series).
type seriesRef struct {
	series      *model.Series
	franchiseID string
}

// CatalogEntry is a searchable top-level node: a franchise or a standalone
// series. FranchiseID is set only for a series owned by a franchise.
type CatalogEntry struct {
	Kind        EntryKind
	ID          string
	Titles      model.Title
	FranchiseID string
}

// EntryKind classifies a catalog entry.
type EntryKind int

// The catalog entry kinds.
const (
	EntryFranchise EntryKind = iota
	EntrySeries
)

// Stats summarizes the loaded dataset.
type Stats struct {
	Franchises int
	Series     int
	Seasons    int
	Episodes   int
}

// Store is an in-memory, read-only index over the dataset. It is built once at
// startup and is safe for concurrent reads.
type Store struct {
	franchises    []*model.Franchise
	franchiseByID map[string]*model.Franchise
	seriesByID    map[string]seriesRef
	entries       []CatalogEntry
	stats         Stats
}

// seriesGlob is the dataset subtree the store reads.
const seriesGlob = "data/series"

// NewStore reads every data/series/*.yaml record from fsys and builds the
// indexes. It returns an error if a record is malformed or if two records share
// a franchise or series id.
func NewStore(fsys fs.FS) (*Store, error) {
	entries, err := fs.ReadDir(fsys, seriesGlob)
	if err != nil {
		return nil, fmt.Errorf("read dataset dir: %w", err)
	}
	s := &Store{
		franchiseByID: map[string]*model.Franchise{},
		seriesByID:    map[string]seriesRef{},
	}
	// Sort filenames so the catalog order is deterministic regardless of the
	// filesystem's directory order.
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		raw, err := fs.ReadFile(fsys, path.Join(seriesGlob, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var rec model.Record
		if err := yaml.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if err := s.add(name, rec); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// isYAML reports whether name has a YAML extension.
func isYAML(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// add indexes one record (a franchise or a standalone series) and accumulates
// its stats, rejecting duplicate ids.
func (s *Store) add(file string, rec model.Record) error {
	switch {
	case rec.Franchise != nil:
		f := rec.Franchise
		if _, dup := s.franchiseByID[f.ID]; dup {
			return fmt.Errorf("%s: duplicate franchise id %q", file, f.ID)
		}
		s.franchises = append(s.franchises, f)
		s.franchiseByID[f.ID] = f
		s.entries = append(s.entries, CatalogEntry{Kind: EntryFranchise, ID: f.ID, Titles: f.Titles})
		s.stats.Franchises++
		for i := range f.Series {
			if err := s.addSeries(file, &f.Series[i], f.ID); err != nil {
				return err
			}
		}
	case rec.Series != nil:
		if err := s.addSeries(file, rec.Series, ""); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s: record has neither franchise nor series", file)
	}
	return nil
}

// addSeries indexes one series and its installments, rejecting duplicate ids.
func (s *Store) addSeries(file string, series *model.Series, franchiseID string) error {
	if _, dup := s.seriesByID[series.ID]; dup {
		return fmt.Errorf("%s: duplicate series id %q", file, series.ID)
	}
	s.seriesByID[series.ID] = seriesRef{series: series, franchiseID: franchiseID}
	s.entries = append(s.entries, CatalogEntry{
		Kind:        EntrySeries,
		ID:          series.ID,
		Titles:      series.Titles,
		FranchiseID: franchiseID,
	})
	s.stats.Series++
	for i := range series.Seasons {
		s.stats.Seasons++
		s.stats.Episodes += len(series.Seasons[i].Episodes)
	}
	for i := range series.Specials {
		s.stats.Episodes += len(series.Specials[i].Episodes)
	}
	return nil
}

// Franchises returns the franchises in deterministic (filename) order.
func (s *Store) Franchises() []*model.Franchise { return s.franchises }

// Franchise returns the franchise with the given id, or false if none exists.
func (s *Store) Franchise(id string) (*model.Franchise, bool) {
	f, ok := s.franchiseByID[id]
	return f, ok
}

// Series returns the series with the given id and its owning franchise id
// (empty for a standalone series), or false if none exists.
func (s *Store) Series(id string) (*model.Series, string, bool) {
	ref, ok := s.seriesByID[id]
	if !ok {
		return nil, "", false
	}
	return ref.series, ref.franchiseID, true
}

// Stats returns the dataset summary computed at load time.
func (s *Store) Stats() Stats { return s.stats }

// Search returns catalog entries whose original or translated title contains
// query (case-insensitive). A blank query matches nothing. limit caps the
// result count; a non-positive limit applies defaultSearchLimit. Results keep
// the deterministic catalog order.
func (s *Store) Search(query string, limit int) []CatalogEntry {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return nil
	}
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	var out []CatalogEntry
	for _, e := range s.entries {
		if titleMatches(e.Titles, q) {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

// defaultSearchLimit caps Search results when the caller passes no limit.
const defaultSearchLimit = 50

// titleMatches reports whether any form of t contains the lowercased needle.
func titleMatches(t model.Title, needle string) bool {
	if strings.Contains(strings.ToLower(t.Original), needle) {
		return true
	}
	for _, v := range t.Translations {
		if strings.Contains(strings.ToLower(v), needle) {
			return true
		}
	}
	return false
}
