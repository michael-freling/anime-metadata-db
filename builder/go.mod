// The builder: fetches upstream sources and writes the committed dataset.
//
// Separate from api/ so that the server's dependencies stay minimal and the
// two halves cannot quietly grow a dependency on each other — the compiler
// enforces the seam that the import graph already had.
module github.com/michael-freling/anime-metadata-db/builder

go 1.25.0

require (
	github.com/michael-freling/anime-metadata-db v0.0.0
	github.com/spf13/cobra v1.10.2
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

replace github.com/michael-freling/anime-metadata-db => ../
