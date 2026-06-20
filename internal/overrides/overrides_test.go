package overrides

import (
	"os"
	"path/filepath"
	"testing"
)

const franchiseYAML = `franchise:
  id: fate
  series:
    - id: fate-zero
      seasons:
        - id: fz-s1
          number: 1
          externalIds: { anilistId: 10087 }
numbered: [fate-zero]
`

const seriesYAML = `series:
  id: demon-slayer
  seasons:
    - id: ds-s1
      number: 1
      externalIds: { anilistId: 101922 }
`

func TestParseFranchise(t *testing.T) {
	o, err := Parse([]byte(franchiseYAML), "franchises/fate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if o.ID() != "fate" {
		t.Errorf("ID = %q", o.ID())
	}
	if !o.IsNumbered("fate-zero") || o.IsNumbered("nope") {
		t.Error("IsNumbered wrong")
	}
}

func TestParseSeries(t *testing.T) {
	o, err := Parse([]byte(seriesYAML), "series/demon-slayer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if o.ID() != "demon-slayer" {
		t.Errorf("ID = %q", o.ID())
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"both", "franchise: { id: a, series: [{id: s}] }\nseries: { id: b }\n"},
		{"neither", "numbered: [x]\n"},
		{"franchise no id", "franchise: { series: [{id: s}] }\n"},
		{"franchise no series", "franchise: { id: a }\n"},
		{"series no id", "series: { seasons: [] }\n"},
		{"unknown field", "series:\n  id: a\n  bogus: 1\n"},
		{"bad yaml", "series: [oops\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.yaml), tt.name); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestIDEmpty(t *testing.T) {
	if (Override{}).ID() != "" {
		t.Error("empty override should have empty id")
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fate.yaml")
	if err := os.WriteFile(path, []byte(franchiseYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	o, err := Load(path, "fate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if o.ID() != "fate" {
		t.Errorf("ID = %q", o.ID())
	}
	if _, err := Load(filepath.Join(dir, "missing.yaml"), "missing.yaml"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "franchises", "fate.yaml"), franchiseYAML)
	mustWrite(t, filepath.Join(dir, "series", "demon-slayer.yaml"), seriesYAML)
	mustWrite(t, filepath.Join(dir, "README.md"), "not yaml")

	ovs, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ovs) != 2 {
		t.Fatalf("expected 2 overrides, got %d", len(ovs))
	}
	// Sorted by relative path: franchises/ before series/.
	if ovs[0].Path != "franchises/fate.yaml" || ovs[1].Path != "series/demon-slayer.yaml" {
		t.Errorf("unexpected order/paths: %q, %q", ovs[0].Path, ovs[1].Path)
	}
}

func TestLoadDirMissing(t *testing.T) {
	ovs, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("missing dir should be no error: %v", err)
	}
	if ovs != nil {
		t.Errorf("expected nil overrides, got %v", ovs)
	}
}

func TestLoadDirDuplicateID(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.yaml"), seriesYAML)
	mustWrite(t, filepath.Join(dir, "b.yaml"), seriesYAML)
	if _, err := LoadDir(dir); err == nil {
		t.Error("expected duplicate id error")
	}
}

func TestLoadDirBadFile(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "bad.yaml"), "series: [oops\n")
	if _, err := LoadDir(dir); err == nil {
		t.Error("expected parse error to propagate")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
