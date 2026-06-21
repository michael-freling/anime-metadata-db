// Package build is the resolve → fill facts → compute numbers → apply
// overrides → validate pipeline that turns one authored override plus the open
// sources into a resolved dataset record.
package build

import (
	"fmt"
	"sort"
	"time"

	"github.com/michael-freling/anime-metadata-db/internal/model"
	"github.com/michael-freling/anime-metadata-db/internal/overrides"
	"github.com/michael-freling/anime-metadata-db/internal/sources/animelists"
	"github.com/michael-freling/anime-metadata-db/internal/sources/offlinedb"
	"github.com/michael-freling/anime-metadata-db/internal/sources/wikidata"
)

// Sources bundles the loaded open-data inputs the build reads from. Wikidata is
// only required when building characters/staff (R2).
type Sources struct {
	Offline   *offlinedb.Database
	AnimeList *animelists.AnimeList
	MovieSets *animelists.MovieSetList
	Wikidata  *wikidata.Entities
}

// Builder runs the build pipeline against a fixed set of sources.
type Builder struct {
	sources Sources
}

// New returns a Builder bound to the given sources.
func New(s Sources) *Builder { return &Builder{sources: s} }

// Build resolves one override into a dataset record and a report of
// low-confidence decisions. It fails on any unknown id, dangling reference or
// schema violation, so a successful build is always a valid dataset.
func (b *Builder) Build(o overrides.Override) (model.Record, *Report, error) {
	report := &Report{}
	var rec model.Record

	switch {
	case o.Franchise != nil:
		rec.Franchise = o.Franchise
	case o.Series != nil:
		rec.Series = o.Series
	default:
		return model.Record{}, nil, fmt.Errorf("override %q: empty record", o.Path)
	}

	var buildErr error
	rec.EachSeries(func(s *model.Series) {
		if buildErr != nil {
			return
		}
		if err := b.buildSeries(s, o.IsNumbered(s.ID), report); err != nil {
			buildErr = err
		}
	})
	if buildErr != nil {
		return model.Record{}, nil, buildErr
	}

	if err := validate(rec); err != nil {
		return model.Record{}, nil, err
	}

	// Fill the nested cast's names from Wikidata and default each character's
	// appearance to its enclosing series. Appearance/VA references are validated
	// in a second pass (ValidateCharacters), once the full R1 id universe is
	// known across all files.
	cast := rec.Cast()
	home := homeSeries(rec)
	for i := range cast {
		c := &cast[i]
		b.fillNames("character "+c.ID, &c.Names, c.ExternalIDs.WikidataID, report)
		defaultAppearances(c, home)
	}

	report.Sort()
	return rec, report, nil
}

// buildSeries fills facts for every node of a series and, when numbered,
// assigns a continuous absoluteNumber.
func (b *Builder) buildSeries(s *model.Series, numbered bool, report *Report) error {
	for i := range s.Seasons {
		if err := b.fillSeason(&s.Seasons[i], report); err != nil {
			return err
		}
	}
	for i := range s.Movies {
		if err := b.fillMovie(&s.Movies[i], report); err != nil {
			return err
		}
	}
	for i := range s.Specials {
		if err := b.fillSpecial(&s.Specials[i], report); err != nil {
			return err
		}
	}
	if numbered {
		assignAbsoluteNumbers(s)
	}
	return nil
}

// lookup resolves an AniList id against the offline database, failing on an
// unknown id (design Part 4, step 2).
func (b *Builder) lookup(entity string, anilistID int) (offlinedb.Anime, error) {
	if anilistID == 0 {
		return offlinedb.Anime{}, fmt.Errorf("%s: missing externalIds.anilistId", entity)
	}
	a, ok := b.sources.Offline.Lookup(anilistID)
	if !ok {
		return offlinedb.Anime{}, fmt.Errorf("%s: unknown AniList id %d", entity, anilistID)
	}
	return a, nil
}

