package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewServerServesHealth(t *testing.T) {
	t.Setenv("PORT", "") // ensure the local default, independent of the CI env
	srv, err := newServer(nil, io.Discard)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	if srv.Addr != ":8080" {
		t.Errorf("default addr = %q, want :8080", srv.Addr)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anime.v1.AnimeService/GetStats", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler.ServeHTTP(rec, req)
	// Asserts a real figure rather than the old `"status":"ok"`, which was a
	// constant the handler wrote unconditionally — it proved the route was
	// wired, not that the dataset had loaded.
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"series":`) {
		t.Errorf("stats response code=%d body=%s", rec.Code, rec.Body.String())
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
