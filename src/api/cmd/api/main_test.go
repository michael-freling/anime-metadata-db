package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServerServesBothServices(t *testing.T) {
	t.Setenv("PORT", "") // ensure the local default, independent of the CI env
	srv, err := newServer(nil, io.Discard)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	if srv.Addr != ":8080" {
		t.Errorf("default addr = %q, want :8080", srv.Addr)
	}

	// Both services are mounted on the one binary, so both paths are checked:
	// a refactor that dropped either handler from the mux would otherwise show
	// up only as a 404 in production.
	//
	// Each asserts a real figure rather than a constant the handler writes
	// unconditionally — that would prove the route was wired, not that the
	// dataset had loaded.
	for _, tc := range []struct{ path, body, want string }{
		{"/anime.v1.AnimeService/SearchSeries", "{}", `"totalSize":`},
		{"/browse.v1.BrowseService/GetStats", "{}", `"series":`},
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		srv.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s: code=%d body=%s", tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestNewServerFlagError(t *testing.T) {
	if _, err := newServer([]string{"-nope"}, io.Discard); err == nil {
		t.Error("expected error for unknown flag")
	}
}

func TestRunListenError(t *testing.T) {
	// An address with no port fails fast in ListenAndServe, exercising run's
	// error path without binding a real port.
	var out bytes.Buffer
	if err := run([]string{"-addr", "bogus-no-port"}, &out); err == nil {
		t.Error("expected ListenAndServe error for a portless address")
	}
}

func TestDefaultAddrUsesPort(t *testing.T) {
	t.Setenv("PORT", "3000")
	if got := defaultAddr(); got != ":3000" {
		t.Errorf("defaultAddr() with PORT=3000 = %q, want :3000", got)
	}
	t.Setenv("PORT", "")
	if got := defaultAddr(); got != ":8080" {
		t.Errorf("defaultAddr() fallback = %q, want :8080", got)
	}
}

func TestResolveVersion(t *testing.T) {
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "abc123")
	if got := resolveVersion(); got != "abc123" {
		t.Errorf("resolveVersion() = %q, want abc123", got)
	}
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "")
	if got := resolveVersion(); got != version {
		t.Errorf("resolveVersion() fallback = %q, want %q", got, version)
	}
}
