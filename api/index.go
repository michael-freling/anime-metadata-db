// Package handler is the Vercel serverless entrypoint for the anime-metadata-db
// API. Vercel's Go runtime (@vercel/go) invokes the exported Handler function
// for every request routed to /api by vercel.json. Handler delegates to the
// same http.Handler used by cmd/api, so the Connect/gRPC-Web service and the
// embedded dataset behave identically locally and on Vercel.
//
// Vercel serves this over HTTP/1.1, which supports the Connect protocol and
// gRPC-Web (full gRPC, which needs HTTP/2, is available only via cmd/api).
package handler

import (
	"net/http"
	"os"
	"sync"

	"github.com/michael-freling/anime-metadata-db/internal/api"
)

// built memoizes the handler across warm invocations of the same function
// instance, so the dataset is parsed once per cold start, not per request.
var (
	once     sync.Once
	built    http.Handler
	buildErr error
)

// version reports the deployed build. Vercel injects the commit SHA; locally it
// falls back to "vercel".
func version() string {
	if sha := os.Getenv("VERCEL_GIT_COMMIT_SHA"); sha != "" {
		return sha
	}
	return "vercel"
}

// load builds the API handler once.
func load() (http.Handler, error) {
	once.Do(func() {
		built, buildErr = api.New(version())
	})
	return built, buildErr
}

// Handler is the Vercel function entrypoint.
func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := load()
	if err != nil {
		http.Error(w, "failed to initialize API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	h.ServeHTTP(w, r)
}
