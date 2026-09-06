package api

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	animedb "github.com/michael-freling/anime-metadata-db"
	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
	browsev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/browse/v1"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
)

// realService serves the committed dataset, so these assertions are made
// against the data that actually ships rather than a fixture sized to pass.
func realService(t *testing.T) *Service {
	t.Helper()
	return NewService(realStore(t), "test")
}

func realBrowse(t *testing.T) *BrowseService {
	t.Helper()
	return NewBrowseService(realStore(t), "test")
}

func realStore(t *testing.T) *Store {
	t.Helper()
	ix, err := index.Open(animedb.Index)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	return NewStore(ix, animedb.DataFS)
}

// topLevelCollections reports the length of every repeated field directly on a
// response message, by name. Only the top level: what a search response owes is
// that its own result list is bounded, not that the records inside it are
// stripped down.
func topLevelCollections(m proto.Message) map[string]int {
	out := map[string]int{}
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.IsList() {
			out[string(fd.Name())] = v.List().Len()
		}
		return true
	})
	return out
}

// The property the design rests on, restated for a two-shape API: a *search*
// may never return a whole collection, because the dataset is built to hold the
// full upstream catalogue and a list that is complete at 150 rows is ruinous at
// 150,000.
//
// A single-entity Get is deliberately not covered. GetSeries embeds a series
// whole, and that is bounded by what one series can be rather than by how large
// the catalogue grows — see TestGetSeriesEmbedsTheWholeSeries, which asserts the
// opposite of this and is the reason the rule is written per-shape.
func TestSearchResponsesNeverReturnTheWholeCollection(t *testing.T) {
	svc := realService(t)
	browse := realBrowse(t)
	ctx := context.Background()

	const limit = 3
	checks := []struct {
		name  string
		field string
		call  func() (proto.Message, int32, error)
	}{
		{"SearchSeries", "series", func() (proto.Message, int32, error) {
			r, err := svc.SearchSeries(ctx, connect.NewRequest(&animev1.SearchSeriesRequest{Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
		{"SearchReleases", "releases", func() (proto.Message, int32, error) {
			r, err := svc.SearchReleases(ctx, connect.NewRequest(&animev1.SearchReleasesRequest{Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
		{"Search", "results", func() (proto.Message, int32, error) {
			r, err := browse.Search(ctx, connect.NewRequest(&browsev1.SearchRequest{Query: "a", Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
		{"ListCatalog", "entries", func() (proto.Message, int32, error) {
			r, err := browse.ListCatalog(ctx, connect.NewRequest(&browsev1.ListCatalogRequest{Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
		{"ListCharacters", "characters", func() (proto.Message, int32, error) {
			r, err := browse.ListCharacters(ctx, connect.NewRequest(&browsev1.ListCharactersRequest{Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
		{"ListStaff", "staff", func() (proto.Message, int32, error) {
			r, err := browse.ListStaff(ctx, connect.NewRequest(&browsev1.ListStaffRequest{Limit: limit}))
			if err != nil {
				return nil, 0, err
			}
			return r.Msg, r.Msg.GetTotalSize(), nil
		}},
	}

	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			msg, total, err := tc.call()
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			got := topLevelCollections(msg)[tc.field]
			if got > limit {
				t.Errorf("%s returned %d %s for limit %d", tc.name, got, tc.field, limit)
			}
			// A limit that is never exceeded proves nothing if the collection
			// is smaller than the limit, so the fixture has to be big enough
			// for the cap to bite.
			if int(total) <= limit {
				t.Fatalf("%s has only %d rows, so the cap is untested", tc.name, total)
			}
		})
	}
}

// The counterpart, and the change this API turns on: a single-entity Get is
// complete. The series with 148 characters is the case that made it real — it
// used to embed 25 and report the rest as a number, so a client rendering the
// cast silently dropped 123 people unless it made a second call.
func TestGetSeriesEmbedsTheWholeSeries(t *testing.T) {
	svc := realService(t)
	browse := realBrowse(t)
	ctx := context.Background()

	const id = "tensei-shitara-slime-datta-ken"
	resp, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: id}))
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	series := resp.Msg.GetSeries()

	// The cast is fetched rather than read off the record, so it is the one
	// that could silently truncate. Cross-check it against the browse API's
	// paged view of the same cast, which counts independently.
	page, err := browse.ListCharacters(ctx, connect.NewRequest(&browsev1.ListCharactersRequest{
		SeriesId: id, Limit: 1,
	}))
	if err != nil {
		t.Fatalf("ListCharacters: %v", err)
	}
	if want := int(page.Msg.GetTotalSize()); len(series.GetCharacters()) != want {
		t.Errorf("GetSeries embedded %d characters, the series has %d", len(series.GetCharacters()), want)
	}
	if len(series.GetCharacters()) <= index.EmbeddedLimit {
		t.Fatalf("%s has %d characters, which is below the old cap of %d — pick a bigger series or this proves nothing",
			id, len(series.GetCharacters()), index.EmbeddedLimit)
	}

	// Episodes likewise: the longest season in the dataset is well past the old
	// cap, so a truncating GetSeries would show up here.
	var longest int
	for _, s := range series.GetSeasons() {
		if n := len(s.GetEpisodes()); n > longest {
			longest = n
		}
	}
	if longest == 0 {
		t.Fatal("no season carried episodes, so this asserts nothing")
	}

	deep, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: "black-clover"}))
	if err != nil {
		t.Fatalf("GetSeries(black-clover): %v", err)
	}
	var most int
	for _, s := range deep.Msg.GetSeries().GetSeasons() {
		if n := len(s.GetEpisodes()); n > most {
			most = n
		}
	}
	if most <= index.EmbeddedLimit {
		t.Errorf("longest black-clover season embedded %d episodes, at or below the old cap of %d",
			most, index.EmbeddedLimit)
	}
}

// walkPages drains a paginated RPC and returns how many items it yielded and
// what the endpoint claimed the total was.
func walkPages(t *testing.T, name string, fetch func(token string) (items []string, next string, total int32)) (int, int32) {
	t.Helper()
	seen := map[string]bool{}
	var total int32
	token := ""
	for pages := 0; ; pages++ {
		if pages > 500 {
			t.Fatalf("%s: paging did not terminate", name)
		}
		items, next, reported := fetch(token)
		total = reported
		for _, id := range items {
			if seen[id] {
				t.Errorf("%s: %q appeared on more than one page", name, id)
			}
			seen[id] = true
		}
		if next == "" {
			break
		}
		token = next
	}
	return len(seen), total
}

// Every search must page its whole collection: each item reachable, none twice,
// and the reported total matching what paging actually produced.
func TestSearchEndpointsPageTheirWholeCollection(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	t.Run("SearchSeries", func(t *testing.T) {
		got, total := walkPages(t, "SearchSeries", func(token string) ([]string, string, int32) {
			resp, err := svc.SearchSeries(ctx, connect.NewRequest(&animev1.SearchSeriesRequest{
				PageToken: token, Limit: 40,
			}))
			if err != nil {
				t.Fatalf("SearchSeries: %v", err)
			}
			ids := make([]string, 0, len(resp.Msg.GetSeries()))
			for _, s := range resp.Msg.GetSeries() {
				ids = append(ids, s.GetId())
			}
			return ids, resp.Msg.GetNextPageToken(), resp.Msg.GetTotalSize()
		})
		if got == 0 || int32(got) != total {
			t.Errorf("paged %d series, endpoint reported %d", got, total)
		}
	})

	t.Run("SearchReleases", func(t *testing.T) {
		got, total := walkPages(t, "SearchReleases", func(token string) ([]string, string, int32) {
			resp, err := svc.SearchReleases(ctx, connect.NewRequest(&animev1.SearchReleasesRequest{
				PageToken: token, Limit: 40,
			}))
			if err != nil {
				t.Fatalf("SearchReleases: %v", err)
			}
			ids := make([]string, 0, len(resp.Msg.GetReleases()))
			for _, w := range resp.Msg.GetReleases() {
				ids = append(ids, w.GetId())
			}
			return ids, resp.Msg.GetNextPageToken(), resp.Msg.GetTotalSize()
		})
		if got == 0 || int32(got) != total {
			t.Errorf("paged %d releases, endpoint reported %d", got, total)
		}
	})
}

// SearchSeries covers every series, not just the top-level ones. A series
// inside a franchise is not a catalog entry, so a search built on the catalog
// would silently omit it — which is the bug this endpoint exists to not have.
func TestSearchSeriesCoversSeriesInsideFranchises(t *testing.T) {
	svc := realService(t)
	resp, err := svc.SearchSeries(context.Background(), connect.NewRequest(&animev1.SearchSeriesRequest{
		Query: "fate", Limit: 100,
	}))
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	var withFranchise int
	for _, s := range resp.Msg.GetSeries() {
		if s.GetFranchiseId() != "" {
			withFranchise++
		}
	}
	if withFranchise == 0 {
		t.Errorf("no result carried a franchise_id; got %d results", len(resp.Msg.GetSeries()))
	}
}

// A SeriesSummary's aggregates come from the index rather than from walking the
// record, so they have to agree with what GetSeries actually returns.
func TestSeriesSummaryAggregatesMatchTheSeries(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	resp, err := svc.SearchSeries(ctx, connect.NewRequest(&animev1.SearchSeriesRequest{
		Query: "demon slayer", Limit: 10,
	}))
	if err != nil {
		t.Fatalf("SearchSeries: %v", err)
	}
	if len(resp.Msg.GetSeries()) == 0 {
		t.Fatal("no series matched, so this asserts nothing")
	}
	for _, summary := range resp.Msg.GetSeries() {
		full, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: summary.GetId()}))
		if err != nil {
			t.Fatalf("GetSeries(%s): %v", summary.GetId(), err)
		}
		s := full.Msg.GetSeries()
		works := len(s.GetSeasons()) + len(s.GetMovies()) + len(s.GetSpecials())
		if int(summary.GetWorks()) != works {
			t.Errorf("%s: summary says %d works, GetSeries has %d", summary.GetId(), summary.GetWorks(), works)
		}
		episodes := 0
		for _, se := range s.GetSeasons() {
			episodes += len(se.GetEpisodes())
		}
		for _, sp := range s.GetSpecials() {
			episodes += len(sp.GetEpisodes())
		}
		if int(summary.GetEpisodes()) != episodes {
			t.Errorf("%s: summary says %d episodes, GetSeries has %d", summary.GetId(), summary.GetEpisodes(), episodes)
		}
		if summary.GetFranchiseId() != full.Msg.GetFranchiseId() {
			t.Errorf("%s: summary franchise %q, GetSeries %q",
				summary.GetId(), summary.GetFranchiseId(), full.Msg.GetFranchiseId())
		}
	}
}

func TestPublicEndpointsRejectBadRequests(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		// want is the code expected, or wantOK for a request that must succeed:
		// connect.CodeOf(nil) is CodeUnknown, not a zero value, so "no error"
		// cannot be spelled as a code.
		want   connect.Code
		wantOK bool
	}{
		{"series without an id", func() error {
			_, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{}))
			return err
		}, connect.CodeInvalidArgument, false},
		{"unknown series", func() error {
			_, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: "nope"}))
			return err
		}, connect.CodeNotFound, false},
		{"series search with a corrupt token", func() error {
			_, err := svc.SearchSeries(ctx, connect.NewRequest(&animev1.SearchSeriesRequest{PageToken: "!!!"}))
			return err
		}, connect.CodeInvalidArgument, false},
		{"release search with a corrupt token", func() error {
			_, err := svc.SearchReleases(ctx, connect.NewRequest(&animev1.SearchReleasesRequest{PageToken: "!!!"}))
			return err
		}, connect.CodeInvalidArgument, false},
		// A quarter with no year would answer with every Winter the dataset
		// holds, under a heading the caller meant to name one year.
		{"quarter without a year", func() error {
			_, err := svc.SearchReleases(ctx, connect.NewRequest(&animev1.SearchReleasesRequest{
				ReleaseSeason: animev1.ReleaseSeason_WINTER,
			}))
			return err
		}, connect.CodeInvalidArgument, false},
		{"quarter with a year is fine", func() error {
			_, err := svc.SearchReleases(ctx, connect.NewRequest(&animev1.SearchReleasesRequest{
				ReleaseYear: 2019, ReleaseSeason: animev1.ReleaseSeason_SPRING,
			}))
			return err
		}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if tc.wantOK {
				if err != nil {
					t.Errorf("got %v, want success", err)
				}
				return
			}
			if got := connect.CodeOf(err); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
