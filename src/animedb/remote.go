package animedb

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// OpenRemote returns the record filesystem and the listing index for the
// dataset published under base.
//
// base names the dataset root — the directory holding data/ — so a record is
// fetched from base + "/" + the path the index already stores against it, and
// the remote and on-disk filesystems answer the same names. RawGitHubBase
// builds one for a commit of this repository.
//
// Only the index is fetched here, the same single read Open does. Records are
// fetched when a request asks for one and cached per process, so a warm
// instance converges on the hot set rather than pulling the catalogue.
func OpenRemote(base string, client *http.Client) (fs.FS, string, error) {
	if _, err := url.Parse(base); err != nil {
		return nil, "", fmt.Errorf("dataset base url %q: %w", base, err)
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	fsys := &remoteFS{
		base:   strings.TrimSuffix(base, "/"),
		client: client,
		cache:  map[string][]byte{},
		budget: cacheBudget,
	}
	index, err := fsys.fetch(IndexPath)
	if err != nil {
		return nil, "", fmt.Errorf("read dataset index: %w", err)
	}
	return fsys, string(index), nil
}

// RawGitHubBase is the dataset root of one commit of a public GitHub
// repository, served by raw.githubusercontent.com.
//
// Pinned to a commit rather than a branch deliberately: a deployment then reads
// the dataset it was built from for as long as it lives, so a push to the
// default branch cannot change what an older deployment serves.
func RawGitHubBase(owner, repo, ref string) string {
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/dataset", owner, repo, ref)
}

// cacheBudget caps what one process will hold in record bytes. Records are a
// few KB each, so this is thousands of them, while still being a ceiling rather
// than "whatever has been asked for since boot" — which is the reason for
// reading records one id at a time in the first place.
const cacheBudget = 8 << 20

// remoteFS is an fs.FS over HTTP: it fetches the file named by Open and caches
// the bytes. Entries are never evicted, only refused once the budget is spent,
// which keeps a warm instance fast without letting a crawl of every id grow
// without bound.
type remoteFS struct {
	base   string
	client *http.Client

	mu     sync.Mutex
	cache  map[string][]byte
	budget int
}

// Open fetches name and returns it as a file. Deliberately no ReadFile method:
// fs.ReadFile's fallback copies through Open, and handing back the cached slice
// itself would let a caller scribble on every later read of the same record.
func (r *remoteFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	body, err := r.fetch(name)
	if err != nil {
		return nil, err
	}
	return newMemFile(path.Base(name), body), nil
}

// fetch returns name's bytes, from the cache when they are there.
func (r *remoteFS) fetch(name string) ([]byte, error) {
	r.mu.Lock()
	cached, ok := r.cache[name]
	r.mu.Unlock()
	if ok {
		return cached, nil
	}

	req, err := http.NewRequest(http.MethodGet, r.base+"/"+name, nil)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	// Named so the traffic is attributable in an upstream's logs, and so a
	// rate-limit conversation has something to point at.
	req.Header.Set("User-Agent", "anime-metadata-db-api/1 (+https://github.com/michael-freling/anime-metadata-db)")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	// Discarded deliberately: the response has been read or abandoned by the
	// time this runs, and a close error at that point says nothing a caller
	// could act on.
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		// Mapped to fs.ErrNotExist rather than passed through as an opaque
		// failure, so a missing record reads as CodeNotFound to a caller
		// exactly as it does on disk.
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	case resp.StatusCode != http.StatusOK:
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("http %s", resp.Status)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	r.mu.Lock()
	if len(body) <= r.budget {
		r.cache[name] = body
		r.budget -= len(body)
	}
	r.mu.Unlock()
	return body, nil
}
