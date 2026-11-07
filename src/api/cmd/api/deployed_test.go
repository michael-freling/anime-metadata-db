package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// committedDataset is the repository's dataset/ directory, reached from this
// package. It is served to the subprocess over HTTP rather than handed to it as
// a path, because a path is the one thing the deployed binary does not have.
const committedDataset = "../../../../dataset"

// TestTheBinaryServesWithNoRepositoryOnDisk runs the built binary the way the
// deployment runs it: from a directory with no dataset/ in any parent, with no
// repository anywhere near it, and with nothing but the platform's own
// variables in its environment.
//
// This is the test that was missing. Every other suite runs inside a checkout
// — server_test.go hands New a path into the repository, and the Playwright
// suite boots `go run ./cmd/api` with cwd inside src/api so walking up finds
// dataset/ — so all of them passed while the deployed function could not find
// its data at all. The property that broke is only false where the repository
// is absent, and nothing ran the binary in that shape.
func TestTheBinaryServesWithNoRepositoryOnDisk(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the binary")
	}

	// The origin stands in for raw.githubusercontent.com, serving the real
	// committed dataset under the same layout, so the paths the index carries
	// are exercised rather than a fixture's.
	origin := httptest.NewServer(http.StripPrefix("/dataset/", http.FileServer(http.Dir(committedDataset))))
	defer origin.Close()

	bin := filepath.Join(t.TempDir(), "api")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// A separate empty directory: /tmp has no dataset/ above it, so findDataset
	// fails here exactly as it does in the function. The binary also lives in a
	// temp directory, so the walk from the executable finds nothing either.
	workdir := t.TempDir()
	if found := datasetAbove(workdir); found != "" {
		t.Fatalf("the test's own working directory has a dataset at %s, so this would prove nothing", found)
	}

	port := freePort(t)
	cmd := exec.Command(bin, "-addr", fmt.Sprintf("127.0.0.1:%d", port))
	cmd.Dir = workdir
	// Deliberately not the ambient environment: this states the whole input,
	// and an inherited ANIME_DATASET_DIR would quietly restore the thing the
	// test exists to remove.
	cmd.Env = []string{
		"HOME=" + workdir,
		"ANIME_DATASET_URL=" + origin.URL + "/dataset",
	}
	var log safeBuffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Waited on in the background so an early exit is observable immediately.
	// Polling ProcessState instead would see nothing until something called
	// Wait, so a server that died during startup — the failure this test is
	// about — would take the full deadline to report.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-exited
	})

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitReady(t, base, exited, &log)

	body := post(t, base+"/anime.v1.AnimeService/SearchSeries", `{"query":"frieren","limit":1}`)
	if !strings.Contains(body, "sousou-no-frieren") {
		t.Errorf("SearchSeries from a fetched dataset returned %q", body)
	}

	// A detail request reads a record, not the index, so this is the half that
	// a working index alone would not prove.
	body = post(t, base+"/anime.v1.AnimeService/GetSeries", `{"id":"sousou-no-frieren"}`)
	if !strings.Contains(body, "seasons") {
		t.Errorf("GetSeries from a fetched dataset returned %q", body)
	}
}

// TestTheBinaryRefusesToStartWithNoDataset asserts the other half: with no
// directory and nothing configured, it must exit rather than come up and serve
// an empty catalogue, which would read to a caller as a dataset that had lost
// its contents.
func TestTheBinaryRefusesToStartWithNoDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the binary")
	}

	bin := filepath.Join(t.TempDir(), "api")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	workdir := t.TempDir()
	cmd := exec.Command(bin, "-addr", "127.0.0.1:0")
	cmd.Dir = workdir
	cmd.Env = []string{"HOME=" + workdir}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("the binary started with no dataset at all:\n%s", out)
	}
	if !strings.Contains(string(out), "no dataset") {
		t.Errorf("output did not explain the failure:\n%s", out)
	}
}

// datasetAbove reports the first dataset/data/index.tsv at or above dir, so a
// test can prove its own premise instead of assuming it.
func datasetAbove(dir string) string {
	for {
		candidate := filepath.Join(dir, "dataset")
		if _, err := os.Stat(filepath.Join(candidate, "data/index.tsv")); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// freePort asks the OS for a port and releases it. The subprocess cannot report
// the port it chose, so one is picked for it.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// waitReady blocks until the server answers, and fails the moment it exits
// instead — the failure this test is about is a process that dies during
// startup, so it is reported in a second rather than timed out on in a minute.
func waitReady(t *testing.T, base string, exited <-chan error, log *safeBuffer) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case err := <-exited:
			t.Fatalf("the server exited during startup (%v):\n%s", err, log.String())
		case <-deadline:
			t.Fatalf("the server never became ready:\n%s", log.String())
		default:
		}
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// safeBuffer collects the subprocess's output. os/exec writes it from its own
// goroutine while the test reads it on failure, so the writes are guarded —
// a plain strings.Builder here is a data race that -race fails on.
type safeBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func post(t *testing.T, url, body string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", url, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post %s: %s\n%s", url, resp.Status, out)
	}
	return string(out)
}
