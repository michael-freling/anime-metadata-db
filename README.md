# anime-metadata-db

An open dataset of anime **franchise / series / season / episode** metadata, plus
the `builder` CLI that compiles it.

The data model and the builder design are documented under
[`docs/content/proposals/`](docs/content/proposals/) (rendered with Hugo). This
repository implements the **R1** model and the build tool from those proposals.

## The two layers

The dataset is kept in **two committed layers, in separate files**, so a rebuild
can never clobber authored work:

| Layer | Who writes it | Holds |
|---|---|---|
| [`overrides/`](overrides/) | **you** (hand-edited) | Structure + decisions the open sources can't express: Series/Franchise boundaries, ordering, `alternateCutOf`, `WatchOrder`s, which series are linearly `numbered`. |
| `data/` | `builder build` (generated) | The full resolved records: overrides **+** facts filled from open data **+** computed `absoluteNumber`. Never hand-edit. |

`builder build` treats `overrides/` as read-only input, so builds are
**deterministic and idempotent**: the same overrides + the same pinned sources
produce the same `data/`.

## Inputs

Facts come from openly-licensed, redistributable sources (AniList is **not** used
— its ToS forbids redistribution):

- [`anime-offline-database`](https://github.com/manami-project/anime-offline-database) (ODbL) — titles, season/year, episode counts, cross-IDs.
- [`Anime-Lists/anime-lists`](https://github.com/Anime-Lists/anime-lists) — AniDB↔TVDB mapping and movie-set grouping.

Sources are **not committed**; `builder init` downloads them into a gitignored
cache (`.sources/`) at the versions pinned in [`config.yaml`](config.yaml). A
source pinned to a rolling ref (`latest`/`master`) is re-pinned automatically
when it changes upstream; a source pinned to a fixed version fails the build on
a checksum mismatch (tamper detection). Use `builder refresh` to update all
pins deliberately.

## Usage

```sh
go build ./cmd/builder

./builder init                 # download the pinned sources into .sources/
./builder build                # (re)build data/ for all overrides
./builder build demon-slayer   # build/rebuild just one franchise or series
./builder refresh              # update sources to latest, bump pins, rebuild all
```

A new entry = create `overrides/series/<id>.yaml` and run `builder build`. Both
standalone Series and multi-storyline Franchises live together under
`overrides/series/` (the builder mirrors that layout into `data/series/`), so a
file's `series:` or `franchise:` key — not its directory — determines its kind.
The build fails on any unknown id, dangling reference, or schema violation, so a
successful build is always a valid dataset. Where it makes a low-confidence guess
(chiefly title-language tagging) it prints a report; pin those cases with an
override. Auto-filled titles default to Japanese (`ja` + romanized `ja-Latn`).

## Development

```sh
go test ./...                                          # unit tests (no network)
golangci-lint run ./...                                # lint (golangci-lint v2)
go test -coverpkg=./... -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
go test -tags e2e -run E2E ./...                       # e2e: downloads the real sources, no mocks
```

CI runs golangci-lint v2 and the test suite with a > 95% coverage gate
(`.github/workflows/go.yml`). The build-tagged e2e tests download the live
open-data sources and run on every PR (`.github/workflows/e2e.yml`), guarding
against upstream source/URL drift.
