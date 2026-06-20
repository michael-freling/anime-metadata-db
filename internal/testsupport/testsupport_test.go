package testsupport

import (
	"context"
	"errors"
	"testing"
)

func TestFakeFetcherRouting(t *testing.T) {
	ctx := context.Background()
	f := FakeFetcher{}
	cases := map[string]string{
		"http://x/anime-offline-database.json": OfflineDBJSON,
		"http://x/anime-movieset-list.xml":     MovieSetXML,
		"http://x/anime-list-master.xml":       AnimeListXML,
	}
	for url, want := range cases {
		got, err := f.Get(ctx, url)
		if err != nil {
			t.Fatalf("Get(%s): %v", url, err)
		}
		if string(got) != want {
			t.Errorf("Get(%s) returned the wrong fixture", url)
		}
	}

	if _, err := f.Get(ctx, "http://x/unknown"); err == nil {
		t.Error("expected error for unknown url")
	}
}

func TestFakeFetcherHooks(t *testing.T) {
	ctx := context.Background()
	if _, err := (FakeFetcher{Err: errors.New("boom")}).Get(ctx, "http://x/offline"); err == nil {
		t.Error("expected Err to be returned")
	}
	if _, err := (FakeFetcher{FailURL: "movieset"}).Get(ctx, "http://x/anime-movieset-list.xml"); err == nil {
		t.Error("expected FailURL to trigger an error")
	}
	// FailURL that doesn't match still serves the fixture.
	if _, err := (FakeFetcher{FailURL: "nomatch"}).Get(ctx, "http://x/offline"); err != nil {
		t.Errorf("non-matching FailURL should succeed: %v", err)
	}
}
