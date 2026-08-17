package index

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// The dataset subtrees an index is built from.
const (
	seriesDir = "data/series"
	staffDir  = "data/staff"
)

// Dataset is every record in data/, parsed. Building it is the expensive pass
// this package exists to keep out of the server: it parses all of the YAML and
// holds the whole object graph, which is affordable in the builder and in CI
// and is not affordable on a request path.
type Dataset struct {
	files []recordFile
	staff []staffEntry
}

// recordFile is one parsed data/series/*.yaml.
type recordFile struct {
	name string
	rec  model.Record
}

// staffEntry is one staff member and the file that holds them.
type staffEntry struct {
	file  string
	staff model.Staff
}

// Load parses every record under data/ in fsys. Filenames are sorted so the
// index is byte-identical between runs regardless of directory order.
func Load(fsys fs.FS) (*Dataset, error) {
	entries, err := fs.ReadDir(fsys, seriesDir)
	if err != nil {
		return nil, fmt.Errorf("read dataset dir: %w", err)
	}
	d := &Dataset{}
	for _, name := range yamlNames(entries) {
		raw, err := fs.ReadFile(fsys, path.Join(seriesDir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		var rec model.Record
		if err := yaml.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if rec.Franchise == nil && rec.Series == nil {
			return nil, fmt.Errorf("%s: record has neither franchise nor series", name)
		}
		d.files = append(d.files, recordFile{name: name, rec: rec})
	}
	if err := d.loadStaff(fsys); err != nil {
		return nil, err
	}
	return d, nil
}

// loadStaff reads every data/staff/*.yaml record. The subtree is optional: a
// dataset with no staff files loads fine, it just has no staff.
func (d *Dataset) loadStaff(fsys fs.FS) error {
	entries, err := fs.ReadDir(fsys, staffDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read staff dir: %w", err)
	}
	for _, name := range yamlNames(entries) {
		raw, err := fs.ReadFile(fsys, path.Join(staffDir, name))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		var rec model.StaffRecord
		if err := yaml.Unmarshal(raw, &rec); err != nil {
			return fmt.Errorf("parse %s: %w", name, err)
		}
		for _, st := range rec.Staff {
			d.staff = append(d.staff, staffEntry{file: name, staff: st})
		}
	}
	return nil
}

// yamlNames returns the YAML filenames of entries, sorted.
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

// isYAML reports whether name has a YAML extension.
func isYAML(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// rows assigns every franchise, series, character and staff member its row
// number, and validates that ids are unique. Row numbers are what the index
// uses for cross-references, so they are assigned in one place.
type rows struct {
	franchise map[string]int
	series    map[string]int
	character map[string]int
	staff     map[string]int
}

// WriteTo renders the index. It returns an error for a dataset that could not
// be served — duplicate ids, or a record that is neither a franchise nor a
// series — so the build fails rather than shipping an index that silently
// drops records.
func (d *Dataset) WriteTo(w io.Writer) (int64, error) {
	out := bufio.NewWriter(w)
	c := &counter{w: out}

	idx, err := d.assignRows()
	if err != nil {
		return 0, err
	}
	works, err := d.flatten(idx)
	if err != nil {
		return 0, err
	}
	stats := d.stats(works)

	fmt.Fprintf(c, "%s\t%d\n", magic, Version)
	d.writeStats(c, stats)
	d.writeFranchises(c)
	d.writeSeries(c, idx)
	d.writeEntries(c, idx, works)
	writeWorks(c, works)
	d.writeCharacters(c, idx)
	d.writeStaff(c)
	d.writeCredits(c, idx)

	if err := out.Flush(); err != nil {
		return c.n, err
	}
	return c.n, c.err
}

// counter tracks bytes written and the first write error, so the section
// writers can stay free of error plumbing.
type counter struct {
	w   io.Writer
	n   int64
	err error
}

func (c *counter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	if err != nil {
		c.err = err
	}
	return n, err
}

// assignRows numbers every node and rejects duplicate ids.
func (d *Dataset) assignRows() (*rows, error) {
	idx := &rows{
		franchise: map[string]int{},
		series:    map[string]int{},
		character: map[string]int{},
		staff:     map[string]int{},
	}
	for _, f := range d.files {
		rec := f.rec
		if rec.Franchise != nil {
			if _, dup := idx.franchise[rec.Franchise.ID]; dup {
				return nil, fmt.Errorf("%s: duplicate franchise id %q", f.name, rec.Franchise.ID)
			}
			idx.franchise[rec.Franchise.ID] = len(idx.franchise)
		}
		var seriesErr error
		rec.EachSeries(func(s *model.Series) {
			if seriesErr != nil {
				return
			}
			if _, dup := idx.series[s.ID]; dup {
				seriesErr = fmt.Errorf("%s: duplicate series id %q", f.name, s.ID)
				return
			}
			idx.series[s.ID] = len(idx.series)
		})
		if seriesErr != nil {
			return nil, seriesErr
		}
		cast := rec.Cast()
		for i := range cast {
			c := &cast[i]
			if _, dup := idx.character[c.ID]; dup {
				return nil, fmt.Errorf("%s: duplicate character id %q", f.name, c.ID)
			}
			idx.character[c.ID] = len(idx.character)
		}
	}
	for _, st := range d.staff {
		if _, dup := idx.staff[st.staff.ID]; dup {
			return nil, fmt.Errorf("%s: duplicate staff id %q", st.file, st.staff.ID)
		}
		idx.staff[st.staff.ID] = len(idx.staff)
	}
	return idx, nil
}

// buildWork is one flattened release, before it is written out.
type buildWork struct {
	kind        WorkKind
	id          string
	seriesRow   int
	number      int
	date        *model.Date
	year        int
	quarter     model.ReleaseSeason
	format      model.SpecialFormat
	episodes    int
	titles      model.Title
	externalIDs model.ExternalIDs
}

// flatten turns every series' seasons, movies and specials into works, in the
// order they appear in the records.
func (d *Dataset) flatten(idx *rows) ([]buildWork, error) {
	var out []buildWork
	for _, f := range d.files {
		f.rec.EachSeries(func(s *model.Series) {
			row := idx.series[s.ID]
			for _, sn := range s.Seasons {
				out = append(out, buildWork{
					kind: WorkSeason, id: sn.ID, seriesRow: row, number: sn.Number,
					date: sn.ReleaseDate, year: sn.ReleaseYear, quarter: sn.ReleaseSeason,
					episodes: len(sn.Episodes), titles: sn.Titles, externalIDs: sn.ExternalIDs,
				})
			}
			for _, m := range s.Movies {
				out = append(out, buildWork{
					kind: WorkMovie, id: m.ID, seriesRow: row,
					date: m.ReleaseDate, year: m.ReleaseYear,
					titles: m.Titles, externalIDs: m.ExternalIDs,
				})
			}
			for _, sp := range s.Specials {
				out = append(out, buildWork{
					kind: WorkSpecial, id: sp.ID, seriesRow: row,
					date: sp.ReleaseDate, year: sp.ReleaseYear, format: sp.Format,
					episodes: len(sp.Episodes), titles: sp.Titles, externalIDs: sp.ExternalIDs,
				})
			}
		})
	}
	return out, nil
}

// stats summarizes the dataset for the header section.
func (d *Dataset) stats(works []buildWork) Stats {
	var s Stats
	for _, f := range d.files {
		if f.rec.Franchise != nil {
			s.Franchises++
		}
		f.rec.EachSeries(func(series *model.Series) {
			s.Series++
			for i := range series.Seasons {
				s.Seasons++
				s.Episodes += len(series.Seasons[i].Episodes)
			}
			for i := range series.Specials {
				s.Episodes += len(series.Specials[i].Episodes)
			}
		})
		s.Characters += len(f.rec.Cast())
	}
	s.Staff = len(d.staff)
	for _, w := range works {
		if w.year == 0 {
			continue
		}
		if s.EarliestReleaseYear == 0 || w.year < s.EarliestReleaseYear {
			s.EarliestReleaseYear = w.year
		}
		if w.year > s.LatestReleaseYear {
			s.LatestReleaseYear = w.year
		}
	}
	return s
}

func (d *Dataset) writeStats(w io.Writer, s Stats) {
	fmt.Fprintf(w, "!%s\tfranchises=%d\tseries=%d\tseasons=%d\tepisodes=%d\tcharacters=%d\tstaff=%d\tearliest=%d\tlatest=%d\n",
		sectionStats, s.Franchises, s.Series, s.Seasons, s.Episodes,
		s.Characters, s.Staff, s.EarliestReleaseYear, s.LatestReleaseYear)
}

func (d *Dataset) writeFranchises(w io.Writer) {
	fmt.Fprintf(w, "!%s\tid\tfile\ttitles\n", sectionFranchises)
	for _, f := range d.files {
		if f.rec.Franchise == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			escape(f.rec.Franchise.ID), escape(f.name), packTitle(f.rec.Franchise.Titles))
	}
}

func (d *Dataset) writeSeries(w io.Writer, idx *rows) {
	fmt.Fprintf(w, "!%s\tid\tfile\tfranchise\ttitles\n", sectionSeries)
	for _, f := range d.files {
		franchise := ""
		if f.rec.Franchise != nil {
			franchise = formatInt(idx.franchise[f.rec.Franchise.ID] + 1)
		}
		f.rec.EachSeries(func(s *model.Series) {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				escape(s.ID), escape(f.name), franchise, packTitle(s.Titles))
		})
	}
}

// writeEntries emits the catalog: franchises and standalone series, in the
// order the files present them, each with the aggregates a browse row shows.
// The aggregates are stored rather than recomputed at boot so that the server
// does not have to reimplement the rollup, and so that a wrong figure is
// visible in the file's diff.
func (d *Dataset) writeEntries(w io.Writer, idx *rows, works []buildWork) {
	// Roll works up per series, then per franchise.
	type agg struct{ first, latest, works, episodes int }
	bySeries := make([]agg, len(idx.series))
	for _, wk := range works {
		a := &bySeries[wk.seriesRow]
		a.works++
		a.episodes += wk.episodes
		if wk.year == 0 {
			continue
		}
		if a.first == 0 || wk.year < a.first {
			a.first = wk.year
		}
		if wk.year > a.latest {
			a.latest = wk.year
		}
	}
	merge := func(dst *agg, src agg) {
		dst.works += src.works
		dst.episodes += src.episodes
		if src.first != 0 && (dst.first == 0 || src.first < dst.first) {
			dst.first = src.first
		}
		if src.latest > dst.latest {
			dst.latest = src.latest
		}
	}

	fmt.Fprintf(w, "!%s\tkind\trow\tfirst_year\tlatest_year\tworks\tepisodes\n", sectionEntries)
	for _, f := range d.files {
		if f.rec.Franchise != nil {
			var total agg
			for i := range f.rec.Franchise.Series {
				merge(&total, bySeries[idx.series[f.rec.Franchise.Series[i].ID]])
			}
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", entryFranchise,
				idx.franchise[f.rec.Franchise.ID]+1,
				formatInt(total.first), formatInt(total.latest),
				formatInt(total.works), formatInt(total.episodes))
		}
		f.rec.EachSeries(func(s *model.Series) {
			row := idx.series[s.ID]
			a := bySeries[row]
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", entrySeries, row+1,
				formatInt(a.first), formatInt(a.latest),
				formatInt(a.works), formatInt(a.episodes))
		})
	}
}

