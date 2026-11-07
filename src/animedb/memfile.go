package animedb

import (
	"bytes"
	"io/fs"
	"time"
)

// memFile is an fs.File over bytes already in hand. It is what a filesystem
// that fetches rather than opens has to return, since there is no descriptor
// behind it.
type memFile struct {
	name string
	size int64
	*bytes.Reader
}

func newMemFile(name string, body []byte) *memFile {
	return &memFile{name: name, size: int64(len(body)), Reader: bytes.NewReader(body)}
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memInfo{f.name, f.size}, nil }
func (f *memFile) Close() error               { return nil }

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() fs.FileMode  { return 0o444 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }
