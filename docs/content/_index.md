---
title: Introduction
type: docs
---

# anime-metadata-db

An open dataset of anime **franchise / series / season / episode** metadata, plus
the `builder` CLI that compiles it and a read-only [Connect RPC](https://connectrpc.com)
API that serves it.

This site documents the project. Use the menu on the left to browse the guides.
The source lives on
[GitHub](https://github.com/michael-freling/anime-metadata-db).

## What's in the repository

- **The dataset** — anime metadata kept in two committed layers (see below).
- **`builder` CLI** (`cmd/builder`) — compiles the authored overrides plus open
  data into the resolved dataset under `data/`.
- **Connect RPC API** (`cmd/api`) — serves the embedded dataset read-only over the
  Connect protocol, gRPC and gRPC-Web, deployable free on Vercel.
- **This documentation site** (`docs/`) — built with [Hugo](https://gohugo.io) and
  the [hugo-book](https://github.com/alex-shpak/hugo-book) theme, published to
  GitHub Pages.

## The two layers

The dataset is kept in **two committed layers, in separate files**, so a rebuild
can never clobber authored work:

| Layer | Who writes it | Holds |
|---|---|---|
| `config/overrides/` | **you** (hand-edited) | Structure + decisions the open sources can't express: Series/Franchise boundaries, ordering, `alternateCutOf`, `WatchOrder`s, which series are linearly `numbered`. |
| `data/` | `builder build` (generated) | The full resolved records: overrides **+** facts filled from open data **+** computed `absoluteNumber`. Never hand-edit. |

`builder build` treats `overrides/` as read-only input, so builds are
**deterministic and idempotent**: the same overrides + the same pinned sources
produce the same `data/`.

## Inputs

Facts come from openly-licensed, redistributable sources (AniList is **not** used
— its ToS forbids redistribution):

- [`anime-offline-database`](https://github.com/manami-project/anime-offline-database) (ODbL) — titles, season/year, episode counts, cross-IDs.
- [`Anime-Lists/anime-lists`](https://github.com/Anime-Lists/anime-lists) — AniDB↔TVDB mapping and movie-set grouping.
- [Wikidata](https://www.wikidata.org) (CC0) — character & staff **names** (R2), resolved by QID.

## The API

The same dataset is served read-only over a Connect RPC service defined in
`proto/anime/v1/anime.proto`. Connect speaks the **Connect protocol, gRPC and
gRPC-Web over plain HTTP**, so clients can call it with an ordinary HTTP `POST` +
JSON. The dataset is compiled into the binary with `go:embed`, so the server is
stateless and self-contained, and it deploys free on Vercel's Go builder.

`AnimeService` exposes `ListFranchises`, `GetFranchise`, `GetSeries`, `Search` and
`GetHealth`. Titles are localized from the request's `Accept-Language` header.

## Where to go next

- [Getting Started]({{< relref "/docs/getting-started" >}}) — run this
  documentation site locally.
- [README on GitHub](https://github.com/michael-freling/anime-metadata-db#readme) —
  full CLI and API usage.
