package api

import (
	"sort"
	"strings"

	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// dateLayout is the canonical wire form for dates (matches model.Date).
const dateLayout = "2006-01-02"

// defaultLang is the fallback language when a request sends no Accept-Language.
const defaultLang = "en"

// localizer resolves model titles for one request's Accept-Language.
type localizer struct {
	lang string // primary language tag, lowercased (e.g. "en", "ja")
	full bool   // Accept-Language: * — also emit the full multilingual title
}

// newLocalizer parses an Accept-Language header value. "*" asks for every
// language; an empty header defaults to English; otherwise the first language
// tag wins (the q-weight and anything after it are ignored).
func newLocalizer(acceptLanguage string) localizer {
	first := strings.TrimSpace(acceptLanguage)
	if i := strings.IndexByte(first, ','); i >= 0 {
		first = first[:i]
	}
	if i := strings.IndexByte(first, ';'); i >= 0 {
		first = first[:i]
	}
	tag := strings.ToLower(strings.TrimSpace(first))
	switch tag {
	case "":
		return localizer{lang: defaultLang}
	case "*":
		return localizer{lang: defaultLang, full: true}
	default:
		return localizer{lang: tag}
	}
}

// title resolves t to a single display string and, in full mode, the complete
// multilingual title (nil otherwise).
func (l localizer) title(t model.Title) (string, *animev1.LocalizedTitle) {
	name := resolveTitle(t, l.lang)
	if l.full {
		return name, toLocalizedTitle(t)
	}
	return name, nil
}

// resolveTitle picks the best single title for lang. Order: the requested
// language (exact, then its primary subtag), then — for a non-English request —
// the native original, then English, then the native original, then any
// translation (deterministically). It returns "" only for an empty title.
func resolveTitle(t model.Title, lang string) string {
	if v := t.Translations[lang]; v != "" {
		return v
	}
	if p := primaryTag(lang); p != lang {
		if v := t.Translations[p]; v != "" {
			return v
		}
	}
	if lang != defaultLang && t.Original != "" {
		return t.Original
	}
	if v := t.Translations[defaultLang]; v != "" {
		return v
	}
	if t.Original != "" {
		return t.Original
	}
	return firstTranslation(t)
}

// primaryTag returns the primary subtag of a BCP-47 tag ("en-us" -> "en").
func primaryTag(tag string) string {
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		return tag[:i]
	}
	return tag
}

// firstTranslation returns a translation in deterministic key order, or "".
func firstTranslation(t model.Title) string {
	keys := make([]string, 0, len(t.Translations))
	for k := range t.Translations {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if t.Translations[k] != "" {
			return t.Translations[k]
		}
	}
	return ""
}

// toLocalizedTitle converts a model.Title, returning nil for a zero title.
func toLocalizedTitle(t model.Title) *animev1.LocalizedTitle {
	if t.IsZero() {
		return nil
	}
	return &animev1.LocalizedTitle{Original: t.Original, Translations: t.Translations}
}

// toExternalIDs converts cross-database ids, returning nil when none are set.
func toExternalIDs(e model.ExternalIDs) *animev1.ExternalIds {
	if e.IsZero() {
		return nil
	}
	return &animev1.ExternalIds{
		AnilistId:  int32(e.AnilistID),
		AnidbId:    int32(e.AnidbID),
		TmdbId:     int32(e.TmdbID),
		TvdbId:     int32(e.TvdbID),
		WikidataId: e.WikidataID,
	}
}

// toReleaseSeason maps a model release quarter to its proto enum.
func toReleaseSeason(s model.ReleaseSeason) animev1.ReleaseSeason {
	switch s {
	case model.SeasonWinter:
		return animev1.ReleaseSeason_RELEASE_SEASON_WINTER
	case model.SeasonSpring:
		return animev1.ReleaseSeason_RELEASE_SEASON_SPRING
	case model.SeasonSummer:
		return animev1.ReleaseSeason_RELEASE_SEASON_SUMMER
	case model.SeasonFall:
		return animev1.ReleaseSeason_RELEASE_SEASON_FALL
	default:
		return animev1.ReleaseSeason_RELEASE_SEASON_UNSPECIFIED
	}
}

// toSpecialFormat maps a model special format to its proto enum.
func toSpecialFormat(f model.SpecialFormat) animev1.SpecialFormat {
	switch f {
	case model.FormatOVA:
		return animev1.SpecialFormat_SPECIAL_FORMAT_OVA
	case model.FormatONA:
		return animev1.SpecialFormat_SPECIAL_FORMAT_ONA
	case model.FormatSpecial:
		return animev1.SpecialFormat_SPECIAL_FORMAT_SPECIAL
	default:
		return animev1.SpecialFormat_SPECIAL_FORMAT_UNSPECIFIED
	}
}

// toEntryKind maps a store catalog kind to its proto enum.
func toEntryKind(k EntryKind) animev1.EntryKind {
	if k == EntryFranchise {
		return animev1.EntryKind_ENTRY_KIND_FRANCHISE
	}
	return animev1.EntryKind_ENTRY_KIND_SERIES
}

// toInt32Ptr converts an optional int, preserving nil.
func toInt32Ptr(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

// toDate formats an optional model.Date as YYYY-MM-DD, returning "" for nil.
func toDate(d *model.Date) string {
	if d == nil {
		return ""
	}
	return d.Format(dateLayout)
}

// toEpisode converts one episode.
func toEpisode(e model.Episode) *animev1.Episode {
	return &animev1.Episode{
		AbsoluteNumber: toInt32Ptr(e.AbsoluteNumber),
		AiredNumber:    int32(e.AiredNumber),
		ReleaseDate:    toDate(e.ReleaseDate),
		Title:          e.Title,
	}
}

// toEpisodes converts a slice of episodes, returning nil for an empty input.
func toEpisodes(in []model.Episode) []*animev1.Episode {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.Episode, len(in))
	for i := range in {
		out[i] = toEpisode(in[i])
	}
	return out
}

// capped returns the first index.EmbeddedLimit elements of in, and the true
// length of in.
//
// Every collection embedded in a Get* response goes through this. Returning
// the count alongside the slice is what keeps the cap honest: a client can see
// that there is more and page it with the matching List RPC, rather than
// mistaking a truncated list for the whole of it.
func capped[T any](in []T) ([]T, int32) {
	total := int32(len(in))
	if len(in) > index.EmbeddedLimit {
		in = in[:index.EmbeddedLimit]
	}
	return in, total
}

// toSeason converts one season, resolving its title via loc.
func toSeason(loc localizer, s model.Season) *animev1.Season {
	title, full := loc.title(s.Titles)
	episodes, episodesTotal := capped(s.Episodes)
	return &animev1.Season{
		Id:             s.ID,
		Title:          title,
		LocalizedTitle: full,
		Number:         int32(s.Number),
		Part:           toInt32Ptr(s.Part),
		ReleaseDate:    toDate(s.ReleaseDate),
		ReleaseYear:    int32(s.ReleaseYear),
		ReleaseSeason:  toReleaseSeason(s.ReleaseSeason),
		ExternalIds:    toExternalIDs(s.ExternalIDs),
		Episodes:       toEpisodes(episodes),
		EpisodesTotal:  episodesTotal,
	}
}

// toMovie converts one movie, resolving its title via loc.
func toMovie(loc localizer, m model.Movie) *animev1.Movie {
	title, full := loc.title(m.Titles)
	var alt *animev1.AlternateCutOf
	if m.AlternateCutOf != nil {
		alt = &animev1.AlternateCutOf{SeasonId: m.AlternateCutOf.SeasonID, Episodes: m.AlternateCutOf.Episodes}
	}
	return &animev1.Movie{
		Id:             m.ID,
		Title:          title,
		LocalizedTitle: full,
		ReleaseDate:    toDate(m.ReleaseDate),
		ReleaseYear:    int32(m.ReleaseYear),
		ExternalIds:    toExternalIDs(m.ExternalIDs),
		AbsoluteNumber: toInt32Ptr(m.AbsoluteNumber),
		AlternateCutOf: alt,
	}
}

