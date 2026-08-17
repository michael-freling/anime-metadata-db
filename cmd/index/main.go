// Command index regenerates data/index.tsv, the listing index the API serves
// browse, search and catalog requests from.
//
// The index is derived entirely from data/, so it is never edited by hand. It
// is committed because the API embeds it: the server has no build step of its
// own on Vercel, so anything it needs at runtime has to be in the repository.
//
// Run it after any change under data/:
//
//	make index
//
// CI runs it with -check, which regenerates the index and fails if the result
// differs from the committed file — the same guard the provenance doc has, for
// the same reason: a derived file that can silently drift is worse than no
// derived file at all.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/michael-freling/anime-metadata-db/internal/index"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		os.Exit(1)
	}
}

// verify re-reads a generated index, and is the real implementation behind
// run's backstop.
func verify(blob string) error {
	_, err := index.Open(blob)
	return err
}

func run(args []string, out io.Writer) error {
	return runWith(args, out, verify)
}

// runWith takes its verifier as a parameter so the failure path can be tested.
// By construction a correct writer never emits something the reader rejects,
// which is what makes this backstop otherwise unreachable from a test — and an
// untested backstop is one that can quietly stop being called.
func runWith(args []string, out io.Writer, verify func(string) error) error {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	root := fs.String("root", ".", "repository root holding data/")
	check := fs.Bool("check", false, "verify the committed index matches data/ instead of rewriting it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dataset, err := index.Load(os.DirFS(*root))
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := dataset.WriteTo(&buf); err != nil {
		return err
	}

	// Re-reading what was just written catches a writer that emits something the
	// reader cannot load — a mismatched column count, a dangling row reference
	// — at build time rather than at the server's next boot. It runs before the
	// file is touched, so a failure leaves the committed index alone rather
	// than replacing it with something unloadable.
	if err := verify(buf.String()); err != nil {
		return fmt.Errorf("generated index does not load: %w", err)
	}

	path := filepath.Join(*root, index.Path)
	if *check {
		committed, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w (run `make index`)", index.Path, err)
		}
		if !bytes.Equal(committed, buf.Bytes()) {
			return fmt.Errorf("%s is out of date — run `make index` and commit the result", index.Path)
		}
		fmt.Fprintf(out, "%s is up to date (%d bytes)\n", index.Path, buf.Len())
		return nil
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s (%d bytes)\n", index.Path, buf.Len())
	return nil
}
