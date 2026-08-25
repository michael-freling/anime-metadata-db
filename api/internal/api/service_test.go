package api

import (
	"context"
	"slices"
	"testing"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
)

// newTestService builds a Service over the standard fixtures.
func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(mustStore(t), "test-version")
}

func TestListFranchises(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.ListFranchises(context.Background(), connect.NewRequest(&animev1.ListFranchisesRequest{}))
	if err != nil {
		t.Fatalf("ListFranchises: %v", err)
	}
	got := resp.Msg.GetFranchises()
	if len(got) != 1 {
		t.Fatalf("got %d franchises, want 1", len(got))
	}
	f := got[0]
	// Default request (no Accept-Language) resolves the title to English and
	// omits the full multilingual set.
	if f.GetId() != "aaa" || f.GetTitle() != "Alpha Franchise" {
		t.Errorf("unexpected franchise %+v", f)
	}
	if f.GetLocalizedTitle() != nil {
		t.Errorf("localized_title should be omitted without Accept-Language: *, got %+v", f.GetLocalizedTitle())
	}
	// Watch orders survive conversion.
	if len(f.GetWatchOrders()) != 1 || f.GetWatchOrders()[0].GetEntries()[0].GetNote() != "start here" {
		t.Errorf("watch orders not converted: %+v", f.GetWatchOrders())
	}

	// Inspect the nested series to exercise every converter branch.
	series := f.GetSeries()
	if len(series) != 1 {
		t.Fatalf("got %d series, want 1", len(series))
	}
	s := series[0]

	seasons := s.GetSeasons()
	if len(seasons) != 4 {
		t.Fatalf("got %d seasons, want 4", len(seasons))
	}
	s1 := seasons[0]
	if s1.GetReleaseSeason() != animev1.ReleaseSeason_WINTER {
		t.Errorf("season1 release season = %v", s1.GetReleaseSeason())
	}
	if s1.GetPart() != 1 || s1.GetReleaseDate() != "2006-01-06" {
		t.Errorf("season1 part/date = %d/%q", s1.GetPart(), s1.GetReleaseDate())
	}
	if e := s1.GetExternalIds(); e.GetAnilistId() != 1 || e.GetTvdbId() != 4 || e.GetWikidataId() != "Q1" {
		t.Errorf("season1 external ids = %+v", e)
	}
	eps := s1.GetEpisodes()
	if len(eps) != 2 {
		t.Fatalf("got %d episodes, want 2", len(eps))
	}
	if eps[0].GetAbsoluteNumber() != 1 || eps[0].GetTitle() != "Pilot" || eps[0].GetReleaseDate() != "2006-01-06" {
		t.Errorf("episode0 = %+v", eps[0])
	}
	if eps[1].AbsoluteNumber != nil {
		t.Errorf("episode1 absoluteNumber should be nil, got %v", eps[1].GetAbsoluteNumber())
	}
	// Unknown release season maps to UNSPECIFIED.
	if seasons[3].GetReleaseSeason() != animev1.ReleaseSeason_SEASON_UNSPECIFIED {
		t.Errorf("bogus season = %v", seasons[3].GetReleaseSeason())
	}

	movies := s.GetMovies()
	if len(movies) != 1 || movies[0].GetAlternateCutOf().GetSeasonId() != "aaa-s1" || movies[0].GetAbsoluteNumber() != 5 {
		t.Errorf("movie not converted: %+v", movies)
	}

	specials := s.GetSpecials()
	wantFormats := []animev1.SpecialFormat{
		animev1.SpecialFormat_FORMAT_OVA,
		animev1.SpecialFormat_FORMAT_ONA,
		animev1.SpecialFormat_FORMAT_SPECIAL,
		animev1.SpecialFormat_FORMAT_UNSPECIFIED,
	}
	if len(specials) != len(wantFormats) {
		t.Fatalf("got %d specials, want %d", len(specials), len(wantFormats))
	}
	for i, want := range wantFormats {
		if specials[i].GetFormat() != want {
			t.Errorf("special %d format = %v, want %v", i, specials[i].GetFormat(), want)
		}
	}
	if specials[0].GetAbsoluteNumber() != 6 || len(specials[0].GetEpisodes()) != 1 {
		t.Errorf("special0 = %+v", specials[0])
	}
}

