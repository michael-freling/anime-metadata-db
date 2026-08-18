package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"connectrpc.com/connect"

	animedb "github.com/michael-freling/anime-metadata-db"
	"github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1/animev1connect"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
)

// NewHandler builds the HTTP handler that serves AnimeService over the Connect,
// gRPC and gRPC-Web protocols, plus a human-readable index at "/".
func NewHandler(store *Store, version string) http.Handler {
	svc := NewService(store, version)
	mux := http.NewServeMux()
	rpcPath, h := animev1connect.NewAnimeServiceHandler(svc, connect.WithInterceptors(varyAcceptLanguage()))
	mux.Handle(rpcPath, h)
	mux.HandleFunc("/", indexHandler(rpcPath))
	return mux
}

// varyAcceptLanguage marks every response as varying by Accept-Language, since
// titles are resolved from that header, so caches key entries per language.
func varyAcceptLanguage() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if resp != nil {
				resp.Header().Add("Vary", "Accept-Language")
			}
			return resp, err
		}
	}
}

// New opens the embedded listing index and returns the API handler. It is the
// entrypoint used by cmd/api.
//
// Only the index is read here. Record files are parsed when a request asks for
// one, so this returns in microseconds regardless of how large the dataset is.
func New(version string) (http.Handler, error) {
	ix, err := index.Open(animedb.Index)
	if err != nil {
		return nil, err
	}
	return NewHandler(NewStore(ix, animedb.DataFS), version), nil
}

// newFromFS builds the handler from an arbitrary dataset filesystem, indexing
// it in memory. It lets tests drive the whole server off a fixture dataset, and
// exercise the load-error path with a synthetic FS.
func newFromFS(fsys fs.FS, version string) (http.Handler, error) {
	store, err := NewStoreFromDataset(fsys)
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
