package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewLoadsEmbeddedDataset(t *testing.T) {
	h, err := New("v-test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if h == nil {
		t.Fatal("New returned a nil handler")
	}
}

func TestNewFromFSError(t *testing.T) {
	// A filesystem with no data/series dir fails to load.
	if _, err := newFromFS(fstest.MapFS{}, "v-test"); err == nil {
		t.Fatal("expected error for an empty dataset filesystem")
	}
}

func TestHandlerServesConnectJSON(t *testing.T) {
	srv := httptest.NewServer(NewHandler(mustStore(t), "v-test"))
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/anime.v1.AnimeService/GetHealth",
		"application/json",
		strings.NewReader("{}"),
	)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	for _, want := range []string{`"status":"ok"`, `"version":"v-test"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body %s missing %q", body, want)
		}
	}
}

func TestHandlerIndexAndNotFound(t *testing.T) {
	srv := httptest.NewServer(NewHandler(mustStore(t), "v-test"))
	defer srv.Close()

	// Root serves the usage note.
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "anime.v1.AnimeService") {
		t.Errorf("index status=%d body=%s", resp.StatusCode, body)
	}

	// Any other unrouted path is a 404.
	resp, err = http.Get(srv.URL + "/nope")
	if err != nil {
		t.Fatalf("GET /nope: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path status = %d, want 404", resp.StatusCode)
	}
}
