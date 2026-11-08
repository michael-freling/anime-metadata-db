package builder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/michael-freling/anime-metadata-db/src/builder/internal/importer"
)

// TestDeclaredTitlesRecoversWhateverRenderWrote pins the two halves of the
// duplicate guard together.
//
// Render quotes a title when YAML needs it; declaredTitles has to recover the
// original string or the comparison in importer.Build silently stops matching,
// and duplicate series come back — 19 pairs the first time ten years were
// imported. Driven through Render rather than against hand-written fixtures so
// a change to the quoting rules cannot pass this by accident.
func TestDeclaredTitlesRecoversWhateverRenderWrote(t *testing.T) {
	titles := []string{
		"Plain Title",
		"青春ブタ野郎はサンタクロースの夢を見ない",
		"Aikatsu! Memorial Stage: Kagayaki no Unit Cup", // colon forces quotes
		"Gochuumon wa Usagi desu ka? Bloom",             // "?" forces quotes
		`has "quotes" inside`,
		`back\slash`,
		"663114", // numeric: quoted so it stays a string
		"null",
		"  padded  ",
		"#not a comment",
	}

	dir := t.TempDir()
	series := filepath.Join(dir, "series")
	if err := os.MkdirAll(series, 0o755); err != nil {
		t.Fatal(err)
	}
	for i, title := range titles {
		id := importer.Slug(title)
		if id == "" {
			id = "x"
		}
		body := importer.Render(importer.Series{
			ID: id, Original: title, Romanized: title, Seasons: []string{id + "-s1"},
		}, 2020)
		path := filepath.Join(series, id+"-"+string(rune('a'+i))+".yaml")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := declaredTitles(dir)
	if err != nil {
		t.Fatalf("declaredTitles: %v", err)
	}
	for _, title := range titles {
		if !got[title] {
			t.Errorf("declaredTitles did not recover %q; a second import would duplicate this series", title)
		}
	}
}

func TestDeclaredTitlesOnAMissingDirectory(t *testing.T) {
	// A repository with no overrides yet is not an error; it has no titles.
	got, err := declaredTitles(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("declaredTitles: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want none", got)
	}
}

func TestUnquoteYAML(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`Plain`, `Plain`},
		{`"quoted"`, `quoted`},
		{`"has \"inner\" quotes"`, `has "inner" quotes`},
		{`"back\\slash"`, `back\slash`},
		{`"663114"`, `663114`},
		// Not a quoted scalar: a lone quote is left alone rather than stripped
		// into something that was never written.
		{`"unbalanced`, `"unbalanced`},
		{``, ``},
	} {
		if got := unquoteYAML(tc.in); got != tc.want {
			t.Errorf("unquoteYAML(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTrim(t *testing.T) {
	if got := trim("short", 20); got != "short" {
		t.Errorf("trim kept %q, want it untouched", got)
	}
	got := trim("a title longer than the budget", 10)
	if len([]rune(got)) != 10 {
		t.Errorf("trim(_, 10) = %q, %d runes, want 10", got, len([]rune(got)))
	}
}