// fillExternalIDs cross-maps AniDB and TVDB ids onto a node from the sources,
// without overwriting ids the override already set.
func (b *Builder) fillExternalIDs(ids *model.ExternalIDs, a offlinedb.Anime) {
	if ids.AnidbID == 0 {
		ids.AnidbID = a.AnidbID()
	}
	if ids.AnidbID != 0 && ids.TvdbID == 0 {
		if m, ok := b.sources.AnimeList.Offset(ids.AnidbID); ok && m.TvdbID != 0 {
			ids.TvdbID = m.TvdbID
		}
	}
}

// fillTitles merges inferred facts into a node's titles per field: an authored
// original or translation always wins, but any field the override left unset is
// filled from the source. So an override that sets only translations.en still
// gets a Japanese (`ja`) name and an `original` added automatically.
func fillTitles(entity string, dst *model.Title, a offlinedb.Anime, report *Report) {
	inferred := inferTitle(a)
	if dst.Original == "" && inferred.Original != "" {
		dst.Original = inferred.Original
	}
	for code, val := range inferred.Translations {
		if _, ok := dst.Translations[code]; ok {
			continue // override wins for this language
		}
		if dst.Translations == nil {
			dst.Translations = make(map[string]string)
		}
		dst.Translations[code] = val
		if code == "en" {
			report.add(entity, "titles", fmt.Sprintf("filled translations.en from source title %q (verify it is not just a romanization)", val))
		}
	}
	if dst.Original == "" {
		report.add(entity, "titles", "no native-script title found; original left empty")
	}
}

// fillReleaseSeason fills releaseYear/releaseSeason from the offline entry's
// animeSeason, preferring an explicit override, then the upstream value, then a
// derivation from releaseDate.
func fillReleaseSeason(year *int, season *model.ReleaseSeason, date *model.Date, a offlinedb.Anime) {
	if *year == 0 {
		if a.AnimeSeason.Year != 0 {
			*year = a.AnimeSeason.Year
		} else if date != nil {
			*year = date.Year()
		}
	}
	if *season == "" {
		if s := model.ReleaseSeason(a.AnimeSeason.Season); s.Valid() {
			*season = s
		} else if date != nil {
			*season = model.SeasonForDate(date.Time)
		}
	}
}

// fillSeason resolves and fills a single Season, generating its episode list
// from the upstream episode count when the override did not supply one.
func (b *Builder) fillSeason(s *model.Season, report *Report) error {
	entity := "season " + s.ID
	a, err := b.lookup(entity, s.ExternalIDs.AnilistID)
	if err != nil {
		return err
	}
	// A season is identified by its number/part; only complete a title the author
	// already started (e.g. an arc name), never fabricate one for a plain
	// "Season N" or a split-cour part — that just duplicates the series name or
	// surfaces an upstream "2nd Season"/mistranslated synonym.
	if !s.Titles.IsZero() {
		fillTitles(entity, &s.Titles, a, report)
	}
	fillReleaseSeason(&s.ReleaseYear, &s.ReleaseSeason, s.ReleaseDate, a)
	b.fillExternalIDs(&s.ExternalIDs, a)
	if len(s.Episodes) == 0 && a.Episodes > 0 {
		s.Episodes = make([]model.Episode, a.Episodes)
		for i := range s.Episodes {
			s.Episodes[i].AiredNumber = i + 1
		}
	}
	return nil
}

// fillMovie resolves and fills a single Movie.
func (b *Builder) fillMovie(m *model.Movie, report *Report) error {
	entity := "movie " + m.ID
	a, err := b.lookup(entity, m.ExternalIDs.AnilistID)
	if err != nil {
		return err
	}
	fillTitles(entity, &m.Titles, a, report)
	var season model.ReleaseSeason
	fillReleaseSeason(&m.ReleaseYear, &season, m.ReleaseDate, a)
	b.fillExternalIDs(&m.ExternalIDs, a)
	if m.AlternateCutOf == nil && m.ExternalIDs.AnidbID != 0 {
		if set, ok := b.sources.MovieSets.SetFor(m.ExternalIDs.AnidbID); ok {
			report.add(entity, "", fmt.Sprintf("belongs to movie set %q — consider grouping", set.Name))
		}
	}
	return nil
}

