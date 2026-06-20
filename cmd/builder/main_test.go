package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michael-freling/anime-metadata-db/internal/testsupport"
)

func repoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides", "demon-slayer.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(testsupport.DemonSlayerOverride), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func runCmd(args ...string) (int, string, string) {
	var out, errBuf bytes.Buffer
	code := run(args, testsupport.FakeFetcher{}, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func TestRunHelp(t *testing.T) {
	code, out, _ := runCmd("--help")
	if code != 0 {
		t.Fatalf("help exit code = %d", code)
	}
	if !strings.Contains(out, "Compile anime franchise overrides") {
		t.Errorf("help output unexpected: %q", out)
	}
}

func TestRunInitBuildRefresh(t *testing.T) {
	dir := repoDir(t)

	if code, _, errOut := runCmd("--dir", dir, "init"); code != 0 {
		t.Fatalf("init exit %d: %s", code, errOut)
	}
	if code, _, errOut := runCmd("--dir", dir, "build"); code != 0 {
		t.Fatalf("build exit %d: %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "demon-slayer.yaml")); err != nil {
		t.Errorf("data not built: %v", err)
	}
	if code, _, errOut := runCmd("--dir", dir, "refresh"); code != 0 {
		t.Fatalf("refresh exit %d: %s", code, errOut)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, errOut := runCmd("frobnicate")
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if !strings.Contains(errOut, "error:") {
		t.Errorf("expected error on stderr, got %q", errOut)
	}
}

func TestRunBuildError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overrides", "bad.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "series:\n  id: bad\n  seasons:\n    - id: x\n      number: 1\n      externalIds: { anilistId: 404404 }\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := runCmd("--dir", dir, "init"); code != 0 {
		t.Fatal("init should succeed")
	}
	code, _, errOut := runCmd("--dir", dir, "build")
	if code != 1 {
		t.Errorf("expected build failure exit 1, got %d", code)
	}
	if !strings.Contains(errOut, "error:") {
		t.Errorf("expected error message, got %q", errOut)
	}
}
