package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	connectcors "connectrpc.com/cors"
)

// corsMaxAge is how long a browser may cache the preflight result. Every RPC is
// a POST with a Content-Type browsers do not treat as simple, so without this
// each call costs two round trips. A day is what connectrpc's own example uses;
// the answer never changes, because it does not depend on the request.
const corsMaxAge = 24 * time.Hour

// withCORS lets browsers on any origin call the API.
//
// The dataset is published so that other people can build on it, and the docs
// invite a plain `curl` from anywhere — but a browser is not curl. Without
// these headers the same request that works from a terminal fails from a web
// page, which quietly limits the audience to servers and CLIs. The interactive
// examples in the API reference are the immediate reason, and they are also the
// proof it works: they run in the reader's browser against the deployed API.
//
// Any origin, and no credentials. There is nothing to protect with an allowlist
// — the service is public, read-only, unauthenticated, and serves the same
// bytes to everyone — so an origin check would only decide who may read what is
// already open, while adding a value to keep in sync with every deployment.
// `Access-Control-Allow-Credentials` is deliberately never set: with it, `*` is
// rejected by browsers, and it would invite sending cookies to an API that has
// no notion of a session.
//
// The header lists come from connectrpc.com/cors rather than being written out
// here, because they are a property of the protocol: Connect, gRPC and gRPC-Web
// each carry their own headers, and a hand-copied list silently stops working
// when one of them gains another. Accept-Language is appended because this API
// resolves titles from it (see varyAcceptLanguage) — browsers may send it
// unasked, but a client that sets it explicitly needs it allowed.
func withCORS(next http.Handler) http.Handler {
	allowedMethods := strings.Join(connectcors.AllowedMethods(), ", ")
	allowedHeaders := strings.Join(append(connectcors.AllowedHeaders(), "Accept-Language"), ", ")
	// Vary is exposed alongside Connect's own list so a browser client can see
	// that a response is language-dependent; it is the one header this service
	// adds that a caller might reasonably read.
	exposedHeaders := strings.Join(append(connectcors.ExposedHeaders(), "Vary"), ", ")
	maxAge := strconv.Itoa(int(corsMaxAge.Seconds()))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Not a browser request. Adding the headers anyway would be
			// harmless but misleading in a curl -v transcript, which the docs
			// show verbatim.
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Expose-Headers", exposedHeaders)
		// Origin is echoed into Vary even though the allowed origin is constant:
		// a cache in front of this must not serve a response that carries CORS
		// headers to a request that had no Origin, or the other way round.
		h.Add("Vary", "Origin")

		if r.Method != http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// A preflight is answered here and never reaches the service. It also
		// must not 404: the RPC paths only accept POST, so letting OPTIONS
		// through would fail the preflight and block every browser call.
		h.Set("Access-Control-Allow-Methods", allowedMethods)
		h.Set("Access-Control-Allow-Headers", allowedHeaders)
		h.Set("Access-Control-Max-Age", maxAge)
		h.Add("Vary", "Access-Control-Request-Method")
		h.Add("Vary", "Access-Control-Request-Headers")
		w.WriteHeader(http.StatusNoContent)
	})
}
