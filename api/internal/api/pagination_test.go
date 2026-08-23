package api

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	animedb "github.com/michael-freling/anime-metadata-db"
	animev1 "github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
)

// realService serves the committed dataset, so these assertions are made
// against the data that actually ships rather than a fixture sized to pass.
func realService(t *testing.T) *Service {
	t.Helper()
	ix, err := index.Open(animedb.Index)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	return NewService(NewStore(ix, animedb.DataFS), "test")
}

// collectionsWithin walks a message and reports any repeated field longer than
// limit, by path. Reflection rather than a hand-written list of fields: a new
// repeated field added later is covered the day it appears, which a list would
// not be.
func collectionsWithin(m proto.Message, limit int) []string {
	var over []string
	var walk func(msg protoreflect.Message, path string)
	walk = func(msg protoreflect.Message, path string) {
		msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			name := path + "." + string(fd.Name())
			switch {
			case fd.IsList():
				list := v.List()
				if list.Len() > limit {
					over = append(over, fmt.Sprintf("%s has %d", name, list.Len()))
				}
				if fd.Kind() == protoreflect.MessageKind {
					for i := 0; i < list.Len(); i++ {
						walk(list.Get(i).Message(), fmt.Sprintf("%s[%d]", name, i))
					}
				}
			case fd.Kind() == protoreflect.MessageKind && !fd.IsMap():
				walk(v.Message(), name)
			}
			return true
		})
	}
	walk(m.ProtoReflect(), "")
	return over
}

// The property this whole design rests on: no response may carry a collection
// that grows with the dataset. If one does, a big enough catalogue turns a
// single request into a bulk export — which is what the storage rework exists
// to prevent, and which no amount of index tuning would fix.
func TestNoResponseEmbedsAnUnboundedCollection(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	// Every id in the dataset, not a sample: the one series with an oversized
	// cast is exactly the case a sample would miss.
	catalog, err := svc.ListCatalog(ctx, connect.NewRequest(&animev1.ListCatalogRequest{Limit: 10000}))
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(catalog.Msg.GetEntries()) == 0 {
		t.Fatal("the catalog is empty, so this test asserts nothing")
	}

	check := func(what string, m proto.Message) {
		t.Helper()
		for _, over := range collectionsWithin(m, index.EmbeddedLimit) {
			t.Errorf("%s: %s, above the %d cap", what, over, index.EmbeddedLimit)
		}
	}

	for _, e := range catalog.Msg.GetEntries() {
		if e.GetKind() == animev1.EntryKind_FRANCHISE {
			resp, err := svc.GetFranchise(ctx, connect.NewRequest(&animev1.GetFranchiseRequest{Id: e.GetId()}))
			if err != nil {
				t.Fatalf("GetFranchise(%s): %v", e.GetId(), err)
			}
			check("GetFranchise("+e.GetId()+")", resp.Msg)
			continue
		}
		resp, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: e.GetId()}))
		if err != nil {
			t.Fatalf("GetSeries(%s): %v", e.GetId(), err)
		}
		check("GetSeries("+e.GetId()+")", resp.Msg)
	}

	chars, err := svc.ListCharacters(ctx, connect.NewRequest(&animev1.ListCharactersRequest{Limit: 10000}))
	if err != nil {
		t.Fatalf("ListCharacters: %v", err)
	}
	for _, c := range chars.Msg.GetCharacters() {
		resp, err := svc.GetCharacter(ctx, connect.NewRequest(&animev1.GetCharacterRequest{Id: c.GetId()}))
		if err != nil {
			t.Fatalf("GetCharacter(%s): %v", c.GetId(), err)
		}
		check("GetCharacter("+c.GetId()+")", resp.Msg)
	}

	staff, err := svc.ListStaff(ctx, connect.NewRequest(&animev1.ListStaffRequest{Limit: 10000}))
	if err != nil {
		t.Fatalf("ListStaff: %v", err)
	}
	for _, st := range staff.Msg.GetStaff() {
		resp, err := svc.GetStaff(ctx, connect.NewRequest(&animev1.GetStaffRequest{Id: st.GetId()}))
		if err != nil {
			t.Fatalf("GetStaff(%s): %v", st.GetId(), err)
		}
		check("GetStaff("+st.GetId()+")", resp.Msg)
	}
}

