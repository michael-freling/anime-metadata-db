// Package animedb is the dataset as Go sees it: the model describing a record's
// shape (subpackage model) and the loader that opens a committed dataset
// directory.
//
// The dataset is read from disk rather than compiled in. It used to be embedded
// with go:embed so a binary carried the data with it and needed no filesystem
// at runtime; that stops being reasonable as the catalogue grows, because every
// cold start then ships the whole dataset inside the executable. Reading it
// means the deployment has to put dataset/ beside the binary — see
// .vercelignore, which keeps it in the upload for exactly this reason.
package animedb

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// IndexPath is the listing index's location within a dataset directory.
//
// It is spelled relative to the dataset root, matching the "data/" prefix the
// index itself stores against every record, so Open's filesystem and the paths
// read out of the index agree without either having to strip a prefix.
const IndexPath = "data/index.tsv"

// Open returns the record filesystem and the listing index for the dataset
// rooted at dir.
//
// dir is the directory holding data/ — the repository's dataset/ directory, or
// any copy of it. The returned fs.FS is rooted there, so a caller reads
// "data/series/<file>.yaml" exactly as the index names it.
//
// Only the index is read here. Record files are opened when a request asks for
// one, so this returns in the time it takes to read a single file however large
// the catalogue grows.
func Open(dir string) (fs.FS, string, error) {
	index, err := os.ReadFile(filepath.Join(dir, IndexPath))
	if err != nil {
		return nil, "", fmt.Errorf("read dataset index: %w", err)
	}
	return os.DirFS(dir), string(index), nil
}
