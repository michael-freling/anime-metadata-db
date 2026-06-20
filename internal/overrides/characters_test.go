package overrides

import (
	"path/filepath"
	"testing"
)

const staffYAML = `staff:
  - id: natsuki-hanae
    externalIds: { wikidataId: Q2596113 }
  - id: ayako-kawasumi
    externalIds: { wikidataId: Q49566 }
`

// mergedSeriesYAML is a series file with co-located cast (R1 + characters).
const mergedSeriesYAML = `series:
  id: demon-slayer
  seasons:
    - id: ds-s1
      number: 1
      externalIds: { anilistId: 101922 }
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

func TestParseStaff(t *testing.T) {
	o, err := ParseStaff([]byte(staffYAML), "staff/voice-actors.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(o.Staff) != 2 || len(o.IDs()) != 2 {
		t.Fatalf("unexpected staff: %+v", o)
	}
}

func TestParseStaffInvalid(t *testing.T) {
	tests := map[string]string{
		"no staff":      "staff: []\n",
		"staff no id":   "staff:\n  - externalIds: { wikidataId: Q1 }\n",
		"unknown field": "staff:\n  - id: a\n    bogus: 1\n",
		"bad yaml":      "staff: [oops\n",
	}
	for name, yaml := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseStaff([]byte(yaml), name); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseMergedSeriesWithCharacters(t *testing.T) {
	o, err := Parse([]byte(mergedSeriesYAML), "series/demon-slayer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if o.ID() != "demon-slayer" || len(o.Characters) != 1 {
		t.Fatalf("unexpected: id=%q characters=%d", o.ID(), len(o.Characters))
	}
	ids := o.IDs()
	if len(ids) != 2 || ids[0] != "demon-slayer" || ids[1] != "tanjiro-kamado" {
		t.Errorf("IDs() = %v", ids)
	}
}

func TestParseMergedCharacterNoID(t *testing.T) {
	bad := "series:\n  id: x\ncharacters:\n  - externalIds: { wikidataId: Q1 }\n"
	if _, err := Parse([]byte(bad), "x.yaml"); err == nil {
		t.Error("expected error for character without id")
	}
}

func TestLoadDirRoutesByKind(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "series", "demon-slayer.yaml"), mergedSeriesYAML)
	mustWrite(t, filepath.Join(dir, "staff", "voice-actors.yaml"), staffYAML)

	bundle, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Series) != 1 || len(bundle.Staff) != 1 {
		t.Fatalf("expected 1 series + 1 staff, got %d + %d", len(bundle.Series), len(bundle.Staff))
	}
	if len(bundle.Series[0].Characters) != 1 {
		t.Errorf("series should carry its characters")
	}
}

func TestLoadDirKindErrors(t *testing.T) {
	t.Run("staff mixed with series", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "series: { id: a }\nstaff: []\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected error: staff must be separate")
		}
	})
	t.Run("characters without series", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "characters:\n  - id: a\n    appearances: [{seriesId: s}]\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected error: characters need a series file")
		}
	})
	t.Run("no recognized key", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "x.yaml"), "numbered: [a]\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected unrecognized-key error")
		}
	})
	t.Run("duplicate id across series and staff", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "a.yaml"), "series:\n  id: shared\n")
		mustWrite(t, filepath.Join(dir, "b.yaml"), "staff:\n  - id: shared\n")
		if _, err := LoadDir(dir); err == nil {
			t.Error("expected duplicate id error")
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