// A cap is only honest if the response says how much was left out. The series
// with 148 characters is the case that made this real: it embedded 100 and said
// nothing, so a client could not tell a full cast from a truncated one.
func TestCappedCollectionsReportTheirTrueSize(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	resp, err := svc.GetSeries(ctx, connect.NewRequest(&animev1.GetSeriesRequest{Id: "tensei-shitara-slime-datta-ken"}))
	if err != nil {
		t.Fatalf("GetSeries: %v", err)
	}
	series := resp.Msg.GetSeries()
	if got := len(series.GetCharacters()); got != index.EmbeddedLimit {
		t.Errorf("embedded cast = %d, want the cap of %d", got, index.EmbeddedLimit)
	}
	if series.GetCharactersTotal() <= int32(index.EmbeddedLimit) {
		t.Errorf("characters_total = %d, which does not report the truncation", series.GetCharactersTotal())
	}

	// The total has to match what paging the same collection actually yields,
	// or it is a different kind of lie.
	page, err := svc.ListCharacters(ctx, connect.NewRequest(&animev1.ListCharactersRequest{
		SeriesId: "tensei-shitara-slime-datta-ken", Limit: 1,
	}))
	if err != nil {
		t.Fatalf("ListCharacters: %v", err)
	}
	if page.Msg.GetTotalSize() != series.GetCharactersTotal() {
		t.Errorf("characters_total = %d but ListCharacters reports %d",
			series.GetCharactersTotal(), page.Msg.GetTotalSize())
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

// Each new endpoint must page its whole collection: every item reachable, none
// twice, and the reported total matching what paging actually produced.
func TestNewListEndpointsPageTheirWholeCollection(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	t.Run("ListCredits", func(t *testing.T) {
		got, total := walkPages(t, "ListCredits", func(token string) ([]string, string, int32) {
			resp, err := svc.ListCredits(ctx, connect.NewRequest(&animev1.ListCreditsRequest{
				StaffId: "ayako-kawasumi", PageToken: token, Limit: 1,
			}))
			if err != nil {
				t.Fatalf("ListCredits: %v", err)
			}
			ids := make([]string, 0, len(resp.Msg.GetCredits()))
			for _, c := range resp.Msg.GetCredits() {
				ids = append(ids, c.GetCharacterId()+"/"+c.GetLanguage())
			}
			return ids, resp.Msg.GetNextPageToken(), resp.Msg.GetTotalSize()
		})
		if got == 0 || int32(got) != total {
			t.Errorf("paged %d credits, endpoint reported %d", got, total)
		}
	})

	t.Run("ListAppearances", func(t *testing.T) {
		got, total := walkPages(t, "ListAppearances", func(token string) ([]string, string, int32) {
			resp, err := svc.ListAppearances(ctx, connect.NewRequest(&animev1.ListAppearancesRequest{
				CharacterId: "artoria-pendragon", PageToken: token, Limit: 1,
			}))
			if err != nil {
				t.Fatalf("ListAppearances: %v", err)
			}
			// An appearance is identified by its series *and* its scope: a
			// character whose cast changed part-way through a series holds one
			// appearance per group of installments, so the series id alone is
			// not unique. Any consumer keying on it — this test did — sees two
			// rows collapse into one.
			ids := make([]string, 0, len(resp.Msg.GetAppearances()))
			for _, a := range resp.Msg.GetAppearances() {
				id := a.GetSeriesId()
				for _, sc := range a.GetScope() {
					id += "/" + sc.GetSeasonId() + sc.GetMovieId() + sc.GetSpecialId()
				}
				ids = append(ids, id)
			}
			return ids, resp.Msg.GetNextPageToken(), resp.Msg.GetTotalSize()
		})
		if got == 0 || int32(got) != total {
			t.Errorf("paged %d appearances, endpoint reported %d", got, total)
		}
	})

	t.Run("ListSeries", func(t *testing.T) {
		got, total := walkPages(t, "ListSeries", func(token string) ([]string, string, int32) {
			resp, err := svc.ListSeries(ctx, connect.NewRequest(&animev1.ListSeriesRequest{
				FranchiseId: "fate", PageToken: token, Limit: 1,
			}))
			if err != nil {
				t.Fatalf("ListSeries: %v", err)
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

	t.Run("ListEpisodes", func(t *testing.T) {
		got, total := walkPages(t, "ListEpisodes", func(token string) ([]string, string, int32) {
			resp, err := svc.ListEpisodes(ctx, connect.NewRequest(&animev1.ListEpisodesRequest{
				SeasonId: "demon-slayer-s1", PageToken: token, Limit: 5,
			}))
			if err != nil {
				t.Fatalf("ListEpisodes: %v", err)
			}
			ids := make([]string, 0, len(resp.Msg.GetEpisodes()))
			for _, e := range resp.Msg.GetEpisodes() {
				ids = append(ids, fmt.Sprint(e.GetAiredNumber()))
			}
			return ids, resp.Msg.GetNextPageToken(), resp.Msg.GetTotalSize()
		})
		if got == 0 || int32(got) != total {
			t.Errorf("paged %d episodes, endpoint reported %d", got, total)
		}
	})
}

func TestNewListEndpointsRejectBadRequests(t *testing.T) {
	svc := realService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
		want connect.Code
	}{
		{"episodes without a parent", func() error {
			_, err := svc.ListEpisodes(ctx, connect.NewRequest(&animev1.ListEpisodesRequest{}))
			return err
		}, connect.CodeInvalidArgument},
		{"episodes with two parents", func() error {
			_, err := svc.ListEpisodes(ctx, connect.NewRequest(&animev1.ListEpisodesRequest{
				SeasonId: "demon-slayer-s1", SpecialId: "demon-slayer-s1",
			}))
			return err
		}, connect.CodeInvalidArgument},
		{"episodes of an unknown parent", func() error {
			_, err := svc.ListEpisodes(ctx, connect.NewRequest(&animev1.ListEpisodesRequest{SeasonId: "nope"}))
			return err
		}, connect.CodeNotFound},
		{"episodes with a corrupt token", func() error {
			_, err := svc.ListEpisodes(ctx, connect.NewRequest(&animev1.ListEpisodesRequest{
				SeasonId: "demon-slayer-s1", PageToken: "!!!",
			}))
			return err
		}, connect.CodeInvalidArgument},
		{"series without a franchise", func() error {
			_, err := svc.ListSeries(ctx, connect.NewRequest(&animev1.ListSeriesRequest{}))
			return err
		}, connect.CodeInvalidArgument},
		{"series of an unknown franchise", func() error {
			_, err := svc.ListSeries(ctx, connect.NewRequest(&animev1.ListSeriesRequest{FranchiseId: "nope"}))
			return err
		}, connect.CodeNotFound},
		{"appearances without a character", func() error {
			_, err := svc.ListAppearances(ctx, connect.NewRequest(&animev1.ListAppearancesRequest{}))
			return err
		}, connect.CodeInvalidArgument},
		{"appearances of an unknown character", func() error {
			_, err := svc.ListAppearances(ctx, connect.NewRequest(&animev1.ListAppearancesRequest{CharacterId: "nope"}))
			return err
		}, connect.CodeNotFound},
		{"credits without a staff id", func() error {
			_, err := svc.ListCredits(ctx, connect.NewRequest(&animev1.ListCreditsRequest{}))
			return err
		}, connect.CodeInvalidArgument},
		{"credits of an unknown staff member", func() error {
			_, err := svc.ListCredits(ctx, connect.NewRequest(&animev1.ListCreditsRequest{StaffId: "nope"}))
			return err
		}, connect.CodeNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := connect.CodeOf(tc.call()); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A season id passed as special_id names something real but of the wrong kind.
// It is not an error — it is a parent with no episodes of that kind — and must
// not return the season's episodes by accident.
func TestListEpisodesDoesNotConfuseParentKinds(t *testing.T) {
	svc := realService(t)
	resp, err := svc.ListEpisodes(context.Background(), connect.NewRequest(&animev1.ListEpisodesRequest{
		SpecialId: "demon-slayer-s1",
	}))
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(resp.Msg.GetEpisodes()) != 0 || resp.Msg.GetTotalSize() != 0 {
		t.Errorf("a season asked for as a special returned %d episodes", len(resp.Msg.GetEpisodes()))
	}
}
