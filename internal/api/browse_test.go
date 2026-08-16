package api

import (
	"context"
	"encoding/base64"
	"testing"

	"connectrpc.com/connect"

	animev1 "github.com/michael-freling/anime-metadata-db/internal/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/internal/model"
)

// entryByID finds a catalog entry in a page by id.
func entryByID(t *testing.T, page Page[CatalogEntry], id string) CatalogEntry {
	t.Helper()
	for _, e := range page.Items {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("entry %q not in page", id)
	return CatalogEntry{}
}

func TestCatalogAggregates(t *testing.T) {
	s := mustStore(t)
	page, err := s.Catalog(nil, "", 0)
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	// One franchise plus three series.
	if page.Total != 4 {
		t.Fatalf("total = %d, want 4", page.Total)
	}

	// The franchise rolls up its series: 4 seasons + 1 movie + 4 specials, with
	// 3 episodes, spanning the 2006 season to the 2010 movie.
	f := entryByID(t, page, "aaa")
	if f.Works != 9 || f.Episodes != 3 || f.FirstReleaseYear != 2006 || f.LatestReleaseYear != 2010 {
		t.Errorf("franchise aggregate = %+v", f)
	}
	// The series under it aggregates to the same figures.
	if m := entryByID(t, page, "aaa-main"); m.Works != f.Works || m.Episodes != f.Episodes {
		t.Errorf("series aggregate = %+v, want same as franchise", m)
	}
	// A series whose only season carries no year reports no span rather than
	// a bogus year 0..0 range being mistaken for a real one elsewhere.
	if z := entryByID(t, page, "zzz"); z.Works != 1 || z.FirstReleaseYear != 0 || z.LatestReleaseYear != 0 {
		t.Errorf("zzz aggregate = %+v", z)
	}
	// A series with nothing under it contributes no works.
	if empty := entryByID(t, page, "minimal"); empty.Works != 0 || empty.Episodes != 0 {
		t.Errorf("minimal aggregate = %+v", empty)
	}
}

// The dataset's own year span, which the UI uses as the floor below which a
// year cannot be real data. The fixture spans the 2006 season to the 2010
// movie; the untitled zzz season carries no year and must not drag the floor
// down to 0.
func TestStatsReleaseYearSpan(t *testing.T) {
	s := mustStore(t)
	got := s.Stats()
	if got.EarliestReleaseYear != 2006 {
		t.Errorf("earliest = %d, want 2006", got.EarliestReleaseYear)
	}
	if got.LatestReleaseYear != 2010 {
		t.Errorf("latest = %d, want 2010", got.LatestReleaseYear)
	}
}

func TestCatalogKindFilter(t *testing.T) {
	s := mustStore(t)
	for _, tc := range []struct {
		name string
		kind EntryKind
		want int
	}{
		{"franchises", EntryFranchise, 1},
		{"series", EntrySeries, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind := tc.kind
			page, err := s.Catalog(&kind, "", 0)
			if err != nil {
				t.Fatalf("Catalog: %v", err)
			}
			if page.Total != tc.want {
				t.Fatalf("total = %d, want %d", page.Total, tc.want)
			}
			for _, e := range page.Items {
				if e.Kind != tc.kind {
					t.Errorf("entry %q has kind %v", e.ID, e.Kind)
				}
			}
		})
	}
}

// TestCatalogPagingCoversEverythingOnce is the property that matters for a
// browse UI: walking the pages must yield every entry exactly once, with no
// skips and no repeats.
func TestCatalogPagingCoversEverythingOnce(t *testing.T) {
	s := mustStore(t)
	seen := map[string]int{}
	token := ""
	for pages := 0; ; pages++ {
		if pages > 10 {
			t.Fatal("pagination did not terminate")
		}
		page, err := s.Catalog(nil, token, 1)
		if err != nil {
			t.Fatalf("Catalog: %v", err)
		}
		for _, e := range page.Items {
			seen[e.ID]++
		}
		if page.NextToken == "" {
			break
		}
		token = page.NextToken
	}
	if len(seen) != 4 {
		t.Fatalf("saw %d distinct entries, want 4", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("entry %q returned %d times", id, n)
		}
	}
}

