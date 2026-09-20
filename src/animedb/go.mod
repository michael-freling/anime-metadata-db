// The dataset as Go sees it: the model that describes a record's shape, and the
// loader that opens a committed dataset directory.
//
// Shared by src/api and src/builder, which are separate modules, so neither
// imposes its dependencies on the other. Deliberately dependency-free beyond
// the YAML decoder the records are written in.
//
// `model` is a public package rather than an internal one because it crosses a
// module boundary: Go scopes `internal/` to the tree under its parent, and
// src/api and src/builder sit outside this module's tree.
module github.com/michael-freling/anime-metadata-db/src/animedb

go 1.25.0

require gopkg.in/yaml.v3 v3.0.1