// toSpecial converts one special, resolving its title via loc.
func toSpecial(loc localizer, sp model.Special) *animev1.Special {
	title, full := loc.title(sp.Titles)
	episodes, episodesTotal := capped(sp.Episodes)
	return &animev1.Special{
		Id:             sp.ID,
		Title:          title,
		LocalizedTitle: full,
		Format:         toSpecialFormat(sp.Format),
		ReleaseDate:    toDate(sp.ReleaseDate),
		ReleaseYear:    int32(sp.ReleaseYear),
		ExternalIds:    toExternalIDs(sp.ExternalIDs),
		Episodes:       toEpisodes(episodes),
		EpisodesTotal:  episodesTotal,
		AbsoluteNumber: toInt32Ptr(sp.AbsoluteNumber),
	}
}

// toVoiceActor converts one voice-actor link, denormalizing the staff member's
// name so a client does not have to call GetStaff to display the cast.
func toVoiceActor(loc localizer, store *Store, va model.VoiceActor) *animev1.VoiceActor {
	out := &animev1.VoiceActor{StaffId: va.StaffID, Language: va.Language}
	if st, ok := store.Staff(va.StaffID); ok {
		out.StaffName = resolveTitle(st.Names, loc.lang)
	}
	return out
}

// toVoiceActors converts a slice of links, returning nil for an empty input.
func toVoiceActors(loc localizer, store *Store, in []model.VoiceActor) []*animev1.VoiceActor {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.VoiceActor, len(in))
	for i := range in {
		out[i] = toVoiceActor(loc, store, in[i])
	}
	return out
}

// toAppearance converts one Character <-> Series edge.
func toAppearance(loc localizer, store *Store, a model.CharacterAppearance) *animev1.CharacterAppearance {
	out := &animev1.CharacterAppearance{
		SeriesId:    a.SeriesID,
		SeriesTitle: seriesTitle(loc, store, a.SeriesID),
		VoiceActors: toVoiceActors(loc, store, a.VoiceActors),
		ExternalIds: toExternalIDs(a.ExternalIDs),
	}
	if len(a.Scope) > 0 {
		out.Scope = make([]*animev1.ScopeRef, len(a.Scope))
		for i, sc := range a.Scope {
			out.Scope[i] = &animev1.ScopeRef{
				SeasonId:  sc.SeasonID,
				MovieId:   sc.MovieID,
				SpecialId: sc.SpecialID,
			}
		}
	}
	return out
}

// toCharacter converts one character, resolving its name via loc.
func toCharacter(loc localizer, store *Store, c *model.Character) *animev1.Character {
	name, full := loc.title(c.Names)
	out := &animev1.Character{
		Id:            c.ID,
		Name:          name,
		LocalizedName: full,
		ExternalIds:   toExternalIDs(c.ExternalIDs),
		VoiceActors:   toVoiceActors(loc, store, c.VoiceActors),
	}
	appearances, appearancesTotal := capped(c.Appearances)
	out.AppearancesTotal = appearancesTotal
	if len(appearances) > 0 {
		out.Appearances = make([]*animev1.CharacterAppearance, len(appearances))
		for i := range appearances {
			out.Appearances[i] = toAppearance(loc, store, appearances[i])
		}
	}
	return out
}

// toCharacters converts a slice of characters, returning nil for an empty input.
func toCharacters(loc localizer, store *Store, in []*model.Character) []*animev1.Character {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.Character, len(in))
	for i, c := range in {
		out[i] = toCharacter(loc, store, c)
	}
	return out
}

// toStaff converts one staff member, resolving their name via loc.
func toStaff(loc localizer, st *model.Staff) *animev1.Staff {
	name, full := loc.title(st.Names)
	return &animev1.Staff{
		Id:            st.ID,
		Name:          name,
		LocalizedName: full,
		ExternalIds:   toExternalIDs(st.ExternalIDs),
	}
}

// toStaffList converts a slice of staff, returning nil for an empty input.
func toStaffList(loc localizer, in []*model.Staff) []*animev1.Staff {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.Staff, len(in))
	for i, st := range in {
		out[i] = toStaff(loc, st)
	}
	return out
}