func TestGetFranchise(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetFranchise(context.Background(), connect.NewRequest(&animev1.GetFranchiseRequest{Id: "aaa"}))
	if err != nil {
		t.Fatalf("GetFranchise: %v", err)
	}
	if resp.Msg.GetFranchise().GetId() != "aaa" {
		t.Errorf("got %q", resp.Msg.GetFranchise().GetId())
	}
}

func TestGetFranchiseErrors(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name string
		id   string
		want connect.Code
	}{
		{"empty id", "", connect.CodeInvalidArgument},
		{"not found", "missing", connect.CodeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetFranchise(context.Background(), connect.NewRequest(&animev1.GetFranchiseRequest{Id: tc.id}))
			if connect.CodeOf(err) != tc.want {
				t.Errorf("code = %v, want %v (err=%v)", connect.CodeOf(err), tc.want, err)
			}
		})
	}
}

func TestGetSeries(t *testing.T) {
	svc := newTestService(t)
	// Standalone series: empty franchise id, FALL season converted.
	resp, err := svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "zzz"}))
	if err != nil {
		t.Fatalf("GetSeries(zzz): %v", err)
	}
	if resp.Msg.GetFranchiseId() != "" {
		t.Errorf("standalone franchise id = %q, want empty", resp.Msg.GetFranchiseId())
	}
	if resp.Msg.GetSeries().GetSeasons()[0].GetReleaseSeason() != animev1.ReleaseSeason_FALL {
		t.Errorf("zzz season = %v", resp.Msg.GetSeries().GetSeasons()[0].GetReleaseSeason())
	}

	// Series under a franchise reports the owner.
	resp, err = svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "aaa-main"}))
	if err != nil {
		t.Fatalf("GetSeries(aaa-main): %v", err)
	}
	if resp.Msg.GetFranchiseId() != "aaa" {
		t.Errorf("franchise id = %q, want aaa", resp.Msg.GetFranchiseId())
	}

	// Minimal series converts with empty titles and no installments.
	resp, err = svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "minimal"}))
	if err != nil {
		t.Fatalf("GetSeries(minimal): %v", err)
	}
	if resp.Msg.GetSeries().GetTitle() != "" || resp.Msg.GetSeries().GetLocalizedTitle() != nil || len(resp.Msg.GetSeries().GetSeasons()) != 0 {
		t.Errorf("minimal series not empty: %+v", resp.Msg.GetSeries())
	}
}

func TestGetSeriesErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: ""})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("empty id code = %v", connect.CodeOf(err))
	}
	if _, err := svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "nope"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("missing code = %v", connect.CodeOf(err))
	}
}

func TestSearch(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.Search(context.Background(), connect.NewRequest(&animev1.SearchRequest{Query: "alpha"}))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	results := resp.Msg.GetResults()
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].GetKind() != animev1.EntryKind_FRANCHISE || results[0].GetId() != "aaa" {
		t.Errorf("result0 = %+v", results[0])
	}
	if results[1].GetKind() != animev1.EntryKind_SERIES || results[1].GetFranchiseId() != "aaa" {
		t.Errorf("result1 = %+v", results[1])
	}
}

func TestGetHealth(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetHealth(context.Background(), connect.NewRequest(&animev1.GetHealthRequest{}))
	if err != nil {
		t.Fatalf("GetHealth: %v", err)
	}
	if resp.Msg.GetStatus() != "ok" || resp.Msg.GetVersion() != "test-version" {
		t.Errorf("health = %+v", resp.Msg)
	}
	if st := resp.Msg.GetStats(); st.GetFranchises() != 1 || st.GetSeries() != 3 || st.GetSeasons() != 5 || st.GetEpisodes() != 3 {
		t.Errorf("stats = %+v", resp.Msg.GetStats())
	}
}

// reqWithLang builds a request carrying an Accept-Language header.
func reqWithLang[T any](msg *T, lang string) *connect.Request[T] {
	r := connect.NewRequest(msg)
	r.Header().Set("Accept-Language", lang)
	return r
}

