package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/anime.v1.AnimeService/GetHealth", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")

	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestHandlerIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	Handler(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "anime.v1.AnimeService") {
		t.Errorf("index code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestVersionFallback(t *testing.T) {
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "")
	if got := version(); got != "vercel" {
		t.Errorf("version() = %q, want vercel", got)
	}
	t.Setenv("VERCEL_GIT_COMMIT_SHA", "abc123")
	if got := version(); got != "abc123" {
		t.Errorf("version() = %q, want abc123", got)
	}
}
