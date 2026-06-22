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
	srv, err := newServer(nil, io.Discard)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	if srv.Addr != ":8080" {
		t.Errorf("default addr = %q, want :8080", srv.Addr)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anime.v1.AnimeService/GetHealth", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("health response code=%d body=%s", rec.Code, rec.Body.String())
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
