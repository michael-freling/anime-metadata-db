package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/internal/gen/anime/v1"
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
	if s1.GetReleaseSeason() != animev1.ReleaseSeason_RELEASE_SEASON_WINTER {
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
	if seasons[3].GetReleaseSeason() != animev1.ReleaseSeason_RELEASE_SEASON_UNSPECIFIED {
		t.Errorf("bogus season = %v", seasons[3].GetReleaseSeason())
	}

	movies := s.GetMovies()
	if len(movies) != 1 || movies[0].GetAlternateCutOf().GetSeasonId() != "aaa-s1" || movies[0].GetAbsoluteNumber() != 5 {
		t.Errorf("movie not converted: %+v", movies)
	}

	specials := s.GetSpecials()
	wantFormats := []animev1.SpecialFormat{
		animev1.SpecialFormat_SPECIAL_FORMAT_OVA,
		animev1.SpecialFormat_SPECIAL_FORMAT_ONA,
		animev1.SpecialFormat_SPECIAL_FORMAT_SPECIAL,
		animev1.SpecialFormat_SPECIAL_FORMAT_UNSPECIFIED,
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
	if resp.Msg.GetSeries().GetSeasons()[0].GetReleaseSeason() != animev1.ReleaseSeason_RELEASE_SEASON_FALL {
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
	if results[0].GetKind() != animev1.EntryKind_ENTRY_KIND_FRANCHISE || results[0].GetId() != "aaa" {
		t.Errorf("result0 = %+v", results[0])
	}
	if results[1].GetKind() != animev1.EntryKind_ENTRY_KIND_SERIES || results[1].GetFranchiseId() != "aaa" {
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
