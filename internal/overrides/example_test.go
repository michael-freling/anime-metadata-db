package overrides

import (
	"os"
	"testing"
)

// TestExampleOverridesParse guards the committed example overrides in the repo
// root: they must always parse and pass schema validation.
func TestExampleOverridesParse(t *testing.T) {
	const dir = "../../overrides"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("no example overrides directory")
	}
	ovs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("example overrides failed to load: %v", err)
	}
	if len(ovs) == 0 {
		t.Fatal("expected at least one example override")
	}
	for _, o := range ovs {
		if o.ID() == "" {
			t.Errorf("override %q has no id", o.Path)
		}
	}
}
