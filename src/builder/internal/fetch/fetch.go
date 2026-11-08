// Package fetch downloads pinned open-data sources over HTTP and computes their
// checksums. It is the only package that touches the network.
package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxSourceBytes caps a downloaded source to guard against a runaway response
// (the real sources are a few tens of MB).
const maxSourceBytes = 512 << 20 // 512 MiB

// userAgent identifies the client. Wikimedia APIs reject requests without a
// descriptive User-Agent, so we always send one.
const userAgent = "anime-metadata-db builder (+https://github.com/michael-freling/anime-metadata-db)"

// maxAttempts bounds how many times one URL is tried. Four is enough to ride
// out a short burst limit without turning a genuine outage into a job that
// hangs for minutes before failing anyway.
const maxAttempts = 4

// retryBase is the first backoff interval; each further attempt doubles it.
const retryBase = time.Second

// Client downloads sources over HTTP.
type Client struct {
	HTTP *http.Client
	// Sleep waits between attempts. Nil means time.Sleep; a test substitutes
	// a recorder so the backoff can be asserted without the wall clock.
	Sleep func(time.Duration)
}

// NewClient returns a Client using the given http.Client, or http.DefaultClient
// when nil.
func NewClient(h *http.Client) *Client {
	if h == nil {
		h = http.DefaultClient
	}
	return &Client{HTTP: h}
}

// Get downloads url and returns its body, failing on a non-2xx status.
//
// A 429 or 503 is retried with backoff rather than failed outright, honouring
// Retry-After when the server sends one. That is the protocol's own answer to
// being told to slow down, and Wikimedia's API asks for it directly: the build
// resolves series titles fifty per request, so a catalogue of 6,382 series
// makes about 128 requests back to back and gets 429ed partway through. At
// 1,075 series it made 22 and never did — the failure arrived with the data,
// not with the code.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var last refusal
	for attempt := 1; ; attempt++ {
		body, ref, err := c.get(ctx, url)
		if err != nil {
			return nil, err
		}
		if ref == nil {
			return body, nil
		}
		last = *ref
		wait := last.backoff(attempt)
		if wait <= 0 || attempt == maxAttempts {
			break
		}
		if err := c.wait(ctx, wait); err != nil {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: unexpected status %s (gave up after %d attempts)",
		url, last.status, maxAttempts)
}

// refusal is a non-2xx response: what the server said, and how long it asked
// to be left alone.
type refusal struct {
	code       int
	status     string
	retryAfter time.Duration
}

// backoff returns how long to wait before the next attempt, or 0 when this
// refusal is not worth retrying.
//
// Only 429 and 503 are retried: both mean "not now". Everything else — a 404
// on a moved release asset, a 403 — means "not ever", and is exactly the
// upstream drift these jobs exist to surface, so retrying would turn a clear
// failure into a slow one.
func (r refusal) backoff(attempt int) time.Duration {
	if r.code != http.StatusTooManyRequests && r.code != http.StatusServiceUnavailable {
		return 0
	}
	// The server's own number wins where it gave one; it knows its window.
	if r.retryAfter > 0 {
		return r.retryAfter
	}
	return retryBase << (attempt - 1)
}

// get performs one attempt. A nil refusal means success and body is the answer.
func (c *Client) get(ctx context.Context, url string) ([]byte, *refusal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body, read-only
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &refusal{
			code:       resp.StatusCode,
			status:     resp.Status,
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil, nil
}

// parseRetryAfter reads the header's delay-seconds form, which is what
// Wikimedia sends. The HTTP-date form is ignored rather than half-supported:
// falling back to the backoff below is a safe answer, and a wrong date parse
// would not be.
func parseRetryAfter(v string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || secs <= 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// httpClient is the transport to use. Defaulted here as well as in NewClient so
// a zero Client works, the same way a nil Sleep does — a struct literal is the
// natural way to set one field and should not be a nil dereference.
func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// wait sleeps for d unless the context ends first.
func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		c.Sleep(d)
		return ctx.Err()
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Checksum returns the lowercase hex SHA-256 of data.
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