func writeWorks(w io.Writer, works []buildWork) {
	fmt.Fprintf(w, "!%s\tkind\tid\tseries\tnumber\tdate\tyear\tquarter\tformat\tepisodes\ttitles\tanilist\tanidb\ttmdb\ttvdb\twikidata\n", sectionWorks)
	for _, wk := range works {
		date := ""
		if wk.date != nil {
			date = wk.date.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			workKindCode(wk.kind), escape(wk.id), wk.seriesRow+1,
			formatInt(wk.number), date, formatInt(wk.year),
			escape(string(wk.quarter)), escape(string(wk.format)),
			formatInt(wk.episodes), packTitle(wk.titles),
			formatInt(wk.externalIDs.AnilistID), formatInt(wk.externalIDs.AnidbID),
			formatInt(wk.externalIDs.TmdbID), formatInt(wk.externalIDs.TvdbID),
			escape(wk.externalIDs.WikidataID))
	}
}

// writeCharacters records each character's names and the series it appears in.
// The series list is what ListCharacters(series_id) filters on; the file is
// what a detail request reopens to read the full record.
func (d *Dataset) writeCharacters(w io.Writer, idx *rows) {
	fmt.Fprintf(w, "!%s\tid\tfile\tnames\tseries\n", sectionCharacters)
	for _, f := range d.files {
		cast := f.rec.Cast()
		for i := range cast {
			c := &cast[i]
			var series []string
			seen := map[int]bool{}
			for _, a := range c.Appearances {
				row, ok := idx.series[a.SeriesID]
				if !ok || seen[row] {
					continue
				}
				seen[row] = true
				series = append(series, formatInt(row+1))
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				escape(c.ID), escape(f.name), packTitle(c.Names), strings.Join(series, "|"))
		}
	}
}

