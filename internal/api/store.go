// Package api is the read-only Connect service over the committed anime
// dataset. It is deliberately separate from internal/builder: the builder
// writes data/, this package serves it. The service implementation converts the
// internal/model records into the generated anime.v1 protobuf messages.
package api

import (
	"errors"
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

	// Aggregates over everything beneath the entry, computed once at load by
	// indexWorks so listing the catalog never walks the hierarchy. The year
	// span is 0/0 when nothing beneath it carries a release year.
	FirstReleaseYear  int
	LatestReleaseYear int
	Works             int
	Episodes          int
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
	Characters int
	Staff      int

	// The span of release years the dataset actually covers. Both are 0 when
	// nothing in it carries a year. EarliestReleaseYear is the floor below which
	// a year cannot be real data for this dataset.
	EarliestReleaseYear int
	LatestReleaseYear   int
}

// StaffCredit is one role a staff member is cast in: the character they voice,
// the language, and the series the casting applies to.
type StaffCredit struct {
	Character *model.Character
	Language  string
	SeriesIDs []string
}

// Store is an in-memory, read-only index over the dataset. It is built once at
// startup and is safe for concurrent reads.
type Store struct {
	franchises    []*model.Franchise
	franchiseByID map[string]*model.Franchise
	seriesByID    map[string]seriesRef
	entries       []CatalogEntry
	stats         Stats

	// R2 cast. Characters are nested in the series records; staff live in their
	// own files. Both are global, so they are indexed once across every record.
	characters         []*model.Character
	characterByID      map[string]*model.Character
	charactersBySeries map[string][]*model.Character
	staff              []*model.Staff
	staffByID          map[string]*model.Staff
	creditsByStaff     map[string][]StaffCredit

	// works is every release flattened out of the hierarchy, in catalog order,
	// so browsing by year or quarter scans one slice instead of walking every
	// record. Built by indexWorks at load.
	works []Work
}

// The dataset subtrees the store reads.
const (
	seriesGlob = "data/series"
	staffGlob  = "data/staff"
)

