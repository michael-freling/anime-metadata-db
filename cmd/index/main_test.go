package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dataset writes a minimal but complete dataset into a temporary root.
func dataset(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "data", "series"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "series", "a.yaml"),
		[]byte("series:\n  id: a\n  titles:\n    translations:\n      en: A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestWritesAndThenVerifies(t *testing.T) {
	root := dataset(t)
	var out bytes.Buffer
	if err := run([]string{"-root", root}, &out); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out.String(), "wrote data/index.tsv") {
		t.Errorf("output = %q", out.String())
	}

	// What was just written is by definition up to date.
	out.Reset()
	if err := run([]string{"-root", root, "-check"}, &out); err != nil {
		t.Fatalf("check after write: %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("output = %q", out.String())
	}
}

// The point of -check: a change to data/ that nobody regenerated the index for
// has to fail, or the committed index silently stops describing the dataset.
func TestCheckFailsWhenTheDatasetMovedOn(t *testing.T) {
	root := dataset(t)
	if err := run([]string{"-root", root}, &bytes.Buffer{}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "series", "b.yaml"),
		[]byte("series:\n  id: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"-root", root, "-check"}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("check passed against a stale index")
	}
	if !strings.Contains(err.Error(), "out of date") {
		t.Errorf("error = %v, want it to mention the index is out of date", err)
	}
}

func TestCheckFailsWithNoIndexAtAll(t *testing.T) {
	err := run([]string{"-root", dataset(t), "-check"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "make index") {
		t.Fatalf("error = %v, want it to point at `make index`", err)
	}
}

func TestRunRejectsBadInput(t *testing.T) {
	if err := run([]string{"-nonsense"}, &bytes.Buffer{}); err == nil {
		t.Error("run accepted an unknown flag")
	}
	if err := run([]string{"-root", t.TempDir()}, &bytes.Buffer{}); err == nil {
		t.Error("run accepted a root with no data/ tree")
	}
}
