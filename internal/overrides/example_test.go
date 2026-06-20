package overrides

import (
	"os"
	"testing"
)

// TestExampleOverridesParse guards the committed example overrides in the repo
// root: they must always parse and pass schema validation.
func TestExampleOverridesParse(t *testing.T) {
	const dir = "../../config/overrides"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no example overrides directory")
	}
	bundle, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("example overrides failed to load: %v", err)
	}
	if len(bundle.Series) == 0 {
		t.Fatal("expected at least one example series override")
	}
	for _, o := range bundle.Series {
		if o.ID() == "" {
			t.Errorf("override %q has no id", o.Path)
		}
	}
	for _, o := range bundle.Characters {
		if len(o.IDs()) == 0 {
			t.Errorf("characters override %q has no ids", o.Path)
		}
	}
}
