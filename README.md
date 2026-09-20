# anime-metadata-db

An open dataset of anime **franchise / series / season / episode** metadata, plus
the `builder` CLI that compiles it.

The data model and the builder design are documented under
[`src/web/content/docs/development/proposals/`](src/web/content/docs/development/proposals/).
Those are internal design notes — read them here in the repository; the
published site deliberately leaves them out. This repository implements the
**R1** model and the build tool from those proposals.

## The two layers

The dataset is kept in **two committed layers, in separate files**, so a rebuild
can never clobber authored work:

| Layer | Who writes it | Holds |
|---|---|---|
| [`dataset/overrides/`](dataset/overrides/) | **you** (hand-edited) | Structure + decisions the open sources can't express: Series/Franchise boundaries, ordering, `alternateCutOf`, `WatchOrder`s, which series are linearly `numbered`. |
| `dataset/data/` | `builder build` (generated) | The full resolved records: overrides **+** facts filled from open data **+** computed `absoluteNumber`. Never hand-edit. |

`builder build` treats `dataset/overrides/` as read-only input, so builds are
**deterministic and idempotent**: the same overrides + the same pinned sources
produce the same `dataset/data/`.

## Inputs

Facts come from open data sources, each on its own terms — listed below and set
out in full in
[Sources and licensing](https://anime-metadata-db.vercel.app/docs/sources-and-licensing).
AniList is **not** used: its ToS forbids storing or redistributing its content.

- [`anime-offline-database`](https://github.com/manami-project/anime-offline-database) (ODbL + DbCL) — titles, season/year, episode counts, cross-IDs.
- [`Anime-Lists/anime-lists`](https://github.com/Anime-Lists/anime-lists) (no stated licence; bare ID pairs) — AniDB↔TVDB mapping and movie-set grouping.
- [Wikidata](https://www.wikidata.org) (CC0) — character & staff **names** (R2) by authored QID, and a series' English **title** via the work its native title resolves to.

The `anilistId` on each node is an identifier parsed out of the cross-reference
URLs the offline database publishes — a join key into that ODbL source, not
AniList data. It is **resolved, not authored**: the build finds the upstream
entries carrying the series' own native title, orders them by airing date, and
fills each installment's id. Where an override does name one, the resolution
still runs and reports a disagreement.

Every build prints the split, so the hand-authored surface is never a guess:

```
anilistId provenance: 699 ids
  492/699 resolved from the series' own title
  207/699 read from an override
```

The remaining 207 sit in 76 series that upstream does not list under their own
title, so nothing can compute them. That number is meant to fall, and CI checks
it against the overrides so it cannot drift upward unnoticed. See
[`NOTICE`](NOTICE) and the
[Sources and licensing](https://anime-metadata-db.vercel.app/docs/sources-and-licensing)
guide.

## Characters & staff (R2)

A series' **cast** is co-located with it: the `characters:` list is **nested
under** the `franchise:`/`series:` in the same `dataset/overrides/series/<id>.yaml`
file as the structure (most characters belong to one series; a cross-franchise
character lives in its home file and its `appearances` reference the other series
by id). **Staff** (voice actors) are global and grouped by language —
`dataset/overrides/staff/japanese-voice-actors.yaml`, etc.

You author the graph — who appears in which series (`appearances` → `seriesId`,
optionally `scope`d to a season/movie/special), the voice-actor links
(`voiceActors` → `staffId`), and each node's Wikidata `QID`. The builder fills
**names** from Wikidata and validates every reference against the R1 ids,
nesting the cast into `dataset/data/series/<id>.yaml` and writing staff to `dataset/data/staff/`.

Only **facts** are stored (ids, names, the appearance + voice-actor graph). The
build never touches AniList/MAL; a consumer fetches *expression* (roles, bios,
images) live at runtime using the stored ids, storing nothing.

Sources are **not committed**; `builder init` downloads them into a gitignored
cache (`.sources/`) at the versions pinned in [`src/builder/config.yaml`](src/builder/config.yaml). A
source pinned to a rolling ref (`latest`/`master`) is re-pinned automatically
when it changes upstream; a source pinned to a fixed version fails the build on
a checksum mismatch (tamper detection). Use `builder refresh` to update all
pins deliberately.

## Usage

```sh
cd src/builder && go build ./cmd/builder   # or: make build-data

./builder init                 # download the pinned sources into .sources/
./builder build                # (re)build dataset/data/ for all overrides
./builder build demon-slayer   # build/rebuild just one franchise or series
./builder refresh              # update sources to latest, bump pins, rebuild all
```

A new entry = create `dataset/overrides/series/<id>.yaml` and run `builder build`.
Both standalone Series and multi-storyline Franchises live together under
`dataset/overrides/series/` (the builder mirrors that layout into `dataset/data/series/`),
so a
file's `series:` or `franchise:` key — not its directory — determines its kind.
The build fails on any unknown id, dangling reference, or schema violation, so a
successful build is always a valid dataset. Where it makes a low-confidence guess
(chiefly title-language tagging) it prints a report; pin those cases with an
override. Auto-filled titles default to Japanese (`ja` + romanized `ja-Latn`).

## API

The same dataset is served read-only over a [Connect RPC](https://connectrpc.com)
service defined in [`src/api/proto/anime/v1/anime.proto`](src/api/proto/anime/v1/anime.proto).
Connect speaks the **Connect protocol, gRPC and gRPC-Web over plain HTTP**, so
clients can call it with an ordinary HTTP `POST` + JSON, no special tooling
required. The server reads `dataset/` from the filesystem at startup, so a
deployment ships the directory beside the binary; `-dataset` or
`ANIME_DATASET_DIR` names it, and otherwise it is found by walking up from the
working directory and from the executable.

Startup reads exactly one file. Listings are answered from
`dataset/data/index.tsv`, a generated file carrying just the fields a browse row
shows, and a YAML record is opened only when a request names a single id — so
boot stays flat as the catalogue grows however large `dataset/data/` gets; see
[`src/api/internal/index`](src/api/internal/index/index.go) for the measurements
behind it. Regenerate it with `make index` after any change under `dataset/data/`.

The code is three Go modules, so the two programs cannot quietly grow a
dependency on each other and neither imposes its dependencies on someone who
only wants the data:

| Module | Holds | Depends on |
|---|---|---|
| `src/animedb/` | `model` and the dataset loader — the types describing the records, and how to open them | nothing but a YAML parser |
| `src/api/` | `cmd/api`, `cmd/index`, `internal/api`, `internal/index`, the proto | `src/animedb` |
| `src/builder/` | `cmd/builder`, `internal/build`, the sources and `config/` | `src/animedb` |

There is no module at the repository root: the data it used to hold is plain
files under `dataset/`, which nothing needs to compile.

A full build owns `dataset/data/`: records whose override was deleted are pruned, so
the tree never keeps a stale one. That also makes a wrong `overridesDir` the
most destructive mistake available here, so the builder refuses to run when the
overrides resolve to nothing and the prune would empty the dataset. CI checks
the committed configuration both ways — a fast path-resolution test in the
normal run, and a job that rebuilds `dataset/data/` from the real sources and diffs it.

`builder` **writes** `dataset/data/`; `src/api/cmd/index` **indexes** it;
`src/api/cmd/api` **reads** it from disk and serves it. `src/api` and
`src/builder` resolve the shared module with a `replace` pointing at their
sibling, so no `go.work` is needed and every module builds standalone — which is
exactly how CI builds them.

The public API is `anime.v1.AnimeService`, and it is three methods wide:
`SearchSeries` finds series by name, `SearchReleases` finds seasons, films and
specials by when they premiered, and `GetSeries` returns one series **whole** —
every installment, every episode and the entire cast in a single response, with
nothing capped and nothing to page. Run it locally:

```sh
cd src/api && go run ./cmd/api                 # listens on :8080 (HTTP/1.1 + cleartext HTTP/2)

curl -X POST localhost:8080/anime.v1.AnimeService/SearchSeries \
  -H 'Content-Type: application/json' -d '{"query":"demon"}'
curl -X POST localhost:8080/anime.v1.AnimeService/SearchReleases \
  -H 'Content-Type: application/json' -d '{"releaseYear":2026,"releaseSeason":"WINTER"}'
curl -X POST localhost:8080/anime.v1.AnimeService/GetSeries \
  -H 'Content-Type: application/json' -d '{"id":"demon-slayer"}'
```

A second service, `browse.v1.BrowseService`, serves
[anime-metadata-web](https://anime-metadata-web.vercel.app) from the same binary:
the franchise grouping, flat catalog rows and the global character and staff
indexes that a browse UI needs. It is deliberately undocumented and carries no
compatibility promise — it changes whenever that site does. It is excluded from
the published API reference, which is why that reference lists three methods.

**Cast.** Characters and staff are global, so the character embedded in a
series' `characters` is the full node, carrying every series it appears in. A
character's `voiceActors` is the cast that holds throughout; an `appearance` may
add to it and `scope` that to specific seasons, movies or specials.

**Localized titles.** Each node returns a single `title` resolved from the
request's `Accept-Language` header (default `en`); resolution falls back
requested-language → native original (for non-English) → English → any. Send
`Accept-Language: *` to additionally receive the full multilingual
`localized_title` (native `original` + all `translations`) on every node.

```sh
# Japanese titles where available, else the native original:
curl -X POST localhost:8080/anime.v1.AnimeService/GetSeries \
  -H 'Content-Type: application/json' -H 'Accept-Language: ja' -d '{"id":"demon-slayer"}'
# Every language on every node:
curl -X POST localhost:8080/anime.v1.AnimeService/SearchSeries \
  -H 'Content-Type: application/json' -H 'Accept-Language: *' -d '{"limit":5}'
```

### Hosting (Vercel)

The service deploys to Vercel's free tier using Vercel's native **Go web-server**
builder: it compiles [`src/api/cmd/api`](src/api/cmd/api) and runs it as a
server, injecting the listen port via `$PORT` (which the server binds
automatically). No `vercel.json` or serverless-function wrapper is needed —
every request is proxied to the server. `.vercelignore` keeps `dataset/` in the
upload, which the server now needs at runtime rather than only at build time. The browse service's `GetStats` reports `$VERCEL_GIT_COMMIT_SHA` as its
`version`.

Connect-protocol, gRPC-Web and JSON clients all work over Vercel; deploy by
connecting the repo in the Vercel dashboard (pushes to the production branch
auto-deploy) or with `vercel deploy`. No configuration beyond the committed
files is needed.

### Regenerating the protobuf code

The generated Go under `src/api/internal/gen/` is committed (and excluded from the
coverage gate). Regenerate it with [buf](https://buf.build) after editing the
`.proto`:

```sh
make generate                    # buf generate (needs buf + protoc-gen-go + protoc-gen-connect-go)
```

## Development

Run these from inside a module — `src/animedb`, `src/api` or `src/builder` —
since the repository root is not one:

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

## Licensing

The code and the data are licensed **separately**:

| | Licence | |
|---|---|---|
| Code — everything except `dataset/` | MIT | [`LICENSE`](LICENSE) |
| `dataset/` as a database | ODbL v1.0 | [`LICENSE-DATA`](LICENSE-DATA) |
| `dataset/` individual records | DbCL v1.0 | [`LICENSE-DATA`](LICENSE-DATA) |

The whole of `dataset/` is covered, not just the generated half:
`dataset/overrides/` is the authored side of the same database and much of what
it holds is derived from the same ODbL source, so splitting the licence between
the two would put those records on the code side of a line they do not belong on.

`dataset/` is a derivative database of `anime-offline-database`, and the ODbL's
share-alike term reaches derivative databases — so the compiled dataset is
offered on the terms it was received under. Publishing a modified dataset means
publishing it under ODbL too, and a deployment carries `dataset/` alongside the
binary, so those terms travel with any copy of the files.
Attribution for every upstream source is in [`NOTICE`](NOTICE); the full picture
is in the
[Sources and licensing](https://anime-metadata-db.vercel.app/docs/sources-and-licensing)
guide.