// fillSpecial resolves and fills a single Special, defaulting its format from
// the upstream media type.
func (b *Builder) fillSpecial(s *model.Special, report *Report) error {
	entity := "special " + s.ID
	a, err := b.lookup(entity, s.ExternalIDs.AnilistID)
	if err != nil {
		return err
	}
	fillTitles(entity, &s.Titles, a, report)
	var season model.ReleaseSeason
	fillReleaseSeason(&s.ReleaseYear, &season, s.ReleaseDate, a)
	b.fillExternalIDs(&s.ExternalIDs, a)
	if s.Format == "" {
		switch a.Type {
		case offlinedb.TypeONA:
			s.Format = model.FormatONA
		case offlinedb.TypeSpecial:
			s.Format = model.FormatSpecial
		default:
			s.Format = model.FormatOVA
		}
	}
	if len(s.Episodes) == 0 && a.Episodes > 0 {
		s.Episodes = make([]model.Episode, a.Episodes)
		for i := range s.Episodes {
			s.Episodes[i].AiredNumber = i + 1
		}
	}
	return nil
}

// numberingUnit is one element of a numbered series' linear order: a whole
// season (carrying its episodes) or an original movie.
type numberingUnit struct {
	key    time.Time
	number int
	part   int
	idx    int
	season *model.Season
	movie  *model.Movie
}

// assignAbsoluteNumbers assigns a continuous absoluteNumber across a numbered
// series: season episodes consume granular numbers, original movies each take a
// slot, all interleaved by release order (design §2.1–2.2).
func assignAbsoluteNumbers(s *model.Series) {
	units := make([]numberingUnit, 0, len(s.Seasons)+len(s.Movies))
	for i := range s.Seasons {
		sea := &s.Seasons[i]
		units = append(units, numberingUnit{
			key:    orderKey(sea.ReleaseDate, sea.ReleaseYear, sea.ReleaseSeason),
			number: sea.Number,
			part:   partOf(sea.Part),
			idx:    len(units),
			season: sea,
		})
	}
	for i := range s.Movies {
		mov := &s.Movies[i]
		if mov.AlternateCutOf != nil {
			continue // alternate cuts take no number; the Season carries it
		}
		units = append(units, numberingUnit{
			key:   orderKey(mov.ReleaseDate, mov.ReleaseYear, ""),
			idx:   len(units),
			movie: mov,
		})
	}

	sort.SliceStable(units, func(i, j int) bool {
		a, b := units[i], units[j]
		if !a.key.Equal(b.key) {
			return a.key.Before(b.key)
		}
		if a.number != b.number {
			return a.number < b.number
		}
		if a.part != b.part {
			return a.part < b.part
		}
		return a.idx < b.idx
	})

	counter := 1
	for _, u := range units {
		switch {
		case u.season != nil:
			for i := range u.season.Episodes {
				n := counter
				u.season.Episodes[i].AbsoluteNumber = &n
				counter++
			}
		case u.movie != nil:
			n := counter
			u.movie.AbsoluteNumber = &n
			counter++
		}
	}
}

// partOf dereferences a season part, treating a nil part as 0.
func partOf(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// seasonStartMonth maps an airing quarter to the month its cours typically
// begin, used to derive an ordering key when only year+season are known.
var seasonStartMonth = map[model.ReleaseSeason]time.Month{
	model.SeasonWinter: time.January,
	model.SeasonSpring: time.April,
	model.SeasonSummer: time.July,
	model.SeasonFall:   time.October,
}

// orderKey produces a sortable release key: the explicit date if present, else
// an approximation from year + airing quarter.
func orderKey(date *model.Date, year int, season model.ReleaseSeason) time.Time {
	if date != nil {
		return date.Time
	}
	month := time.January
	if m, ok := seasonStartMonth[season]; ok {
		month = m
	}
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}
