package builder

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataset writes a data tree from path → YAML body and returns its directory.
func dataset(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// diffOf runs the comparison the way the command does, with the head tree as a
// repo's configured dataDir.
func diffOf(t *testing.T, base string, head map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	for name, body := range head {
		full := filepath.Join(repo, "data", name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := (&App{Dir: repo, Out: &out}).Diff(base); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

const frieren = `series:
  id: sousou-no-frieren
  titles:
    original: 葬送のフリーレン
    translations:
      ja-Latn: Sousou no Frieren
`

// The claim this exists to support: two builds of the same thing are the same
// thing. Saying so explicitly matters more than it looks — "no output" would
// read identically whether the comparison ran or not.
func TestDiffIdentical(t *testing.T) {
	files := map[string]string{"series/a.yaml": frieren}
	got := diffOf(t, dataset(t, files), files)
	if !strings.Contains(got, "dataset identical: 1 records rebuilt and compared, none changed") {
		t.Errorf("got:\n%s", got)
	}
}

// A field gaining a value is the common case — a derivation filling something
// that used to be authored, or a title appearing where there was none.
func TestDiffReportsAddedAndChangedFields(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": frieren})
	got := diffOf(t, base, map[string]string{"series/a.yaml": `series:
  id: sousou-no-frieren
  titles:
    original: 葬送のフリーレン
    translations:
      ja-Latn: Sousou no Frieren
      en: "Frieren: Beyond Journey's End"
`})
	if !strings.Contains(got, "1 changed, 0 added, 0 removed") {
		t.Errorf("counts wrong:\n%s", got)
	}
	if !strings.Contains(got, "+ series.titles.translations.en: Frieren: Beyond Journey's End") {
		t.Errorf("added field not named:\n%s", got)
	}
}

// A whole block appearing must read as the values it introduced. Reporting it
// as "{1 fields}" names the shape and hides the fact, which is the one thing a
// reviewer is here for.
func TestDiffFlattensAnAddedSubtree(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": frieren})
	got := diffOf(t, base, map[string]string{"series/a.yaml": frieren + `  externalIds:
    wikidataId: Q98642652
    anidbId: 17617
`})
	for _, want := range []string{
		"+ series.externalIds.anidbId: 17617",
		"+ series.externalIds.wikidataId: Q98642652",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "fields}") {
		t.Errorf("a subtree was summarised instead of flattened:\n%s", got)
	}
}

// A changed value shows both sides, since "it changed" without the old value
// cannot be reviewed.
func TestDiffShowsBothSidesOfAChange(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": "series:\n  id: x\n  titles:\n    original: old\n"})
	got := diffOf(t, base, map[string]string{"series/a.yaml": "series:\n  id: x\n  titles:\n    original: new\n"})
	if !strings.Contains(got, "~ series.titles.original: old → new") {
		t.Errorf("got:\n%s", got)
	}
}

func TestDiffReportsAddedAndRemovedRecords(t *testing.T) {
	base := dataset(t, map[string]string{"series/gone.yaml": frieren})
	got := diffOf(t, base, map[string]string{"series/new.yaml": frieren})
	if !strings.Contains(got, "0 changed, 1 added, 1 removed") {
		t.Errorf("counts wrong:\n%s", got)
	}
	if !strings.Contains(got, "+ series/new.yaml (new record)") || !strings.Contains(got, "- series/gone.yaml (record gone)") {
		t.Errorf("got:\n%s", got)
	}
}

// A list that grew reports the new element's leaves, not its shape.
func TestDiffReportsAGrownList(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": "series:\n  id: x\n  seasons:\n    - id: x-s1\n"})
	got := diffOf(t, base, map[string]string{"series/a.yaml": "series:\n  id: x\n  seasons:\n    - id: x-s1\n    - id: x-s2\n"})
	if !strings.Contains(got, "+ series.seasons[1].id: x-s2") {
		t.Errorf("got:\n%s", got)
	}
}

// The totals stay exact when the detail is elided, and the elision says how
// much it dropped — a truncated diff that reads as a complete one is worse than
// no diff at all.
func TestDiffTruncatesDetailButNotTotals(t *testing.T) {
	base := map[string]string{}
	head := map[string]string{}
	const many = maxRecordsShown + 5
	for i := range many {
		name := filepath.Join("series", string(rune('a'+i%26))+string(rune('0'+i/26))+".yaml")
		base[name] = "series:\n  id: x\n  titles:\n    original: old\n"
		head[name] = "series:\n  id: x\n  titles:\n    original: new\n"
	}
	got := diffOf(t, dataset(t, base), head)

	if !strings.Contains(got, "25 changed") {
		t.Errorf("the total must count every record:\n%s", got)
	}
	if !strings.Contains(got, "… and 5 more changed record(s)") {
		t.Errorf("the elision must say what it dropped:\n%s", got)
	}
	if n := strings.Count(got, "~ series.titles.original"); n != maxRecordsShown {
		t.Errorf("showed %d records in detail, want %d", n, maxRecordsShown)
	}
}

// Per-record detail is capped too, so one record rewritten end to end cannot
// push everything else out of the comment.
func TestDiffTruncatesFieldsWithinARecord(t *testing.T) {
	var oldFields, newFields strings.Builder
	oldFields.WriteString("series:\n  id: x\n  titles:\n")
	newFields.WriteString("series:\n  id: x\n  titles:\n")
	for i := range maxFieldsShown + 3 {
		key := string(rune('a' + i))
		fmtLine := "    " + key + ": %s\n"
		oldFields.WriteString(strings.Replace(fmtLine, "%s", "old", 1))
		newFields.WriteString(strings.Replace(fmtLine, "%s", "new", 1))
	}
	base := dataset(t, map[string]string{"series/a.yaml": oldFields.String()})
	got := diffOf(t, base, map[string]string{"series/a.yaml": newFields.String()})

	if !strings.Contains(got, "… and 3 more field(s)") {
		t.Errorf("got:\n%s", got)
	}
}

// A base directory that is not there is a mistake worth an error, not an empty
// diff that would read as "nothing changed".
func TestDiffMissingBaseIsAnError(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := (&App{Dir: repo, Out: &out}).Diff(filepath.Join(repo, "nope"))
	if err == nil {
		t.Fatal("expected an error for a missing base directory")
	}
	if !strings.Contains(err.Error(), "base dataset") {
		t.Errorf("the error should say which side is missing: %v", err)
	}
}

// Malformed YAML must fail rather than be silently treated as an empty record,
// which would report every one of its fields as removed.
func TestDiffMalformedRecordIsAnError(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": "series:\n  id: x\n"})
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "data", "series"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "data", "series", "a.yaml"), []byte("series: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := (&App{Dir: repo, Out: &out}).Diff(base); err == nil {
		t.Fatal("expected a parse error")
	}
}

// A block disappearing is the derivation case in reverse — an authored value
// replaced by a computed one leaves the override, and the reviewer needs to see
// which values went, not that "a subtree" did.
func TestDiffFlattensARemovedSubtree(t *testing.T) {
	base := dataset(t, map[string]string{"series/a.yaml": frieren + `  externalIds:
    anilistId: 154587
  seasons:
    - id: x-s1
      externalIds:
        anilistId: 182255
`})
	got := diffOf(t, base, map[string]string{"series/a.yaml": frieren})
	for _, want := range []string{
		"- series.externalIds.anilistId: 154587",
		"- series.seasons[0].externalIds.anilistId: 182255",
		"- series.seasons[0].id: x-s1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "items]") || strings.Contains(got, "fields}") {
		t.Errorf("a removed subtree was summarised instead of flattened:\n%s", got)
	}
}

// A field that changes shape rather than value — a scalar becoming a block, or
// a block becoming a list. Naming the shape is right here: the two sides have
// nothing comparable inside them, and printing every leaf of both would bury
// the fact that the shape itself moved.
func TestDiffReportsAShapeChange(t *testing.T) {
	base := dataset(t, map[string]string{
		"series/a.yaml": "series:\n  id: x\n  titles: plain\n",
		"series/b.yaml": "series:\n  id: y\n  seasons:\n    a: 1\n    b: 2\n",
	})
	got := diffOf(t, base, map[string]string{
		"series/a.yaml": "series:\n  id: x\n  titles:\n    original: now-a-map\n",
		"series/b.yaml": "series:\n  id: y\n  seasons:\n    - one\n    - two\n",
	})
	if !strings.Contains(got, "~ series.titles: plain → {1 fields}") {
		t.Errorf("scalar → map not reported as a shape change:\n%s", got)
	}
	if !strings.Contains(got, "~ series.seasons: {2 fields} → [2 items]") {
		t.Errorf("map → list not reported as a shape change:\n%s", got)
	}
}
