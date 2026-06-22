package api

import (
	"fmt"
	"io/fs"
	"net/http"

	animedb "github.com/michael-freling/anime-metadata-db"
	"github.com/michael-freling/anime-metadata-db/gen/anime/v1/animev1connect"
)

// NewHandler builds the HTTP handler that serves AnimeService over the Connect,
// gRPC and gRPC-Web protocols, plus a human-readable index at "/".
func NewHandler(store *Store, version string) http.Handler {
	svc := NewService(store, version)
	mux := http.NewServeMux()
	rpcPath, h := animev1connect.NewAnimeServiceHandler(svc)
	mux.Handle(rpcPath, h)
	mux.HandleFunc("/", indexHandler(rpcPath))
	return mux
}

// New loads the embedded dataset and returns the API handler. It is the
// entrypoint used by both cmd/api and the Vercel function.
func New(version string) (http.Handler, error) {
	return newFromFS(animedb.DataFS, version)
}

// newFromFS builds the handler from an arbitrary dataset filesystem. It backs
// New and lets tests exercise the load-error path with a synthetic FS.
func newFromFS(fsys fs.FS, version string) (http.Handler, error) {
	store, err := NewStore(fsys)
	if err != nil {
		return nil, err
	}
	return NewHandler(store, version), nil
}

// indexHandler serves a short plain-text usage note at "/" and returns 404 for
// any other unrouted path.
func indexHandler(rpcPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "anime-metadata-db API (Connect/gRPC)\n\n")
		fmt.Fprintf(w, "Service: %s\n", animev1connect.AnimeServiceName)
		fmt.Fprintf(w, "Base path: %s\n\n", rpcPath)
		fmt.Fprintf(w, "Example (Connect, JSON over HTTP POST):\n")
		fmt.Fprintf(w, "  curl -X POST %sGetHealth \\\n", rpcPath)
		fmt.Fprintf(w, "    -H 'Content-Type: application/json' -d '{}'\n")
	}
}
