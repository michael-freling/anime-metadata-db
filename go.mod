// The dataset itself: the committed records under data/, the generated listing
// index, and the model that describes their shape.
//
// It is deliberately dependency-free and holds no programs. api/ and builder/
// are separate modules that depend on this one, so neither imposes its
// dependencies on a consumer that only wants the data.
module github.com/michael-freling/anime-metadata-db

go 1.25.0

require gopkg.in/yaml.v3 v3.0.1
