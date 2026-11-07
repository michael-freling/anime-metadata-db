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
