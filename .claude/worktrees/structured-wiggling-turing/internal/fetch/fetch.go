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
)

// maxSourceBytes caps a downloaded source to guard against a runaway response
// (the real sources are a few tens of MB).
const maxSourceBytes = 512 << 20 // 512 MiB

// userAgent identifies the client. Wikimedia APIs reject requests without a
// descriptive User-Agent, so we always send one.
const userAgent = "anime-metadata-db builder (+https://github.com/michael-freling/anime-metadata-db)"

// Client downloads sources over HTTP.
type Client struct {
	HTTP *http.Client
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
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // response body, read-only
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get %s: unexpected status %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return body, nil
}

// Checksum returns the lowercase hex SHA-256 of data.
func Checksum(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