func TestGetFranchiseLanguage(t *testing.T) {
	svc := newTestService(t)

	// A Japanese request with no ja translation falls back to the native original.
	resp, err := svc.GetFranchise(context.Background(), reqWithLang(&animev1.GetFranchiseRequest{Id: "aaa"}, "ja"))
	if err != nil {
		t.Fatalf("GetFranchise(ja): %v", err)
	}
	if got := resp.Msg.GetFranchise().GetTitle(); got != "エー" {
		t.Errorf("ja title = %q, want native エー", got)
	}
	if resp.Msg.GetFranchise().GetLocalizedTitle() != nil {
		t.Error("ja request should not include the full localized_title")
	}

	// Accept-Language: * returns the English title plus the full multilingual set.
	resp, err = svc.GetFranchise(context.Background(), reqWithLang(&animev1.GetFranchiseRequest{Id: "aaa"}, "*"))
	if err != nil {
		t.Fatalf("GetFranchise(*): %v", err)
	}
	f := resp.Msg.GetFranchise()
	if f.GetTitle() != "Alpha Franchise" {
		t.Errorf("* title = %q, want English default", f.GetTitle())
	}
	lt := f.GetLocalizedTitle()
	if lt.GetOriginal() != "エー" || lt.GetTranslations()["en"] != "Alpha Franchise" {
		t.Errorf("* localized_title = %+v", lt)
	}
	// A node with no title still omits localized_title even in full mode.
	if f.GetSeries()[0].GetSeasons()[0].GetLocalizedTitle() != nil {
		t.Error("empty-title season should have nil localized_title")
	}
}

func TestGetCharacter(t *testing.T) {
	svc := newTestService(t)

	resp, err := svc.GetCharacter(context.Background(), connect.NewRequest(&animev1.GetCharacterRequest{Id: "alpha-hero"}))
	if err != nil {
		t.Fatalf("GetCharacter: %v", err)
	}
	c := resp.Msg.GetCharacter()
	if c.GetId() != "alpha-hero" || c.GetName() != "Alpha Hero" {
		t.Errorf("character = %+v", c)
	}
	if c.GetLocalizedName() != nil {
		t.Error("localized_name should be omitted without Accept-Language: *")
	}
	if c.GetExternalIds().GetWikidataId() != "Q100" {
		t.Errorf("external ids = %+v", c.GetExternalIds())
	}
	// The default cast carries the staff member's resolved name.
	vas := c.GetVoiceActors()
	if len(vas) != 1 || vas[0].GetStaffId() != "va-one" || vas[0].GetStaffName() != "Voice One" {
		t.Errorf("voice actors = %+v", vas)
	}

	appearances := c.GetAppearances()
	if len(appearances) != 2 {
		t.Fatalf("got %d appearances, want 2", len(appearances))
	}
	// Scope covers all three installment kinds, and each carries a label so a
	// client can name the installment without showing a reader its id. A
	// numbered season usually has no title of its own, which is what number is
	// for; a movie or special has no number, which is what the title is for.
	scope := appearances[0].GetScope()
	if len(scope) != 3 || scope[0].GetSeasonId() != "aaa-s1" || scope[1].GetMovieId() != "aaa-movie" || scope[2].GetSpecialId() != "aaa-ova" {
		t.Fatalf("scope = %+v", scope)
	}
	if scope[0].GetNumber() != 1 || scope[0].GetTitle() != "" {
		t.Errorf("season scope = %+v, want number 1 and no title of its own", scope[0])
	}
	if scope[1].GetTitle() != "Alpha Movie" || scope[1].GetNumber() != 0 {
		t.Errorf("movie scope = %+v, want its title and no number", scope[1])
	}
	// An installment with neither a title nor a number — this fixture's OVA —
	// resolves to no label at all. The client then shows the series alone
	// rather than inventing a name or printing the id, which is the least
	// misleading of the options available.
	if scope[2].GetTitle() != "" || scope[2].GetNumber() != 0 {
		t.Errorf("untitled special scope = %+v, want no label", scope[2])
	}
	// An appearance that lists no cast of its own still reports the character's:
	// the response carries the cast that actually applies, so a caller never has
	// to merge two lists itself.
	if plain := appearances[0].GetVoiceActors(); len(plain) != 1 || plain[0].GetStaffId() != "va-one" {
		t.Errorf("appearance with no cast of its own = %+v, want the character's", plain)
	}
	// The second appearance adds a voice actor rather than replacing the
	// character's — an English dub does not unseat the original cast — so it
	// reports both, the added one being an unnamed staff member. Each is
	// marked with where it came from, so a client can render the addition
	// without restating the constant cast on every row.
	both := appearances[1].GetVoiceActors()
	if len(both) != 2 || both[0].GetStaffId() != "va-one" || both[1].GetStaffId() != "va-two" ||
		both[1].GetStaffName() != "" {
		t.Fatalf("appearance cast = %+v, want the character's plus its own", both)
	}
	if !both[0].GetThroughout() || both[1].GetThroughout() {
		t.Errorf("cast provenance = %v/%v, want the character's marked and the appearance's not",
			both[0].GetThroughout(), both[1].GetThroughout())
	}
	// An appearance that adds nobody reports the character's cast, marked.
	if plain := appearances[0].GetVoiceActors(); !plain[0].GetThroughout() {
		t.Errorf("inherited cast should be marked as holding throughout: %+v", plain[0])
	}
	// Asked without a series, a character's own cast holds throughout by
	// definition, so nothing is marked and the field stays absent on the wire.
	for _, va := range c.GetVoiceActors() {
		if va.GetThroughout() {
			t.Errorf("unscoped character cast should not be marked: %+v", va)
		}
	}
	if appearances[1].GetExternalIds().GetAnilistId() != 99 {
		t.Errorf("appearance external ids = %+v", appearances[1].GetExternalIds())
	}

	// Accept-Language: * adds the full multilingual name.
	resp, err = svc.GetCharacter(context.Background(), reqWithLang(&animev1.GetCharacterRequest{Id: "alpha-hero"}, "*"))
	if err != nil {
		t.Fatalf("GetCharacter(*): %v", err)
	}
	if got := resp.Msg.GetCharacter().GetLocalizedName().GetOriginal(); got != "アルファ" {
		t.Errorf("localized original = %q", got)
	}
	// A Japanese request resolves the staff name to their native form.
	resp, err = svc.GetCharacter(context.Background(), reqWithLang(&animev1.GetCharacterRequest{Id: "alpha-hero"}, "ja"))
	if err != nil {
		t.Fatalf("GetCharacter(ja): %v", err)
	}
	if got := resp.Msg.GetCharacter().GetVoiceActors()[0].GetStaffName(); got != "声優一" {
		t.Errorf("ja staff name = %q, want 声優一", got)
	}
}