func TestWorksFilters(t *testing.T) {
	s := mustStore(t)
	season, movie, special := WorkSeason, WorkMovie, WorkSpecial
	for _, tc := range []struct {
		name   string
		filter WorkFilter
		want   int
	}{
		{"everything", WorkFilter{}, 10},
		{"by kind season", WorkFilter{Kind: &season}, 5},
		{"by kind movie", WorkFilter{Kind: &movie}, 1},
		{"by kind special", WorkFilter{Kind: &special}, 4},
		{"by year", WorkFilter{ReleaseYear: 2006}, 1},
		{"by year with no works", WorkFilter{ReleaseYear: 1999}, 0},
		{"by quarter", WorkFilter{ReleaseSeason: model.SeasonWinter}, 1},
		{"by series", WorkFilter{SeriesID: "zzz"}, 1},
		{"by series with none", WorkFilter{SeriesID: "minimal"}, 0},
		{"combined", WorkFilter{SeriesID: "aaa-main", Kind: &season}, 4},
		{"combined with no overlap", WorkFilter{SeriesID: "zzz", Kind: &movie}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.Works(tc.filter, "", 0)
			if err != nil {
				t.Fatalf("Works: %v", err)
			}
			if page.Total != tc.want {
				t.Fatalf("total = %d, want %d", page.Total, tc.want)
			}
		})
	}
}

// A quarter filter excludes movies and specials, which carry a year without a
// quarter — otherwise a seasonal chart would list films it should not.
func TestWorksQuarterExcludesUndatedKinds(t *testing.T) {
	s := mustStore(t)
	page, err := s.Works(WorkFilter{ReleaseSeason: model.SeasonWinter}, "", 0)
	if err != nil {
		t.Fatalf("Works: %v", err)
	}
	for _, w := range page.Items {
		if w.Kind != WorkSeason {
			t.Errorf("work %q of kind %v matched a quarter filter", w.ID, w.Kind)
		}
	}
}

func TestWorksCarryTheirSeries(t *testing.T) {
	s := mustStore(t)
	page, err := s.Works(WorkFilter{SeriesID: "aaa-main"}, "", 0)
	if err != nil {
		t.Fatalf("Works: %v", err)
	}
	for _, w := range page.Items {
		if w.SeriesID != "aaa-main" {
			t.Errorf("work %q has series %q", w.ID, w.SeriesID)
		}
		if w.SeriesTitles.IsZero() {
			t.Errorf("work %q carries no series title", w.ID)
		}
	}
}

