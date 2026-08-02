---
title: Sources and licensing
weight: 5
---


Where every fact in the dataset comes from, under what terms it was taken, and
what that means for you if you use it. This page describes the project's
position; it is not legal advice.

## The short version

The code and the data are licensed **separately**:

| | Licence | File |
|---|---|---|
| Code — everything except `data/` | MIT | [`LICENSE`](https://github.com/michael-freling/anime-metadata-db/blob/main/LICENSE) |
| `data/` as a database | ODbL v1.0 | [`LICENSE-DATA`](https://github.com/michael-freling/anime-metadata-db/blob/main/LICENSE-DATA) |
| `data/` individual records | DbCL v1.0 | same file |

Attribution for the upstream sources lives in
[`NOTICE`](https://github.com/michael-freling/anime-metadata-db/blob/main/NOTICE).

## Where each field comes from

| Source | Licence | What it contributes |
|---|---|---|
| [anime-offline-database](https://github.com/manami-project/anime-offline-database) | **ODbL + DbCL** | titles, release year and season, episode counts, cross-provider ids |
| [Anime-Lists](https://github.com/Anime-Lists/anime-lists) | *none stated* | the AniDB→TVDB id mapping (`tvdbId`), movie-set grouping hints |
| [Wikidata](https://www.wikidata.org) | **CC0** | character and staff names, and the appearance / voice-actor graph |
| This project | ODbL (as published here) | franchise and series grouping, ids, season and part numbers, `absoluteNumber`, watch orders |

The last row is the part with no upstream at all. Deciding that Fate/Zero and
Fate/stay night are two series of one franchise, that Golden Kamuy's five
seasons carry a continuous episode numbering, or that Trigun Stargaze is season
two of Stampede — none of that is in any source. It is the project's editorial
work, and it is why the dataset exists.

## Why the data is under ODbL

`data/` is built from anime-offline-database, so it is a **derivative database**
of an ODbL source, and ODbL's share-alike term reaches derivative databases —
not just verbatim copies. The dataset is therefore offered on the same terms it
was received under. That is an obligation inherited from the source, not a
preference.

Two consequences worth being explicit about:

- **Sharing a modified dataset publicly means sharing it under ODbL too.** If
  you extend, correct or re-derive `data/` and make it publicly available, the
  result carries the same licence.
- **The terms travel with binaries.** `dataset.go` embeds `data/` with
  `go:embed`, so a compiled `cmd/api` binary — or a deployment of it — contains
  the database. It is not only the YAML files that are covered.

A *Produced Work* built from the data — a chart, an article, an app screen — is
treated differently by ODbL and needs attribution rather than share-alike. The
dataset itself, and an API that returns it, are not Produced Works.

## Anime-Lists has no stated licence

The project publishes no licence file. What the build takes from it is bare
identifier pairs — numbers linking one public database to another — which this
project treats as facts rather than protected expression, and which are widely
vendored on that basis. That reasoning is stated plainly rather than hidden
because it is the weakest link in the chain: if you are redistributing this
dataset in a context where that matters, reach your own view.

## Why AniList ids appear but AniList data does not

AniList's Terms of Service prohibit using its API as a backup or data storage
service and prohibit mass collection, so the build never calls AniList and no
AniList content is stored or served. There is no AniList endpoint in
[`config.yaml`](https://github.com/michael-freling/anime-metadata-db/blob/main/config.yaml).

The `anilistId` on every node is an identifier **parsed out of the
cross-reference URLs that anime-offline-database publishes** — the same array
that yields `anidbId`, `myAnimeListId` and `kitsuId`. It is a join key into the
ODbL source, not AniList data.

This is the point of the "facts, not expression" rule the dataset follows
throughout: the dataset stores ids, names and structure, and leaves synopses,
artwork, ratings and biographies to be fetched live at runtime from whichever
service the consumer has a right to use. See
[Using the dataset]({{< relref "/docs/using-the-dataset" >}}).

## If you redistribute this dataset

Keep [`NOTICE`](https://github.com/michael-freling/anime-metadata-db/blob/main/NOTICE)
intact, credit this project and the upstream sources, and offer any publicly
released derivative database under ODbL. If you only publish a Produced Work
built from the data, attribution is enough.
