// Command api serves the read-only anime dataset over the Connect, gRPC and
// gRPC-Web protocols. The dataset is embedded, so the binary is self-contained
// and needs no filesystem at runtime.
//
// Usage:
//
//	api [-addr :8080]
//
// The h2c wrapper enables cleartext HTTP/2 so full gRPC clients work locally
// without TLS. On Vercel the same handler is served over HTTP/1.1 by the
// function in api/.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/michael-freling/anime-metadata-db/internal/api"
)

// version is overridable at build time with -ldflags "-X main.version=...".
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
	addr := fs.String("addr", ":8080", "listen address")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	handler, err := api.New(version)
	if err != nil {
		return nil, err
	}
	// Enable cleartext HTTP/2 (h2c) alongside HTTP/1.1 so full gRPC clients
	// work locally without TLS, using the stdlib's native support (Go 1.24+).
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