// writeStaff records each staff member's names and the languages they are
// credited in, which is all ListStaff needs to filter and render a row.
func (d *Dataset) writeStaff(w io.Writer) {
	langs := d.creditLanguages()
	fmt.Fprintf(w, "!%s\tid\tfile\tnames\tlangs\tanilist\tanidb\ttmdb\ttvdb\twikidata\n", sectionStaff)
	for _, st := range d.staff {
		list := langs[st.staff.ID]
		sort.Strings(list)
		escaped := make([]string, len(list))
		for i, l := range list {
			escaped[i] = escape(l)
		}
		e := st.staff.ExternalIDs
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			escape(st.staff.ID), escape(st.file), packTitle(st.staff.Names), strings.Join(escaped, "|"),
			formatInt(e.AnilistID), formatInt(e.AnidbID), formatInt(e.TmdbID), formatInt(e.TvdbID),
			escape(e.WikidataID))
	}
}

// creditLanguages collects the distinct languages each staff member is
// credited in.
func (d *Dataset) creditLanguages() map[string][]string {
	out := map[string][]string{}
	seen := map[string]map[string]bool{}
	d.eachCredit(func(staffID, language, _ string) {
		langs, ok := seen[staffID]
		if !ok {
			langs = map[string]bool{}
			seen[staffID] = langs
		}
		if langs[language] {
			return
		}
		langs[language] = true
		out[staffID] = append(out[staffID], language)
	})
	return out
}

