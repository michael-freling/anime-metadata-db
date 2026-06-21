package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