// Which cast a character reports depends on what was asked. Scoped to a series
// it is that series' cast; unscoped there is no series to be specific to, so it
// is only the cast that holds throughout, and the per-series additions are on
// the appearances.
func TestCharacterCastIsScopedToTheSeriesAsked(t *testing.T) {
	svc := newTestService(t)
	staffIDs := func(vas []*animev1.VoiceActor) []string {
		out := make([]string, len(vas))
		for i, va := range vas {
			out[i] = va.GetStaffId()
		}
		return out
	}
	for _, tc := range []struct {
		seriesID string
		want     []string
	}{
		{"", []string{"va-one"}},              // every character: no series in question
		{"aaa-main", []string{"va-one"}},      // nobody is cast only here
		{"zzz", []string{"va-one", "va-two"}}, // va-two is cast only here
	} {
		resp, err := svc.ListCharacters(context.Background(), connect.NewRequest(
			&animev1.ListCharactersRequest{SeriesId: tc.seriesID, Query: "Alpha Hero"}))
		if err != nil {
			t.Fatalf("ListCharacters(%q): %v", tc.seriesID, err)
		}
		chars := resp.Msg.GetCharacters()
		if len(chars) != 1 {
			t.Fatalf("ListCharacters(%q) returned %d characters, want 1", tc.seriesID, len(chars))
		}
		if got := staffIDs(chars[0].GetVoiceActors()); !slices.Equal(got, tc.want) {
			t.Errorf("ListCharacters(seriesId=%q) cast = %v, want %v", tc.seriesID, got, tc.want)
		}
	}
}

func TestGetCharacterErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetCharacter(context.Background(), connect.NewRequest(&animev1.GetCharacterRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("empty id: got %v, want invalid_argument", err)
	}
	if _, err := svc.GetCharacter(context.Background(), connect.NewRequest(&animev1.GetCharacterRequest{Id: "nobody"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("unknown id: got %v, want not_found", err)
	}
}

func TestListCharacters(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name     string
		req      *animev1.ListCharactersRequest
		wantIDs  []string
		wantCode connect.Code
	}{
		{"whole cast", &animev1.ListCharactersRequest{}, []string{"alpha-hero", "zed-friend"}, 0},
		{"by series", &animev1.ListCharactersRequest{SeriesId: "aaa-main"}, []string{"alpha-hero"}, 0},
		{"series with no cast", &animev1.ListCharactersRequest{SeriesId: "minimal"}, nil, 0},
		{"limit", &animev1.ListCharactersRequest{Limit: 1}, []string{"alpha-hero"}, 0},
		{"unknown series", &animev1.ListCharactersRequest{SeriesId: "nope"}, nil, connect.CodeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.ListCharacters(context.Background(), connect.NewRequest(tc.req))
			if tc.wantCode != 0 {
				if connect.CodeOf(err) != tc.wantCode {
					t.Fatalf("got %v, want code %v", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListCharacters: %v", err)
			}
			got := resp.Msg.GetCharacters()
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d characters, want %d", len(got), len(tc.wantIDs))
			}
			for i, c := range got {
				if c.GetId() != tc.wantIDs[i] {
					t.Errorf("character %d = %q, want %q", i, c.GetId(), tc.wantIDs[i])
				}
			}
		})
	}
}

func TestGetStaff(t *testing.T) {
	svc := newTestService(t)

	resp, err := svc.GetStaff(context.Background(), connect.NewRequest(&animev1.GetStaffRequest{Id: "va-one"}))
	if err != nil {
		t.Fatalf("GetStaff: %v", err)
	}
	st := resp.Msg.GetStaff()
	if st.GetId() != "va-one" || st.GetName() != "Voice One" || st.GetExternalIds().GetWikidataId() != "Q200" {
		t.Errorf("staff = %+v", st)
	}
	if st.GetLocalizedName() != nil {
		t.Error("localized_name should be omitted without Accept-Language: *")
	}

	credits := resp.Msg.GetCredits()
	if len(credits) != 2 {
		t.Fatalf("got %d credits, want 2: %+v", len(credits), credits)
	}
	// va-one is the character's own voice actor, so the credit covers every
	// series they appear in — including the one that adds an English actor.
	if credits[0].GetCharacterId() != "alpha-hero" || credits[0].GetCharacterName() != "Alpha Hero" ||
		credits[0].GetLanguage() != "ja" || len(credits[0].GetSeriesIds()) != 2 {
		t.Errorf("credit 0 = %+v", credits[0])
	}
	if credits[1].GetCharacterId() != "zed-friend" || credits[1].GetSeriesIds() != nil {
		t.Errorf("credit 1 = %+v", credits[1])
	}

	// An unnamed staff member with only an override credit still resolves.
	resp, err = svc.GetStaff(context.Background(), reqWithLang(&animev1.GetStaffRequest{Id: "va-two"}, "*"))
	if err != nil {
		t.Fatalf("GetStaff(va-two): %v", err)
	}
	if resp.Msg.GetStaff().GetName() != "" || resp.Msg.GetStaff().GetLocalizedName() != nil {
		t.Errorf("unnamed staff = %+v", resp.Msg.GetStaff())
	}
	if len(resp.Msg.GetCredits()) != 1 || resp.Msg.GetCredits()[0].GetSeriesIds()[0] != "zzz" {
		t.Errorf("va-two credits = %+v", resp.Msg.GetCredits())
	}
}

// A credit carries the title of each series it covers, positionally aligned
// with series_ids, so a roles list can name the shows without a call per id.
//
// Worth its own test because the frontend falls back to humanizing the id when
// a title is missing — so a desynced or empty title would render as something
// plausible rather than as an error, and ship unnoticed.
func TestGetStaffCreditsCarrySeriesTitles(t *testing.T) {
	svc := newTestService(t)

	resp, err := svc.GetStaff(context.Background(), connect.NewRequest(&animev1.GetStaffRequest{Id: "va-one"}))
	if err != nil {
		t.Fatalf("GetStaff: %v", err)
	}
	for _, c := range resp.Msg.GetCredits() {
		if len(c.GetSeriesTitles()) != len(c.GetSeriesIds()) {
			t.Errorf("credit %q: %d titles for %d ids — the two must stay aligned",
				c.GetCharacterId(), len(c.GetSeriesTitles()), len(c.GetSeriesIds()))
		}
	}
	// The character's own voice actor covers every series they appear in, so
	// both are named and both carry a resolved title.
	credit := resp.Msg.GetCredits()[0]
	if len(credit.GetSeriesIds()) != 2 || credit.GetSeriesIds()[0] != "aaa-main" ||
		credit.GetSeriesIds()[1] != "zzz" {
		t.Fatalf("credit series ids = %v, want [aaa-main zzz]", credit.GetSeriesIds())
	}
	if credit.GetSeriesTitles()[0] != "Alpha Main" || credit.GetSeriesTitles()[1] != "Zed Standalone" {
		t.Errorf("series titles = %v", credit.GetSeriesTitles())
	}

	// Resolved for the request's language, like every other title. zzz is the
	// one fixture series with a native original, so asking in Japanese must
	// return it — a credit built with the wrong localizer would still return
	// the English title here and pass every assertion above.
	resp, err = svc.GetStaff(context.Background(), reqWithLang(&animev1.GetStaffRequest{Id: "va-two"}, "ja"))
	if err != nil {
		t.Fatalf("GetStaff(va-two): %v", err)
	}
	credit = resp.Msg.GetCredits()[0]
	if credit.GetSeriesIds()[0] != "zzz" || credit.GetSeriesTitles()[0] != "ゼッド" {
		t.Errorf("ja credit = ids %v, titles %v; want [zzz] / [ゼッド]",
			credit.GetSeriesIds(), credit.GetSeriesTitles())
	}
}

func TestGetStaffErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.GetStaff(context.Background(), connect.NewRequest(&animev1.GetStaffRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("empty id: got %v, want invalid_argument", err)
	}
	if _, err := svc.GetStaff(context.Background(), connect.NewRequest(&animev1.GetStaffRequest{Id: "nobody"})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Errorf("unknown id: got %v, want not_found", err)
	}
}

func TestListStaff(t *testing.T) {
	svc := newTestService(t)
	tests := []struct {
		name    string
		req     *animev1.ListStaffRequest
		wantIDs []string
	}{
		{"everyone", &animev1.ListStaffRequest{}, []string{"va-one", "va-two"}},
		{"by language", &animev1.ListStaffRequest{Language: "ja"}, []string{"va-one"}},
		{"language with no credits", &animev1.ListStaffRequest{Language: "fr"}, nil},
		{"limit", &animev1.ListStaffRequest{Limit: 1}, []string{"va-one"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := svc.ListStaff(context.Background(), connect.NewRequest(tc.req))
			if err != nil {
				t.Fatalf("ListStaff: %v", err)
			}
			got := resp.Msg.GetStaff()
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d staff, want %d", len(got), len(tc.wantIDs))
			}
			for i, st := range got {
				if st.GetId() != tc.wantIDs[i] {
					t.Errorf("staff %d = %q, want %q", i, st.GetId(), tc.wantIDs[i])
				}
			}
		})
	}
}