// writeCredits emits the staff -> character reverse index: one row per staff
// member, character and language, with the series the casting covers.
func (d *Dataset) writeCredits(w io.Writer, idx *rows) {
	fmt.Fprintf(w, "!%s\tstaff\tcharacter\tlanguage\tseries\n", sectionCredits)

	// Grouped the way the store reads them — by character, then language — so
	// the rows for one staff member arrive together and in dataset order.
	for _, f := range d.files {
		cast := f.rec.Cast()
		for i := range cast {
			c := &cast[i]
			byStaff, order := creditsFor(c)
			for _, staffID := range order {
				staffRow, ok := idx.staff[staffID]
				if !ok {
					// A character cast to a staff member with no record. The
					// dataset lint catches this; the index skips it rather
					// than emitting a dangling row.
					continue
				}
				langs := byStaff[staffID]
				keys := make([]string, 0, len(langs))
				for lang := range langs {
					keys = append(keys, lang)
				}
				sort.Strings(keys)
				for _, lang := range keys {
					var series []string
					for _, id := range langs[lang] {
						if row, ok := idx.series[id]; ok {
							series = append(series, formatInt(row+1))
						}
					}
					fmt.Fprintf(w, "%d\t%d\t%s\t%s\n",
						staffRow+1, idx.character[c.ID]+1, escape(lang), strings.Join(series, "|"))
				}
			}
		}
	}
}

// creditsFor collects one character's castings as staff id -> language ->
// series ids, plus the staff ids in first-seen order so output is stable.
//
// A character's default voiceActors apply to every appearance that does not
// override them, so one credit is recorded per (staff, language) with the
// series it covers.
func creditsFor(c *model.Character) (map[string]map[string][]string, []string) {
	byStaff := map[string]map[string][]string{}
	var order []string
	record := func(staffID, language, seriesID string) {
		langs, ok := byStaff[staffID]
		if !ok {
			langs = map[string][]string{}
			byStaff[staffID] = langs
			order = append(order, staffID)
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
	return byStaff, order
}

// eachCredit calls fn for every (staff, language, series) casting in the
// dataset.
func (d *Dataset) eachCredit(fn func(staffID, language, seriesID string)) {
	for _, f := range d.files {
		cast := f.rec.Cast()
		for i := range cast {
			byStaff, order := creditsFor(&cast[i])
			for _, staffID := range order {
				for lang, series := range byStaff[staffID] {
					if len(series) == 0 {
						fn(staffID, lang, "")
						continue
					}
					for _, id := range series {
						fn(staffID, lang, id)
					}
				}
			}
		}
	}
}

// Build parses the dataset in fsys and returns an index over it, without
// writing a file. It is the in-memory equivalent of `make index` followed by
// Open, for callers that have no prebuilt index — tests, and tools reading a
// dataset they have just written.
//
// It parses every record, which is the cost the committed index exists to keep
// off the server's boot path.
func Build(fsys fs.FS) (*Index, error) {
	d, err := Load(fsys)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if _, err := d.WriteTo(&buf); err != nil {
		return nil, err
	}
	return Open(buf.String())
}
