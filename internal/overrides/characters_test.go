package overrides

import (
	"path/filepath"
	"testing"
)

const charactersYAML = `staff:
  - id: natsuki-hanae
    externalIds: { wikidataId: Q2596113 }
characters:
  - id: tanjiro-kamado
    externalIds: { wikidataId: Q85805158 }
    voiceActors:
      - { staffId: natsuki-hanae, language: ja }
    appearances:
      - seriesId: demon-slayer
        scope:
          - { seasonId: ds-s1 }
`

func TestParseCharacters(t *testing.T) {
	o, err := ParseCharacters([]byte(charactersYAML), "characters/demon-slayer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Characters) != 1 || len(o.Staff) != 1 {
		t.Fatalf("unexpected counts: %+v", o)
	}
	ids := o.IDs()
	if len(ids) != 2 {
		t.Errorf("IDs() = %v", ids)
	}
}

func TestParseCharactersInvalid(t *testing.T) {
	tests := map[string]string{
		"neither":         "{}\n",
		"character no id": "characters:\n  - appearances: [{seriesId: x}]\n",
		"staff no id":     "staff:\n  - externalIds: { wikidataId: Q1 }\n",
		"unknown field":   "characters:\n  - id: a\n    bogus: 1\n",
		"bad yaml":        "characters: [oops\n",
	}
	for name, yaml := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCharacters([]byte(yaml), name); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestLoadDirRoutesByKind(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "series", "demon-slayer.yaml"), seriesYAML)
	mustWrite(t, filepath.Join(dir, "characters", "demon-slayer.yaml"), charactersYAML)

	bundle, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Series) != 1 {
		t.Errorf("expected 1 series, got %d", len(bundle.Series))
	}
	if len(bundle.Characters) != 1 {
		t.Errorf("expected 1 characters file, got %d", len(bundle.Characters))
	}
	if bundle.Characters[0].Path != "characters/demon-slayer.yaml" {
		t.Errorf("unexpected path: %q", bundle.Characters[0].Path)
	}
}

func TestLoadDirKindErrors(t *testing.T) {
	t.Run("mixed keys", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "series: { id: a }\ncharacters: []\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected mixed-kind error")
		}
	})
	t.Run("no recognized key", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "numbered: [a]\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected unrecognized-key error")
		}
	})
	t.Run("duplicate id across R1 and R2", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "a.yaml"), "series:\n  id: shared\n")
		mustWrite(t, filepath.Join(dir, "b.yaml"), "characters:\n  - id: shared\n    appearances: [{seriesId: shared}]\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected duplicate id error across R1/R2")
		}
	})
	t.Run("not a mapping", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "- just\n- a\n- list\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected error for non-mapping document")
		}
	})
}