// toStaffCredits converts a staff member's roles, resolving each character's
// name via loc.
func toStaffCredits(loc localizer, store *Store, in []StaffCredit) []*animev1.StaffCredit {
	if len(in) == 0 {
		return nil
	}
	out := make([]*animev1.StaffCredit, len(in))
	for i, credit := range in {
		out[i] = &animev1.StaffCredit{
			CharacterId:   credit.CharacterID,
			CharacterName: resolveTitle(credit.CharacterNames, loc.lang),
			Language:      credit.Language,
			SeriesIds:     credit.SeriesIDs,
			SeriesTitles:  seriesTitles(loc, store, credit.SeriesIDs),
		}
	}
	return out
}

// toSeries converts one series, its installments and the cast appearing in it.
func toSeries(loc localizer, store *Store, s *model.Series) (*animev1.Series, error) {
	title, full := loc.title(s.Titles)
	// The cast comes back already limited, with its own total, because it is
	// the one collection here that is not stored inside this record.
	castPage, err := store.CharactersPage(s.ID, "", "", index.EmbeddedLimit)
	if err != nil {
		return nil, err
	}
	seasons, seasonsTotal := capped(s.Seasons)
	movies, moviesTotal := capped(s.Movies)
	specials, specialsTotal := capped(s.Specials)
	out := &animev1.Series{
		Id:              s.ID,
		Title:           title,
		LocalizedTitle:  full,
		Characters:      toCharacters(loc, store, castPage.Items),
		CharactersTotal: int32(castPage.Total),
		SeasonsTotal:    seasonsTotal,
		MoviesTotal:     moviesTotal,
		SpecialsTotal:   specialsTotal,
	}
	if len(seasons) > 0 {
		out.Seasons = make([]*animev1.Season, len(seasons))
		for i := range seasons {
			out.Seasons[i] = toSeason(loc, seasons[i])
		}
	}
	if len(movies) > 0 {
		out.Movies = make([]*animev1.Movie, len(movies))
		for i := range movies {
			out.Movies[i] = toMovie(loc, movies[i])
		}
	}
	if len(specials) > 0 {
		out.Specials = make([]*animev1.Special, len(specials))
		for i := range specials {
			out.Specials[i] = toSpecial(loc, specials[i])
		}
	}
	return out, nil
}

// toFranchise converts one franchise and its nested series and watch orders.
// The cast is carried by each nested series, not by the franchise itself.
func toFranchise(loc localizer, store *Store, f *model.Franchise) (*animev1.Franchise, error) {
	title, full := loc.title(f.Titles)
	series, seriesTotal := capped(f.Series)
	watchOrders, watchOrdersTotal := capped(f.WatchOrders)
	out := &animev1.Franchise{
		Id: f.ID, Title: title, LocalizedTitle: full,
		SeriesTotal: seriesTotal, WatchOrdersTotal: watchOrdersTotal,
	}
	if len(series) > 0 {
		out.Series = make([]*animev1.Series, len(series))
		for i := range series {
			s, err := toSeries(loc, store, &series[i])
			if err != nil {
				return nil, err
			}
			out.Series[i] = s
		}
	}
	if len(watchOrders) > 0 {
		out.WatchOrders = make([]*animev1.WatchOrder, len(watchOrders))
		for i, wo := range watchOrders {
			entries := make([]*animev1.WatchOrderEntry, len(wo.Entries))
			for j, e := range wo.Entries {
				entries[j] = &animev1.WatchOrderEntry{Ref: e.Ref, Note: e.Note}
			}
			out.WatchOrders[i] = &animev1.WatchOrder{Name: wo.Name, Entries: entries}
		}
	}
	return out, nil
}

// toSearchResult converts a catalog entry to a search result, resolving its
// title via loc.
func toSearchResult(loc localizer, e CatalogEntry) *animev1.SearchResult {
	title, full := loc.title(e.Titles)
	return &animev1.SearchResult{
		Kind:           toEntryKind(e.Kind),
		Id:             e.ID,
		Title:          title,
		LocalizedTitle: full,
		FranchiseId:    e.FranchiseID,
	}
}

// toCatalogEntry renders a browse row: the entry plus its precomputed
// aggregates, and deliberately none of the nested structure.
func toCatalogEntry(loc localizer, e CatalogEntry) *animev1.CatalogEntry {
	title, full := loc.title(e.Titles)
	return &animev1.CatalogEntry{
		Kind:              toEntryKind(e.Kind),
		Id:                e.ID,
		Title:             title,
		LocalizedTitle:    full,
		FranchiseId:       e.FranchiseID,
		FirstReleaseYear:  int32(e.FirstReleaseYear),
		LatestReleaseYear: int32(e.LatestReleaseYear),
		Works:             int32(e.Works),
		Episodes:          int32(e.Episodes),
	}
}

