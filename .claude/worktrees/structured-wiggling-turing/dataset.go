// Package animedb is the module root. It embeds the committed dataset so that
// binaries which serve it — the API server (cmd/api) and the Vercel function
// (api/) — carry the data with them and need no filesystem at runtime.
//
// The builder (cmd/builder, internal/builder) writes data/; the API
// (internal/api) reads this embedded copy. Keeping the embed at the module root
// is required because go:embed cannot reach files above a package directory.
package animedb

import "embed"

// DataFS holds the generated dataset under the "data/" prefix
// (data/series/*.yaml, data/staff/*.yaml).
//
//go:embed data
var DataFS embed.FS
