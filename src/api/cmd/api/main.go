// Command api serves the read-only anime dataset over the Connect, gRPC and
// gRPC-Web protocols.
//
// Usage:
//
//	api [-addr :8080] [-dataset ./dataset] [-dataset-url https://.../dataset]
//
// The dataset comes from a directory when there is one and over HTTP when there
// is not — see resolveDataset, which is the whole of the difference between a
// checkout and a deployment.
//
// This is also the deployment entrypoint on Vercel, whose native Go builder
// compiles cmd/api and runs it as a web server, injecting the listen port via
// the PORT environment variable (which -addr defaults to). The h2c support
// enables cleartext HTTP/2 so full gRPC clients work without TLS.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	animedb "github.com/michael-freling/anime-metadata-db/src/animedb"
	"github.com/michael-freling/anime-metadata-db/src/api/internal/api"
)

// source is where the dataset is read from: exactly one of dir or url is set.
type source struct {
	dir string
	url string
}

// resolveDataset decides where the records come from, in this order:
//
//  1. an explicit -dataset-url, or ANIME_DATASET_URL;
//  2. an explicit -dataset, or a dataset/ directory found by walking up from
//     the working directory and then from the executable;
//  3. the commit this deployment was built from, fetched over HTTP.
//
// Disk before the platform's own commit is deliberate: in a checkout — a test,
// `go run ./cmd/api`, the Playwright suite — the working tree is what the
// author means, and it must win even when Vercel's variables happen to be set.
// On Vercel step 2 finds nothing, because the Go builder ships the binary and
// not the repository, so it falls through to step 3 and reads the dataset it
// was built from.
func resolveDataset(flagDir, flagURL string) (source, error) {
	if flagURL != "" {
		return source{url: flagURL}, nil
	}
	if env := os.Getenv("ANIME_DATASET_URL"); env != "" {
		return source{url: env}, nil
	}
	if flagDir != "" {
		return source{dir: flagDir}, nil
	}
	if env := os.Getenv("ANIME_DATASET_DIR"); env != "" {
		return source{dir: env}, nil
	}
	if dir := findDataset(); dir != "" {
		return source{dir: dir}, nil
	}
	// VERCEL_GIT_* are injected by the platform, so the deployed path needs
	// nothing configured by hand — which is the point: a dashboard variable
	// somebody has to remember to set is a way for a deployment to read the
	// wrong commit.
	owner, repo := os.Getenv("VERCEL_GIT_REPO_OWNER"), os.Getenv("VERCEL_GIT_REPO_SLUG")
	if sha := os.Getenv("VERCEL_GIT_COMMIT_SHA"); sha != "" && owner != "" && repo != "" {
		return source{url: animedb.RawGitHubBase(owner, repo, sha)}, nil
	}
	return source{}, fmt.Errorf("no dataset: pass -dataset <dir> or -dataset-url <url>, " +
		"or run where a dataset/ directory is reachable")
}

// findDataset walks up from the working directory and then from the executable
// looking for a dataset directory, so `go run ./cmd/api` works from anywhere in
// the tree and a built binary works beside a deployed copy.
//
// It requires the index, not just the directory name, so a half-populated or
// unrelated dataset/ is not mistaken for the real one.
func findDataset() string {
	var roots []string
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, wd)
	}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	for _, root := range roots {
		for dir := root; ; {
			candidate := filepath.Join(dir, "dataset")
			if _, err := os.Stat(filepath.Join(candidate, animedb.IndexPath)); err == nil {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return ""
}

// version is overridable at build time with -ldflags "-X main.version=...". At
// runtime a Vercel deployment's commit SHA takes precedence (see resolveVersion).
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run parses flags, builds the server and listens. It blocks until the server
// stops or fails.
func run(args []string, out io.Writer) error {
	srv, err := newServer(args, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "anime-metadata-db API listening on %s\n", srv.Addr)
	return srv.ListenAndServe()
}

// newServer parses args and returns a configured *http.Server. It is separated
// from run so the wiring is testable without binding a port.
func newServer(args []string, out io.Writer) (*http.Server, error) {
	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	fs.SetOutput(out)
	addr := fs.String("addr", defaultAddr(), "listen address (defaults to :$PORT, else :8080)")
	dir := fs.String("dataset", "", "dataset directory (the one holding data/); default: found by walking up")
	datasetURL := fs.String("dataset-url", "", "dataset base URL (the one holding data/); default: this deployment's commit")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	src, err := resolveDataset(*dir, *datasetURL)
	if err != nil {
		return nil, err
	}
	var handler http.Handler
	if src.url != "" {
		fmt.Fprintf(out, "dataset: %s\n", src.url)
		handler, err = api.NewRemote(src.url, resolveVersion())
	} else {
		fmt.Fprintf(out, "dataset: %s\n", src.dir)
		handler, err = api.New(src.dir, resolveVersion())
	}
	if err != nil {
		return nil, err
	}
	// Enable cleartext HTTP/2 (h2c) alongside HTTP/1.1 so full gRPC clients
	// work without TLS, using the stdlib's native support (Go 1.24+).
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{
		Addr:              *addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
	}, nil
}

// defaultAddr binds the port Vercel (or any PaaS) provides via $PORT, falling
// back to :8080 for local runs.
func defaultAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8080"
}

// resolveVersion prefers the Vercel deployment's commit SHA, then the
// build-time version, so GetStats reports the deployed revision.
func resolveVersion() string {
	if sha := os.Getenv("VERCEL_GIT_COMMIT_SHA"); sha != "" {
		return sha
	}
	return version
}