// toWorkSummary renders one release, carrying its series' resolved title so a
// row can name its show without a second call.
func toWorkSummary(loc localizer, w Work) *animev1.WorkSummary {
	title, full := loc.title(w.Titles)
	seriesTitle, _ := loc.title(w.SeriesTitles)
	return &animev1.WorkSummary{
		Kind:           toWorkKind(w.Kind),
		Id:             w.ID,
		Title:          title,
		LocalizedTitle: full,
		SeriesId:       w.SeriesID,
		SeriesTitle:    seriesTitle,
		Number:         int32(w.Number),
		ReleaseDate:    toDate(w.ReleaseDate),
		ReleaseYear:    int32(w.ReleaseYear),
		ReleaseSeason:  toReleaseSeason(w.ReleaseSeason),
		Format:         toSpecialFormat(w.Format),
		EpisodeCount:   int32(w.EpisodeCount),
		ExternalIds:    toExternalIDs(w.ExternalIDs),
	}
}

// toWorkKind maps the internal work kind onto the wire enum.
func toWorkKind(k WorkKind) animev1.WorkKind {
	switch k {
	case WorkSeason:
		return animev1.WorkKind_WORK_KIND_SEASON
	case WorkMovie:
		return animev1.WorkKind_WORK_KIND_MOVIE
	case WorkSpecial:
		return animev1.WorkKind_WORK_KIND_SPECIAL
	}
	return animev1.WorkKind_WORK_KIND_UNSPECIFIED
}

// fromWorkKind maps the wire enum back onto the internal work kind. It returns
// nil for UNSPECIFIED, which is the filter's "match every kind".
func fromWorkKind(k animev1.WorkKind) *WorkKind {
	var w WorkKind
	switch k {
	case animev1.WorkKind_WORK_KIND_SEASON:
		w = WorkSeason
	case animev1.WorkKind_WORK_KIND_MOVIE:
		w = WorkMovie
	case animev1.WorkKind_WORK_KIND_SPECIAL:
		w = WorkSpecial
	default:
		return nil
	}
	return &w
}

// fromEntryKind maps the wire enum back onto the internal entry kind. It
// returns nil for UNSPECIFIED, which is the filter's "match both kinds".
func fromEntryKind(k animev1.EntryKind) *EntryKind {
	var e EntryKind
	switch k {
	case animev1.EntryKind_ENTRY_KIND_FRANCHISE:
		e = EntryFranchise
	case animev1.EntryKind_ENTRY_KIND_SERIES:
		e = EntrySeries
	default:
		return nil
	}
	return &e
}

// fromReleaseSeason maps the wire enum back onto the internal release season.
// UNSPECIFIED becomes the empty season, which matches everything.
func fromReleaseSeason(s animev1.ReleaseSeason) model.ReleaseSeason {
	switch s {
	case animev1.ReleaseSeason_RELEASE_SEASON_WINTER:
		return model.SeasonWinter
	case animev1.ReleaseSeason_RELEASE_SEASON_SPRING:
		return model.SeasonSpring
	case animev1.ReleaseSeason_RELEASE_SEASON_SUMMER:
		return model.SeasonSummer
	case animev1.ReleaseSeason_RELEASE_SEASON_FALL:
		return model.SeasonFall
	}
	return ""
}

// seriesTitle resolves a series id to its title for the request's language.
// Empty when the id names nothing, which the build's referential integrity
// check should already have prevented.
func seriesTitle(loc localizer, store *Store, seriesID string) string {
	titles, ok := store.SeriesTitle(seriesID)
	if !ok {
		return ""
	}
	title, _ := loc.title(titles)
	return title
}

// seriesTitles resolves each id to its title, positionally. An id that names
// nothing yields an empty string rather than being dropped, so the two slices
// stay aligned and a caller can always pair them up.
func seriesTitles(loc localizer, store *Store, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = seriesTitle(loc, store, id)
	}
	return out
}