// A series response carries the cast appearing in it, including a character
// whose home record is a different file.
func TestGetSeriesIncludesCast(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "zzz"}))
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	cast := resp.Msg.GetSeries().GetCharacters()
	if len(cast) != 1 || cast[0].GetId() != "alpha-hero" {
		t.Fatalf("zzz cast = %+v, want alpha-hero", cast)
	}
	// A series' cast is that series' cast: va-two is cast only in zzz, and a
	// page about zzz that left them out would be listing the wrong actors.
	// Marked, so "who is specific to this series" is still answerable.
	vas := cast[0].GetVoiceActors()
	if len(vas) != 2 || vas[0].GetStaffId() != "va-one" || vas[1].GetStaffId() != "va-two" {
		t.Fatalf("zzz cast voice actors = %+v, want va-one and va-two", vas)
	}
	if !vas[0].GetThroughout() || vas[1].GetThroughout() {
		t.Errorf("series-scoped cast provenance = %v/%v, want the character's marked only",
			vas[0].GetThroughout(), vas[1].GetThroughout())
	}
	// A series with no cast carries none.
	resp, err = svc.GetSeries(context.Background(), connect.NewRequest(&animev1.GetSeriesRequest{Id: "minimal"}))
	if err != nil {
		t.Fatalf("GetSeries(minimal): %v", err)
	}
	if got := resp.Msg.GetSeries().GetCharacters(); got != nil {
		t.Errorf("minimal cast = %+v, want nil", got)
	}
}
