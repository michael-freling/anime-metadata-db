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
// (data/series/*.yaml, data/staff/*.yaml). The API reads a record out of it
// only when a request names a single id.
//
//go:embed data
var DataFS embed.FS

// Index is the generated listing index (see internal/index). Embedding it as a
// string rather than reading it out of DataFS is deliberate: a string constant
// lives in the binary's read-only data, so the server holds the whole index
// without allocating a byte of heap for it, and boots without parsing YAML.
//
//go:embed data/index.tsv
var Index string
