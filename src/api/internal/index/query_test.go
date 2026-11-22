package index

import (
	"encoding/base64"
	"testing"
)

// page2 adapts the internal page() to the slice-shaped signature these tests
// were written against, which is also what Paginate exposes.
func page2[T any](in []T, token string, limit int) (Page[T], error) {
	return Paginate(in, token, limit)
}

func TestPaginateEdges(t *testing.T) {
	in := []int{1, 2, 3, 4, 5}

	t.Run("non-positive limit applies the default", func(t *testing.T) {
		page, err := page2(in, "", 0)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != len(in) || page.NextToken != "" {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("last page carries no token", func(t *testing.T) {
		page, err := page2(in, encodeCursor(3), 2)
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
		page, err := page2(in, encodeCursor(99), 2)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != 0 || page.NextToken != "" || page.Total != 5 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		page, err := page2([]int{}, "", 10)
		if err != nil {
			t.Fatalf("paginate: %v", err)
		}
		if len(page.Items) != 0 || page.NextToken != "" || page.Total != 0 {
			t.Fatalf("page = %+v", page)
		}
	})

	t.Run("bad token propagates", func(t *testing.T) {
		if _, err := page2(in, "!!!", 2); err == nil {
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

// encodeRaw base64s a literal token body, for building deliberately malformed
// cursors in tests.
func encodeRaw(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

// Work answers "which series does this installment belong to, and which file
// holds it" — the lookup a caller holding only an installment id needs before
// it can read anything. Every kind resolves, and an id that is not an
// installment's says so rather than returning a plausible-looking zero series.
func TestWorkResolvesAnInstallmentToItsSeriesAndFile(t *testing.T) {
	ix := mustIndex(t)

	for _, tc := range []struct {
		id, series string
		kind       WorkKind
	}{
		{"aaa-s1", "aaa-main", WorkSeason},
		{"aaa-s2", "aaa-main", WorkSeason},
		{"aaa-movie", "aaa-main", WorkMovie},
		{"aaa-ova", "aaa-side", WorkSpecial},
		{"zzz-s1", "zzz", WorkSeason},
	} {
		series, file, kind, ok := ix.Work(tc.id)
		if !ok {
			t.Errorf("Work(%q) found nothing", tc.id)
			continue
		}
		if series != tc.series {
			t.Errorf("Work(%q) series = %q, want %q", tc.id, series, tc.series)
		}
		if kind != tc.kind {
			t.Errorf("Work(%q) kind = %v, want %v", tc.id, kind, tc.kind)
		}
		if file == "" {
			t.Errorf("Work(%q) named no file, so the caller cannot read the record", tc.id)
		}
	}

	// A series id is not an installment id, and neither is a name nobody uses.
	for _, id := range []string{"aaa-main", "nothing-by-this-name", ""} {
		if _, _, _, ok := ix.Work(id); ok {
			t.Errorf("Work(%q) claimed to resolve an id that is not an installment's", id)
		}
	}
}

// SeriesOf pages the series under one franchise. An unknown franchise is an
// empty page rather than an error, since asking about a franchise that is not
// there is a question, not a fault.
func TestSeriesOfPagesAFranchiseAndIsEmptyForAnUnknownOne(t *testing.T) {
	ix := mustIndex(t)

	all, err := ix.SeriesOf("aaa", "", 50)
	if err != nil {
		t.Fatalf("SeriesOf: %v", err)
	}
	if len(all.Items) < 2 {
		t.Fatalf("SeriesOf(aaa) = %d series, want the franchise's two or more", len(all.Items))
	}
	for _, r := range all.Items {
		if r.ID == "" || r.File == "" {
			t.Errorf("a returned ref is missing its id or file: %+v", r)
		}
	}
	if all.NextToken != "" {
		t.Errorf("NextToken = %q; a page holding every series has no next page", all.NextToken)
	}

	// Paged one at a time, the pages together are the whole franchise, and the
	// paging stops because it ran out of series — not because a count matched.
	var seen []string
	token := ""
	for pages := 0; ; pages++ {
		if pages > len(all.Items)+1 {
			t.Fatalf("paging did not terminate after %d pages", pages)
		}
		p, err := ix.SeriesOf("aaa", token, 1)
		if err != nil {
			t.Fatalf("SeriesOf page %d: %v", pages, err)
		}
		for _, r := range p.Items {
			seen = append(seen, r.ID)
		}
		if p.NextToken == "" {
			break
		}
		token = p.NextToken
	}
	if len(seen) != len(all.Items) {
		t.Errorf("paging one at a time saw %d series, the single page saw %d", len(seen), len(all.Items))
	}

	none, err := ix.SeriesOf("no-such-franchise", "", 50)
	if err != nil {
		t.Errorf("SeriesOf(unknown) = error %v, want an empty page", err)
	}
	if len(none.Items) != 0 || none.NextToken != "" {
		t.Errorf("SeriesOf(unknown) = %+v, want an empty page", none)
	}
}