// NewStore reads every data/series/*.yaml record from fsys and builds the
// indexes. It returns an error if a record is malformed or if two records share
// a franchise or series id.
func NewStore(fsys fs.FS) (*Store, error) {
	entries, err := fs.ReadDir(fsys, seriesGlob)
	if err != nil {
		return nil, fmt.Errorf("read dataset dir: %w", err)
	}
	s := &Store{
		franchiseByID:      map[string]*model.Franchise{},
		seriesByID:         map[string]seriesRef{},
		characterByID:      map[string]*model.Character{},
		charactersBySeries: map[string][]*model.Character{},
		staffByID:          map[string]*model.Staff{},
		creditsByStaff:     map[string][]StaffCredit{},
	}
	for _, name := range yamlNames(entries) {
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
	if err := s.loadStaff(fsys); err != nil {
		return nil, err
	}
	s.indexCredits()
	s.indexWorks()
	return s, nil
}

// yamlNames returns the YAML filenames of entries, sorted, so the catalog order
// is deterministic regardless of the filesystem's directory order.
func yamlNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !isYAML(e.Name()) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// loadStaff reads every data/staff/*.yaml record. The subtree is optional: a
// dataset with no staff files loads fine, it just has no staff.
func (s *Store) loadStaff(fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, staffGlob)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read staff dir: %w", err)
	}
	for _, name := range yamlNames(entries) {
		raw, err := fs.ReadFile(fsys, path.Join(staffGlob, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		var rec model.StaffRecord
		if err := yaml.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		for i := range rec.Staff {
			st := &rec.Staff[i]
			if _, dup := s.staffByID[st.ID]; dup {
				return fmt.Errorf("%s: duplicate staff id %q", name, st.ID)
			}
			s.staff = append(s.staff, st)
			s.staffByID[st.ID] = st
			s.stats.Staff++
		}
	}
	return nil
}

// indexCredits builds the staff -> characters reverse index. A character's
// default voiceActors apply to every appearance that does not override them, so
// one credit is recorded per (staff, language) with the series it covers.
func (s *Store) indexCredits() {
	for _, c := range s.characters {
		// staff id -> language -> series ids (deduplicated, insertion-ordered).
		byStaff := map[string]map[string][]string{}
		record := func(staffID, language, seriesID string) {
			langs, ok := byStaff[staffID]
			if !ok {
				langs = map[string][]string{}
				byStaff[staffID] = langs
			}
			for _, existing := range langs[language] {
				if existing == seriesID {
					return
				}
			}
			if seriesID != "" {
				langs[language] = append(langs[language], seriesID)
			} else if _, seen := langs[language]; !seen {
				langs[language] = nil
			}
		}
		for _, a := range c.Appearances {
			cast := a.VoiceActors
			if len(cast) == 0 {
				cast = c.VoiceActors
			}
			for _, va := range cast {
				record(va.StaffID, va.Language, a.SeriesID)
			}
		}
		if len(c.Appearances) == 0 {
			for _, va := range c.VoiceActors {
				record(va.StaffID, va.Language, "")
			}
		}
		// Emit in a deterministic order: character order outer, language inner.
		for staffID, langs := range byStaff {
			keys := make([]string, 0, len(langs))
			for lang := range langs {
				keys = append(keys, lang)
			}
			sort.Strings(keys)
			for _, lang := range keys {
				s.creditsByStaff[staffID] = append(s.creditsByStaff[staffID], StaffCredit{
					Character: c,
					Language:  lang,
					SeriesIDs: langs[lang],
				})
			}
		}
	}
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
	return s.indexCast(file, rec)
}

// indexCast indexes the record's characters, which are nested under the
// franchise (for a multi-series brand) or under the standalone series. Ids are
// global, so a duplicate across two files is an error. A character is filed
// under every series it appears in.
func (s *Store) indexCast(file string, rec model.Record) error {
	cast := rec.Cast()
	for i := range cast {
		c := &cast[i]
		if _, dup := s.characterByID[c.ID]; dup {
			return fmt.Errorf("%s: duplicate character id %q", file, c.ID)
		}
		s.characters = append(s.characters, c)
		s.characterByID[c.ID] = c
		s.stats.Characters++
		for _, a := range c.Appearances {
			if a.SeriesID == "" {
				continue
			}
			s.charactersBySeries[a.SeriesID] = append(s.charactersBySeries[a.SeriesID], c)
		}
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

// Character returns the character with the given id, or false if none exists.
func (s *Store) Character(id string) (*model.Character, bool) {
	c, ok := s.characterByID[id]
	return c, ok
}

// Characters returns the cast of seriesID, or the whole cast when seriesID is
// empty, in deterministic dataset order. limit caps the count; a non-positive
// limit applies defaultListLimit.
func (s *Store) Characters(seriesID string, limit int) []*model.Character {
	all := s.characters
	if seriesID != "" {
		all = s.charactersBySeries[seriesID]
	}
	return capSlice(all, limit)
}

// Staff returns the staff member with the given id, or false if none exists.
func (s *Store) Staff(id string) (*model.Staff, bool) {
	st, ok := s.staffByID[id]
	return st, ok
}

// StaffList returns every staff member, or only those credited in language when
// it is non-empty, in deterministic dataset order. limit caps the count; a
// non-positive limit applies defaultListLimit.
func (s *Store) StaffList(language string, limit int) []*model.Staff {
	if language == "" {
		return capSlice(s.staff, limit)
	}
	var out []*model.Staff
	for _, st := range s.staff {
		for _, credit := range s.creditsByStaff[st.ID] {
			if credit.Language == language {
				out = append(out, st)
				break
			}
		}
	}
	return capSlice(out, limit)
}

// StaffCredits returns every character the staff member is cast as, in
// deterministic dataset order (nil for an unknown or uncredited id).
func (s *Store) StaffCredits(staffID string) []StaffCredit { return s.creditsByStaff[staffID] }

// capSlice truncates in to limit entries, applying defaultListLimit when limit
// is non-positive.
func capSlice[T any](in []T, limit int) []T {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if len(in) > limit {
		return in[:limit]
	}
	return in
}

// defaultListLimit caps list results when the caller passes no limit.
const defaultListLimit = 100

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