func TestPaginateEdges(t *testing.T) {
	in := []int{1, 2, 3, 4, 5}

	t.Run("non-positive limit applies the default", func(t *testing.T) {
		page, err := paginate(in, "", 0)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != len(in) || page.NextToken != "" {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("last page carries no token", func(t *testing.T) {
		page, err := paginate(in, encodeCursor(3), 2)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != 2 || page.NextToken != "" {
			t.Fatalf("page = %+v", page)
		}
	})

	// An offset past the end yields an empty final page rather than an error or
	// a panic, so a client that over-pages degrades gracefully.
	t.Run("offset past the end", func(t *testing.T) {
		page, err := paginate(in, encodeCursor(99), 2)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != 0 || page.NextToken != "" || page.Total != 5 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		page, err := paginate([]int{}, "", 10)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != 0 || page.NextToken != "" || page.Total != 0 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("bad token propagates", func(t *testing.T) {
		if _, err := paginate(in, "!!!", 2); err == nil {
			t.Fatal("want an error for a malformed token")
		}
	})
}

func TestCursorRoundTrip(t *testing.T) {
	for _, offset := range []int{0, 1, 42, 1_000_000} {
		got, err := decodeCursor(encodeCursor(offset))
		if err != nil {
			t.Fatalf("decodeCursor(%d): %v", offset, err)
		}
		if got != offset {
			t.Errorf("round trip of %d = %d", offset, got)
		}
	}
}

// A malformed token is rejected rather than silently treated as the start of
// the result set, so a corrupted cursor surfaces instead of looking like an
// unexpected jump back to page one.
func TestDecodeCursorRejectsGarbage(t *testing.T) {
	for _, tc := range []struct {
		name, token string
	}{
		{"not base64", "!!!not-base64!!!"},
		{"missing prefix", encodeRaw("12")},
		{"not a number", encodeRaw("o:abc")},
		{"negative", encodeRaw("o:-1")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeCursor(tc.token); err == nil {
				t.Fatalf("decodeCursor(%q) = nil error", tc.token)
			}
		})
	}
	if got, err := decodeCursor(""); got != 0 || err != nil {
		t.Errorf("empty token = (%d, %v), want (0, nil)", got, err)
	}
}

func TestSearchPage(t *testing.T) {
	s := mustStore(t)

	t.Run("blank query matches nothing", func(t *testing.T) {
		page, err := s.SearchPage("  ", "", 0)
		if err != nil {
			t.Fatalf("SearchPage: %v", err)
		}
		if page.Total != 0 || len(page.Items) != 0 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("paging reaches results the limit would have truncated", func(t *testing.T) {
		first, err := s.SearchPage("a", "", 1)
		if err != nil {
			t.Fatalf("SearchPage: %v", err)
		}
		if first.Total < 2 {
			t.Skipf("fixture yields %d matches; nothing to page", first.Total)
		}
		if first.NextToken == "" {
			t.Fatal("want a next token when more matches remain")
		}
		second, err := s.SearchPage("a", first.NextToken, 1)
		if err != nil {
			t.Fatalf("SearchPage: %v", err)
		}
		if len(second.Items) == 0 || second.Items[0].ID == first.Items[0].ID {
			t.Errorf("second page repeated the first: %+v", second.Items)
		}
	})

	t.Run("bad token", func(t *testing.T) {
		if _, err := s.SearchPage("a", "!!!", 0); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestCharactersAndStaffPages(t *testing.T) {
	s := mustStore(t)

	all, err := s.CharactersPage("", "", "", 0)
	if err != nil {
		t.Fatalf("CharactersPage: %v", err)
	}
	if all.Total != 2 {
		t.Errorf("characters total = %d, want 2", all.Total)
	}

	// An unknown series yields an empty page; the service layer is what turns a
	// typo into NotFound.
	unknown, err := s.CharactersPage("nope", "", "", 0)
	if err != nil {
		t.Fatalf("CharactersPage: %v", err)
	}
	if unknown.Total != 0 {
		t.Errorf("unknown series total = %d, want 0", unknown.Total)
	}

	staff, err := s.StaffPage("", "", "", 0)
	if err != nil {
		t.Fatalf("StaffPage: %v", err)
	}
	if staff.Total != 2 {
		t.Errorf("staff total = %d, want 2", staff.Total)
	}

	// va-one is credited in both ja and en, va-two only in en — so the language
	// filter is only really exercised by ja, which must exclude va-two.
	for lang, want := range map[string]int{"en": 2, "ja": 1, "de": 0} {
		page, err := s.StaffPage(lang, "", "", 0)
		if err != nil {
			t.Fatalf("StaffPage(%q): %v", lang, err)
		}
		if page.Total != want {
			t.Errorf("%q staff total = %d, want %d", lang, page.Total, want)
		}
	}

	for _, bad := range []func() error{
		func() error { _, err := s.CharactersPage("", "", "!!!", 0); return err },
		func() error { _, err := s.StaffPage("", "", "!!!", 0); return err },
	} {
		if err := bad(); err == nil {
			t.Error("want an error for a malformed token")
		}
	}
}

// --- service level ----------------------------------------------------------

func TestListCatalogRPC(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.ListCatalog(context.Background(), connect.NewRequest(&animev1.ListCatalogRequest{Limit: 2}))
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if got := len(resp.Msg.GetEntries()); got != 2 {
		t.Fatalf("entries = %d, want 2", got)
	}
	if resp.Msg.GetTotalSize() != 4 || resp.Msg.GetNextPageToken() == "" {
		t.Fatalf("total = %d, next = %q", resp.Msg.GetTotalSize(), resp.Msg.GetNextPageToken())
	}
	// A catalog row must not carry the nested structure — that is the whole
	// reason it exists.
	for _, e := range resp.Msg.GetEntries() {
		if e.GetId() == "" || e.GetTitle() == "" {
			t.Errorf("entry missing id/title: %+v", e)
		}
	}

	t.Run("kind filter", func(t *testing.T) {
		resp, err := svc.ListCatalog(context.Background(), connect.NewRequest(&animev1.ListCatalogRequest{
			Kind: animev1.EntryKind_ENTRY_KIND_FRANCHISE,
		}))
		if err != nil {
			t.Fatalf("ListCatalog: %v", err)
		}
		if resp.Msg.GetTotalSize() != 1 {
			t.Fatalf("franchise total = %d, want 1", resp.Msg.GetTotalSize())
		}
	})

	t.Run("bad token is invalid argument", func(t *testing.T) {
		_, err := svc.ListCatalog(context.Background(), connect.NewRequest(&animev1.ListCatalogRequest{PageToken: "!!!"}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
		}
	})
}

func TestListWorksRPC(t *testing.T) {
	svc := newTestService(t)

	resp, err := svc.ListWorks(context.Background(), connect.NewRequest(&animev1.ListWorksRequest{}))
	if err != nil {
		t.Fatalf("ListWorks: %v", err)
	}
	if resp.Msg.GetTotalSize() != 10 {
		t.Fatalf("total = %d, want 10", resp.Msg.GetTotalSize())
	}

	t.Run("filters map onto the wire enums", func(t *testing.T) {
		resp, err := svc.ListWorks(context.Background(), connect.NewRequest(&animev1.ListWorksRequest{
			ReleaseYear:   2006,
			ReleaseSeason: animev1.ReleaseSeason_RELEASE_SEASON_WINTER,
			Kind:          animev1.WorkKind_WORK_KIND_SEASON,
		}))
		if err != nil {
			t.Fatalf("ListWorks: %v", err)
		}
		if resp.Msg.GetTotalSize() != 1 {
			t.Fatalf("total = %d, want 1", resp.Msg.GetTotalSize())
		}
		w := resp.Msg.GetWorks()[0]
		if w.GetKind() != animev1.WorkKind_WORK_KIND_SEASON || w.GetSeriesId() != "aaa-main" {
			t.Errorf("work = %+v", w)
		}
		if w.GetSeriesTitle() == "" {
			t.Error("work carries no series title")
		}
	})

	t.Run("unknown series is not found", func(t *testing.T) {
		_, err := svc.ListWorks(context.Background(), connect.NewRequest(&animev1.ListWorksRequest{SeriesId: "nope"}))
		if connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("code = %v, want not_found", connect.CodeOf(err))
		}
	})

	t.Run("bad token is invalid argument", func(t *testing.T) {
		_, err := svc.ListWorks(context.Background(), connect.NewRequest(&animev1.ListWorksRequest{PageToken: "!!!"}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("code = %v, want invalid_argument", connect.CodeOf(err))
		}
	})
}

// The three pre-existing list endpoints gained cursors; check they report a
// total and hand back a usable token.
func TestExistingListsPaginate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	search, err := svc.Search(ctx, connect.NewRequest(&animev1.SearchRequest{Query: "a", Limit: 1}))
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if search.Msg.GetTotalSize() == 0 {
		t.Error("search reported no total")
	}

	chars, err := svc.ListCharacters(ctx, connect.NewRequest(&animev1.ListCharactersRequest{Limit: 1}))
	if err != nil {
		t.Fatalf("ListCharacters: %v", err)
	}
	if chars.Msg.GetTotalSize() != 2 || chars.Msg.GetNextPageToken() == "" {
		t.Errorf("characters total = %d, next = %q", chars.Msg.GetTotalSize(), chars.Msg.GetNextPageToken())
	}
	next, err := svc.ListCharacters(ctx, connect.NewRequest(&animev1.ListCharactersRequest{
		Limit: 1, PageToken: chars.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("ListCharacters page 2: %v", err)
	}
	if len(next.Msg.GetCharacters()) != 1 ||
		next.Msg.GetCharacters()[0].GetId() == chars.Msg.GetCharacters()[0].GetId() {
		t.Error("second character page repeated the first")
	}

	staff, err := svc.ListStaff(ctx, connect.NewRequest(&animev1.ListStaffRequest{Limit: 1}))
	if err != nil {
		t.Fatalf("ListStaff: %v", err)
	}
	if staff.Msg.GetTotalSize() != 2 {
		t.Errorf("staff total = %d, want 2", staff.Msg.GetTotalSize())
	}

	for name, call := range map[string]func() error{
		"search": func() error {
			_, e := svc.Search(ctx, connect.NewRequest(&animev1.SearchRequest{Query: "a", PageToken: "!!!"}))
			return e
		},
		"characters": func() error {
			_, e := svc.ListCharacters(ctx, connect.NewRequest(&animev1.ListCharactersRequest{PageToken: "!!!"}))
			return e
		},
		"staff": func() error {
			_, e := svc.ListStaff(ctx, connect.NewRequest(&animev1.ListStaffRequest{PageToken: "!!!"}))
			return e
		},
	} {
		if code := connect.CodeOf(call()); code != connect.CodeInvalidArgument {
			t.Errorf("%s bad token: code = %v, want invalid_argument", name, code)
		}
	}
}

// --- enum mapping -----------------------------------------------------------

func TestWorkKindMapping(t *testing.T) {
	for _, k := range []WorkKind{WorkSeason, WorkMovie, WorkSpecial} {
		got := fromWorkKind(toWorkKind(k))
		if got == nil || *got != k {
			t.Errorf("round trip of %v = %v", k, got)
		}
	}
	if got := toWorkKind(WorkKind(99)); got != animev1.WorkKind_WORK_KIND_UNSPECIFIED {
		t.Errorf("unknown kind = %v", got)
	}
	// UNSPECIFIED is the filter's "any kind", so it must map to no filter.
	if got := fromWorkKind(animev1.WorkKind_WORK_KIND_UNSPECIFIED); got != nil {
		t.Errorf("unspecified = %v, want nil", got)
	}
}

func TestEntryKindAndSeasonMapping(t *testing.T) {
	for _, tc := range []struct {
		in   animev1.EntryKind
		want *EntryKind
	}{
		{animev1.EntryKind_ENTRY_KIND_FRANCHISE, ptr(EntryFranchise)},
		{animev1.EntryKind_ENTRY_KIND_SERIES, ptr(EntrySeries)},
		{animev1.EntryKind_ENTRY_KIND_UNSPECIFIED, nil},
	} {
		got := fromEntryKind(tc.in)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("%v = %v, want nil", tc.in, *got)
		case tc.want != nil && (got == nil || *got != *tc.want):
			t.Errorf("%v mapped wrong", tc.in)
		}
	}

	for in, want := range map[animev1.ReleaseSeason]model.ReleaseSeason{
		animev1.ReleaseSeason_RELEASE_SEASON_WINTER:      model.SeasonWinter,
		animev1.ReleaseSeason_RELEASE_SEASON_SPRING:      model.SeasonSpring,
		animev1.ReleaseSeason_RELEASE_SEASON_SUMMER:      model.SeasonSummer,
		animev1.ReleaseSeason_RELEASE_SEASON_FALL:        model.SeasonFall,
		animev1.ReleaseSeason_RELEASE_SEASON_UNSPECIFIED: "",
	} {
		if got := fromReleaseSeason(in); got != want {
			t.Errorf("fromReleaseSeason(%v) = %q, want %q", in, got, want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// encodeRaw base64s a literal token body, for building deliberately malformed
// cursors in tests.
func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// Searching by name is what a search page is for, so it gets the same
// treatment as title search: any language, case-insensitive, substring.
func TestCharactersPageQuery(t *testing.T) {
	s := mustStore(t)
	for _, tc := range []struct {
		name, seriesID, query string
		wantIDs               []string
	}{
		{"by english name", "", "alpha hero", []string{"alpha-hero"}},
		{"case-insensitive", "", "ALPHA", []string{"alpha-hero"}},
		{"substring", "", "hero", []string{"alpha-hero"}},
		{"trims whitespace", "", "  hero  ", []string{"alpha-hero"}},
		{"no match", "", "nobodyhere", nil},
		{"empty query matches everyone", "", "", []string{"alpha-hero", "zed-friend"}},
		// alpha-hero appears in both series, so a series filter plus a query
		// must intersect rather than fall back to either one alone.
		{"combined with a series", "zzz", "alpha", []string{"alpha-hero"}},
		{"a name absent from that series matches nothing", "zzz", "zed", nil},
		// zed-friend is nested under zzz but declares no appearances, so it is
		// in the global cast and in no series' cast — a query must not resurrect
		// it under a series.
		{"globally findable but not in a series", "", "zed", []string{"zed-friend"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.CharactersPage(tc.seriesID, tc.query, "", 0)
			if err != nil {
				t.Fatalf("CharactersPage: %v", err)
			}
			if len(page.Items) != len(tc.wantIDs) {
				ids := make([]string, len(page.Items))
				for i, c := range page.Items {
					ids[i] = c.ID
				}
				t.Fatalf("got %v, want %v", ids, tc.wantIDs)
			}
			for i, c := range page.Items {
				if c.ID != tc.wantIDs[i] {
					t.Errorf("result %d = %q, want %q", i, c.ID, tc.wantIDs[i])
				}
			}
		})
	}
}

func TestStaffPageQuery(t *testing.T) {
	s := mustStore(t)
	for _, tc := range []struct {
		name, language, query string
		want                  int
	}{
		{"by name", "", "voice one", 1},
		{"case-insensitive", "", "VOICE", 1},
		{"original script", "", "声優一", 1},
		{"no match", "", "nobodyhere", 0},
		{"empty query keeps everyone", "", "", 2},
		// A staff member with no name at all must not match a non-empty query.
		{"unnamed staff are not matched", "", "va-two", 0},
		{"combined with a language", "ja", "voice", 1},
		{"language and query that do not overlap", "ja", "nobody", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.StaffPage(tc.language, tc.query, "", 0)
			if err != nil {
				t.Fatalf("StaffPage: %v", err)
			}
			if len(page.Items) != tc.want {
				t.Fatalf("got %d results, want %d", len(page.Items), tc.want)
			}
		})
	}
}

func TestListCharactersRPCQuery(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.ListCharacters(context.Background(), connect.NewRequest(&animev1.ListCharactersRequest{
		Query: "hero",
	}))
	if err != nil {
		t.Fatalf("ListCharacters: %v", err)
	}
	if resp.Msg.GetTotalSize() != 1 || resp.Msg.GetCharacters()[0].GetId() != "alpha-hero" {
		t.Fatalf("total = %d, characters = %+v", resp.Msg.GetTotalSize(), resp.Msg.GetCharacters())
	}
}

func TestListStaffRPCQuery(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.ListStaff(context.Background(), connect.NewRequest(&animev1.ListStaffRequest{
		Query: "voice",
	}))
	if err != nil {
		t.Fatalf("ListStaff: %v", err)
	}
	if resp.Msg.GetTotalSize() != 1 {
		t.Fatalf("total = %d, want 1", resp.Msg.GetTotalSize())
	}
}

// The works query got none of the treatment the character and staff queries
// did, though it has a rule neither of them has: it matches the work's own
// title OR its series' title, because an untitled season has no title to match.
func TestWorksFilterQuery(t *testing.T) {
	s := mustStore(t)
	season := WorkSeason
	for _, tc := range []struct {
		name   string
		filter WorkFilter
		want   int
	}{
		// "Alpha Main" is the series title; its seasons carry no titles of
		// their own, so matching the series is the only way to find them.
		{"matches by series title", WorkFilter{Query: "alpha main"}, 9},
		{"case-insensitive", WorkFilter{Query: "ALPHA MAIN"}, 9},
		{"trims whitespace", WorkFilter{Query: "  alpha main  "}, 9},
		// "Alpha Movie" is a work's own title.
		{"matches by the work's own title", WorkFilter{Query: "alpha movie"}, 1},
		{"original script", WorkFilter{Query: "ゼッド"}, 1},
		{"no match", WorkFilter{Query: "nothinghere"}, 0},
		{"empty query matches everything", WorkFilter{Query: ""}, 10},
		// Combining must intersect, not let either side win.
		{"with a kind", WorkFilter{Query: "alpha main", Kind: &season}, 4},
		{"with a year", WorkFilter{Query: "alpha main", ReleaseYear: 2006}, 1},
		{"with a year that excludes it", WorkFilter{Query: "alpha movie", ReleaseYear: 2006}, 0},
		{"with a quarter", WorkFilter{Query: "alpha main", ReleaseSeason: model.SeasonWinter}, 1},
		{"with a series", WorkFilter{Query: "alpha", SeriesID: "zzz"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := s.Works(tc.filter, "", 0)
			if err != nil {
				t.Fatalf("Works: %v", err)
			}
			if page.Total != tc.want {
				ids := make([]string, len(page.Items))
				for i, w := range page.Items {
					ids[i] = w.ID
				}
				t.Fatalf("total = %d, want %d (%v)", page.Total, tc.want, ids)
			}
		})
	}
}

func TestListWorksRPCQuery(t *testing.T) {
	svc := newTestService(t)
	resp, err := svc.ListWorks(context.Background(), connect.NewRequest(&animev1.ListWorksRequest{
		Query: "alpha movie",
	}))
	if err != nil {
		t.Fatalf("ListWorks: %v", err)
	}
	if resp.Msg.GetTotalSize() != 1 || resp.Msg.GetWorks()[0].GetId() != "aaa-movie" {
		t.Fatalf("total = %d, works = %+v", resp.Msg.GetTotalSize(), resp.Msg.GetWorks())
	}
}
