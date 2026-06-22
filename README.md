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
| [`config/overrides/`](config/overrides/) | **you** (hand-edited) | Structure + decisions the open sources can't express: Series/Franchise boundaries, ordering, `alternateCutOf`, `WatchOrder`s, which series are linearly `numbered`. |
| `data/` | `builder build` (generated) | The full resolved records: overrides **+** facts filled from open data **+** computed `absoluteNumber`. Never hand-edit. |

`builder build` treats `overrides/` as read-only input, so builds are
**deterministic and idempotent**: the same overrides + the same pinned sources
produce the same `data/`.

## Inputs

Facts come from openly-licensed, redistributable sources (AniList is **not** used
— its ToS forbids redistribution):

- [`anime-offline-database`](https://github.com/manami-project/anime-offline-database) (ODbL) — titles, season/year, episode counts, cross-IDs.
- [`Anime-Lists/anime-lists`](https://github.com/Anime-Lists/anime-lists) — AniDB↔TVDB mapping and movie-set grouping.
- [Wikidata](https://www.wikidata.org) (CC0) — character & staff **names** (R2), resolved by QID via the wbgetentities API.

## Characters & staff (R2)

A series' **cast** is co-located with it: the `characters:` list is **nested
under** the `franchise:`/`series:` in the same `config/overrides/series/<id>.yaml`
file as the structure (most characters belong to one series; a cross-franchise
character lives in its home file and its `appearances` reference the other series
by id). **Staff** (voice actors) are global and grouped by language —
`config/overrides/staff/japanese-voice-actors.yaml`, etc.

You author the graph — who appears in which series (`appearances` → `seriesId`,
optionally `scope`d to a season/movie/special), the voice-actor links
(`voiceActors` → `staffId`), and each node's Wikidata `QID`. The builder fills
**names** from Wikidata and validates every reference against the R1 ids,
nesting the cast into `data/series/<id>.yaml` and writing staff to `data/staff/`.

Only **facts** are stored (ids, names, the appearance + voice-actor graph). The
build never touches AniList/MAL; a consumer fetches *expression* (roles, bios,
images) live at runtime using the stored ids, storing nothing.

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

A new entry = create `config/overrides/series/<id>.yaml` and run `builder build`.
Both standalone Series and multi-storyline Franchises live together under
`config/overrides/series/` (the builder mirrors that layout into `data/series/`),
so a
file's `series:` or `franchise:` key — not its directory — determines its kind.
The build fails on any unknown id, dangling reference, or schema violation, so a
successful build is always a valid dataset. Where it makes a low-confidence guess
(chiefly title-language tagging) it prints a report; pin those cases with an
override. Auto-filled titles default to Japanese (`ja` + romanized `ja-Latn`).

## API

The same dataset is served read-only over a [Connect RPC](https://connectrpc.com)
service defined in [`proto/anime/v1/anime.proto`](proto/anime/v1/anime.proto).
Connect speaks the **Connect protocol, gRPC and gRPC-Web over plain HTTP**, so
clients can call it with an ordinary HTTP `POST` + JSON, no special tooling
required. The dataset is compiled into the binary with `go:embed`, so the server
is stateless and self-contained.

The code is split to keep the two concerns explicit: `internal/builder` (+
`cmd/builder`) **writes** `data/`; `internal/api` (+ `cmd/api`) **reads** the
embedded copy and serves it. `internal/model` is the shared data model.

`AnimeService` exposes: `ListFranchises`, `GetFranchise`, `GetSeries`, `Search`
and `GetHealth`. Run it locally:

```sh
go run ./cmd/api                 # listens on :8080 (HTTP/1.1 + cleartext HTTP/2)

curl -X POST localhost:8080/anime.v1.AnimeService/GetHealth \
  -H 'Content-Type: application/json' -d '{}'
curl -X POST localhost:8080/anime.v1.AnimeService/Search \
  -H 'Content-Type: application/json' -d '{"query":"demon"}'
```

### Hosting (Vercel)

The service deploys to Vercel's free tier as a single Go serverless function:
[`api/index.go`](api/index.go) wraps the same `http.Handler`, and
[`vercel.json`](vercel.json) rewrites every route to it. Connect-protocol,
gRPC-Web and JSON clients all work over Vercel's HTTP/1.1; full gRPC (HTTP/2
streaming) is available only from `cmd/api`. Deploy with `vercel deploy` (or
connect the repo in the Vercel dashboard) — no configuration beyond the
committed files is needed.

### Regenerating the protobuf code

The generated Go under `gen/` is committed (and excluded from the coverage
gate). Regenerate it with [buf](https://buf.build) after editing the `.proto`:

```sh
make generate                    # buf generate (needs buf + protoc-gen-go + protoc-gen-connect-go)
```

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
