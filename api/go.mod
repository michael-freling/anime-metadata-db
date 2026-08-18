// The read-only Connect API over the committed dataset, and the generator for
// the listing index it serves from.
module github.com/michael-freling/anime-metadata-db/api

go 1.25.0

require (
	connectrpc.com/connect v1.20.0
	github.com/michael-freling/anime-metadata-db v0.0.0
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v3 v3.0.1
)

// The dataset lives beside this module in the same repository; it is never
// consumed from a proxy, so it is always resolved from the working tree.
replace github.com/michael-freling/anime-metadata-db => ../
