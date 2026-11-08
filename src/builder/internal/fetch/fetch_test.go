package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	c := NewClient(nil)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q", body)
	}
}

func TestGetNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := NewClient(srv.Client()).Get(context.Background(), srv.URL); err == nil {
		t.Error("expected error for 404")
	}
}

func TestGetBadURL(t *testing.T) {
	// A control character in the URL fails request construction.
	if _, err := NewClient(nil).Get(context.Background(), "http://bad\x00url"); err == nil {
		t.Error("expected request build error")
	}
}

func TestGetTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // now refuses connections
	if _, err := NewClient(nil).Get(context.Background(), url); err == nil {
		t.Error("expected transport error")
	}
}

func TestGetContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewClient(nil).Get(ctx, srv.URL); err == nil {
		t.Error("expected context cancellation error")
	}
}

func TestChecksum(t *testing.T) {
	// Known SHA-256 of the empty input.
	if got := Checksum(nil); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("Checksum(nil) = %q", got)
	}
	if Checksum([]byte("a")) == Checksum([]byte("b")) {
		t.Error("different inputs should hash differently")
	}
}

func TestNewClientDefault(t *testing.T) {
	if NewClient(nil).HTTP != http.DefaultClient {
		t.Error("nil should default to http.DefaultClient")
	}
	custom := &http.Client{}
	if NewClient(custom).HTTP != custom {
		t.Error("custom client should be retained")
	}
}

// TestGetRetriesWhenToldToSlowDown is the failure that arrived with the data.
// At 1,075 series the build made 22 Wikidata requests; at 6,382 it makes about
// 128 back to back, and Wikidata answered 429 partway through, failing `init`
// outright.
func TestGetRetriesWhenToldToSlowDown(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var waits []time.Duration
	c := &Client{Sleep: func(d time.Duration) { waits = append(waits, d) }}
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want ok", body)
	}
	if hits != 3 {
		t.Errorf("made %d requests, want 3", hits)
	}
	// Doubling, so a burst limit is ridden out rather than hammered.
	if len(waits) != 2 || waits[0] != time.Second || waits[1] != 2*time.Second {
		t.Errorf("waits = %v, want [1s 2s]", waits)
	}
}

func TestGetHonoursRetryAfter(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		if hits == 1 {
			// The server knows its own window; its number must win over the
			// backoff schedule.
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var waits []time.Duration
	c := &Client{Sleep: func(d time.Duration) { waits = append(waits, d) }}
	if _, err := c.Get(context.Background(), srv.URL); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(waits) != 1 || waits[0] != 7*time.Second {
		t.Errorf("waits = %v, want [7s]", waits)
	}
}

// TestGetDoesNotRetryAPermanentFailure keeps upstream drift loud. A moved
// release asset answers 404, and that is exactly what the live-source jobs
// exist to surface — retrying it would turn a clear failure into a slow one.
func TestGetDoesNotRetryAPermanentFailure(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		http.NotFound(w, nil)
	}))
	defer srv.Close()

	var waits []time.Duration
	c := &Client{Sleep: func(d time.Duration) { waits = append(waits, d) }}
	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Get succeeded on a 404")
	}
	if hits != 1 {
		t.Errorf("made %d requests, want 1", hits)
	}
	if len(waits) != 0 {
		t.Errorf("waited %v before giving up on a 404", waits)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v, want it to carry the status", err)
	}
}

func TestGetGivesUpAfterTheAttemptBudget(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &Client{Sleep: func(time.Duration) {}}
	_, err := c.Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("Get succeeded against a server that only refuses")
	}
	if hits != maxAttempts {
		t.Errorf("made %d requests, want %d", hits, maxAttempts)
	}
	// A rate limit that never lifts has to read as that, not as a generic
	// failure, or the fix looks like the wrong one.
	for _, want := range []string{"429", "gave up"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to mention %s", err, want)
		}
	}
}

func TestGetStopsWhenTheContextEnds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{Sleep: func(time.Duration) { cancel() }}
	if _, err := c.Get(ctx, srv.URL); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
