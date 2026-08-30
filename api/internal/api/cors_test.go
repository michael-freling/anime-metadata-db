package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// corsTestHandler is the handler under test wrapped around a marker, so a test
// can tell "the request reached the service" from "CORS answered it".
func corsTestHandler(reached *bool) http.Handler {
	return withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	}))
}

// A preflight has to be answered by the middleware, because the RPC paths only
// accept POST. Before this existed the browser got a 405 and every call failed
// before it was made — which is exactly what the deployed API returned.
func TestCORSAnswersPreflightWithoutReachingTheService(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodOptions, "/anime.v1.AnimeService/GetHealth", nil)
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,connect-protocol-version")
	rec := httptest.NewRecorder()
	corsTestHandler(&reached).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want %d", rec.Code, http.StatusNoContent)
	}
	if reached {
		t.Error("a preflight must not reach the service")
	}
	h := rec.Header()
	if got := h.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("allow-origin: got %q, want %q", got, "*")
	}
	// Credentials with a wildcard origin is rejected by browsers outright, and
	// this API has no session to send anyway.
	if got := h.Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("credentials must never be allowed, got %q", got)
	}
	if got := h.Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("max-age: got %q, want 86400", got)
	}

	// The headers a Connect browser client actually sends must be allowed, or
	// the preflight succeeds and the real request is still blocked.
	allow := strings.ToLower(h.Get("Access-Control-Allow-Headers"))
	for _, want := range []string{"content-type", "connect-protocol-version", "accept-language"} {
		if !strings.Contains(allow, want) {
			t.Errorf("allow-headers is missing %q: %s", want, allow)
		}
	}
	if m := h.Get("Access-Control-Allow-Methods"); !strings.Contains(m, http.MethodPost) {
		t.Errorf("allow-methods must include POST, got %q", m)
	}
}

// A real (non-preflight) cross-origin request must both reach the service and
// carry the headers that let the browser hand the body to JavaScript.
func TestCORSPassesRealRequestsThrough(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/anime.v1.AnimeService/GetHealth", strings.NewReader("{}"))
	req.Header.Set("Origin", "https://example.test")
	rec := httptest.NewRecorder()
	corsTestHandler(&reached).ServeHTTP(rec, req)

	if !reached {
		t.Error("a real request must reach the service")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("allow-origin: got %q, want %q", got, "*")
	}
	// Without this the response arrives but the client cannot read the gRPC
	// status headers, so an error surfaces as an unexplained failure.
	expose := strings.ToLower(rec.Header().Get("Access-Control-Expose-Headers"))
	for _, want := range []string{"grpc-status", "grpc-message", "vary"} {
		if !strings.Contains(expose, want) {
			t.Errorf("expose-headers is missing %q: %s", want, expose)
		}
	}
	// Origin varies even though the allowed origin is constant, so a shared
	// cache cannot serve a CORS-annotated response to a request that had none.
	if v := rec.Header().Values("Vary"); !containsFold(v, "Origin") {
		t.Errorf("Vary must include Origin, got %v", v)
	}
}

// A request with no Origin is not from a browser. It must be untouched, because
// the docs print `curl -v` transcripts and headers nobody asked for are noise.
func TestCORSLeavesNonBrowserRequestsAlone(t *testing.T) {
	var reached bool
	req := httptest.NewRequest(http.MethodPost, "/anime.v1.AnimeService/GetHealth", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	corsTestHandler(&reached).ServeHTTP(rec, req)

	if !reached {
		t.Error("a request without an Origin must reach the service")
	}
	for _, h := range []string{"Access-Control-Allow-Origin", "Access-Control-Expose-Headers"} {
		if got := rec.Header().Get(h); got != "" {
			t.Errorf("%s must not be set without an Origin, got %q", h, got)
		}
	}
}

// The whole point is that the deployed service answers a browser. This drives
// the real handler rather than the marker, so it fails if NewHandler ever stops
// wrapping the mux.
func TestHandlerAnswersPreflightOnAnRPCPath(t *testing.T) {
	h, err := newFromFS(newTestFS(), "test")
	if err != nil {
		t.Fatalf("newFromFS: %v", err)
	}
	req := httptest.NewRequest(http.MethodOptions, "/anime.v1.AnimeService/GetHealth", nil)
	req.Header.Set("Origin", "https://example.test")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("the wired handler must answer a preflight: got %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("allow-origin: got %q, want %q", got, "*")
	}
}

// containsFold reports whether any value equals want, case-insensitively.
func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}
