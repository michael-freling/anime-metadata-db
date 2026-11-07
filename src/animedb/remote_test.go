package animedb

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// stub serves a small dataset and counts requests, so a test can tell a cache
// hit from a fetch.
func stub(t *testing.T) (base string, hits *atomic.Int64) {
	t.Helper()
	hits = new(atomic.Int64)
	files := map[string]string{
		"/dataset/data/index.tsv":           "!anime-metadata-db-index\t1\n",
		"/dataset/data/series/frieren.yaml": "series:\n    id: frieren\n",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if got := r.Header.Get("User-Agent"); !strings.Contains(got, "anime-metadata-db") {
			t.Errorf("User-Agent = %q, want it to name this project", got)
		}
		if r.URL.Path == "/dataset/data/series/boom.yaml" {
			http.Error(w, "upstream sulked", http.StatusInternalServerError)
			return
		}
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/dataset", hits
}

func TestOpenRemoteReadsTheIndexThenRecordsOnDemand(t *testing.T) {
	base, hits := stub(t)

	fsys, idx, err := OpenRemote(base, nil)
	if err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	if !strings.HasPrefix(idx, "!anime-metadata-db-index") {
		t.Errorf("index = %q, want the index header", idx)
	}
	// Opening the dataset must cost exactly one request. The reason the server
	// reads an index at all is that boot does not scale with the catalogue,
	// and that has to hold for the fetched source too.
	if got := hits.Load(); got != 1 {
		t.Errorf("opening the dataset made %d requests, want 1", got)
	}

	body, err := fs.ReadFile(fsys, "data/series/frieren.yaml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(body), "id: frieren") {
		t.Errorf("record = %q", body)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("after one record: %d requests, want 2", got)
	}

	// Cached: a second read of the same record must not go back out.
	if _, err := fs.ReadFile(fsys, "data/series/frieren.yaml"); err != nil {
		t.Fatalf("second ReadFile: %v", err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("re-reading a record made %d requests, want it cached at 2", got)
	}
}

func TestOpenRemoteMapsAMissingRecordToErrNotExist(t *testing.T) {
	base, _ := stub(t)
	fsys, _, err := OpenRemote(base, nil)
	if err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	// A 404 has to arrive as fs.ErrNotExist, or the server answers "internal
	// error" where reading from disk would have answered "not found".
	if _, err := fs.ReadFile(fsys, "data/series/nope.yaml"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing record: err = %v, want fs.ErrNotExist", err)
	}
}

func TestOpenRemoteSurfacesAnUpstreamFailure(t *testing.T) {
	base, _ := stub(t)
	fsys, _, err := OpenRemote(base, nil)
	if err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	_, err = fs.ReadFile(fsys, "data/series/boom.yaml")
	if err == nil {
		t.Fatal("a 500 from upstream returned no error")
	}
	// Distinct from the 404 case above: a sulking upstream must not be
	// reported as a record that does not exist, which would turn an outage
	// into 502 confident "not found" answers.
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a 500 was reported as a missing file: %v", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %v, want it to carry the status", err)
	}
}

func TestOpenRemoteFailsWhenTheIndexIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, _, err := OpenRemote(srv.URL+"/dataset", nil)
	if err == nil {
		t.Fatal("OpenRemote succeeded with no index")
	}
	if !strings.Contains(err.Error(), "read dataset index") {
		t.Errorf("err = %v, want it to name the index", err)
	}
}

func TestOpenRemoteRejectsAnInvalidPath(t *testing.T) {
	base, _ := stub(t)
	fsys, _, err := OpenRemote(base, nil)
	if err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	// fs.FS forbids these, and a filesystem that forwarded them into a URL
	// would reach outside the dataset on whatever host is serving it.
	for _, name := range []string{"../etc/passwd", "/absolute", "data/../../x"} {
		if _, err := fsys.Open(name); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("Open(%q): err = %v, want fs.ErrInvalid", name, err)
		}
	}
}

func TestRawGitHubBasePinsTheCommit(t *testing.T) {
	got := RawGitHubBase("michael-freling", "anime-metadata-db", "abc123")
	want := "https://raw.githubusercontent.com/michael-freling/anime-metadata-db/abc123/dataset"
	if got != want {
		t.Errorf("RawGitHubBase() = %q, want %q", got, want)
	}
}

// TestFetchedFilesLookLikeFiles covers the fs.FileInfo a fetched record
// reports. Anything walking the dataset generically — and the index builder
// does — asks for these, so they have to be answerable rather than panic.
func TestFetchedFilesLookLikeFiles(t *testing.T) {
	base, _ := stub(t)
	fsys, _, err := OpenRemote(base, nil)
	if err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	f, err := fsys.Open("data/series/frieren.yaml")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Name(); got != "frieren.yaml" {
		t.Errorf("Name() = %q, want the base name", got)
	}
	if got, want := info.Size(), int64(len("series:\n    id: frieren\n")); got != want {
		t.Errorf("Size() = %d, want %d", got, want)
	}
	if info.IsDir() {
		t.Error("IsDir() = true for a record")
	}
	if got := info.Mode(); got.Perm() == 0 || got&fs.ModeDir != 0 {
		t.Errorf("Mode() = %v, want a readable regular file", got)
	}
	// Nothing is served from a filesystem with timestamps, so there is no
	// honest value here and the zero time says so.
	if !info.ModTime().IsZero() {
		t.Errorf("ModTime() = %v, want the zero time", info.ModTime())
	}
	if info.Sys() != nil {
		t.Errorf("Sys() = %v, want nil", info.Sys())
	}
}

func TestOpenRemoteRejectsAnUnparseableBase(t *testing.T) {
	// A control character is one of the few things url.Parse refuses, and the
	// point is that a bad base fails here rather than as a confusing per-record
	// error on the first detail request.
	if _, _, err := OpenRemote("http://\x7f/dataset", nil); err == nil {
		t.Fatal("OpenRemote accepted an unparseable base url")
	}
}

func TestOpenRemoteUsesTheClientItIsGiven(t *testing.T) {
	base, _ := stub(t)
	used := new(atomic.Bool)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		used.Store(true)
		return http.DefaultTransport.RoundTrip(r)
	})}
	if _, _, err := OpenRemote(base, client); err != nil {
		t.Fatalf("OpenRemote: %v", err)
	}
	// The caller's client carries its own timeout and transport; silently
	// substituting the default would drop both.
	if !used.Load() {
		t.Error("OpenRemote ignored the client it was given")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestARecordTooBigForTheCacheIsStillServed pins the behaviour at the budget
// ceiling: caching is an optimisation, so running out of it must cost a refetch
// and never an error. Built directly rather than through OpenRemote so the
// budget can be made small without making the constant configurable.
func TestARecordTooBigForTheCacheIsStillServed(t *testing.T) {
	base, hits := stub(t)
	fsys := &remoteFS{
		base:   base,
		client: &http.Client{Timeout: 5 * time.Second},
		cache:  map[string][]byte{},
		budget: 1, // smaller than any record
	}

	for i := range 2 {
		body, err := fs.ReadFile(fsys, "data/series/frieren.yaml")
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if !strings.Contains(string(body), "id: frieren") {
			t.Errorf("read %d returned %q", i, body)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("made %d requests, want 2 — the record exceeds the budget so neither read is cached", got)
	}
}
