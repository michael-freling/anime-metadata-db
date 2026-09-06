package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"connectrpc.com/connect"

	animedb "github.com/michael-freling/anime-metadata-db"
	"github.com/michael-freling/anime-metadata-db/api/internal/gen/anime/v1/animev1connect"
	"github.com/michael-freling/anime-metadata-db/api/internal/gen/browse/v1/browsev1connect"
	"github.com/michael-freling/anime-metadata-db/api/internal/index"
)

// NewHandler builds the HTTP handler that serves both services over the
// Connect, gRPC and gRPC-Web protocols, plus a human-readable index at "/".
//
// Two services, one binary, one host: anime.v1.AnimeService is the public API,
// and browse.v1.BrowseService is the undocumented one anime-metadata-web calls.
// They are separate packages rather than separate deployments because the split
// is about what is promised, not about who can reach it — the dataset is public
// either way. What the split buys is that the published reference describes
// three methods, and the site can change its own eight without that being a
// breaking change to anybody.
//
// Wrapped in withCORS so browsers can call it, which a terminal client neither
// needs nor notices. It wraps the whole mux rather than just the RPC paths so a
// preflight to any path gets a consistent answer instead of a 404 from the
// catch-all below.
func NewHandler(store *Store, version string) http.Handler {
	mux := http.NewServeMux()
	opts := connect.WithInterceptors(varyAcceptLanguage())

	rpcPath, h := animev1connect.NewAnimeServiceHandler(NewService(store, version), opts)
	mux.Handle(rpcPath, h)

	browsePath, browseHandler := browsev1connect.NewBrowseServiceHandler(NewBrowseService(store, version), opts)
	mux.Handle(browsePath, browseHandler)

	mux.HandleFunc("/", indexHandler(rpcPath))
	return withCORS(mux)
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
		fmt.Fprintf(w, "  curl -X POST %sSearchSeries \\\n", rpcPath)
		fmt.Fprintf(w, "    -H 'Content-Type: application/json' -d '{\"query\": \"fate\"}'\n")
	}
}
