---
title: Introduction
type: docs
---

# anime-metadata-db

An open dataset of anime **franchise → series → season → episode** metadata,
plus a `builder` CLI that compiles it and a read-only
[Connect RPC](https://connectrpc.com) API that serves it.

The dataset answers structural questions the big anime databases don't model
cleanly: which seasons and movies belong to one continuity, how a franchise's
series relate, the absolute episode ordering across a whole series, and curated
watch orders — all from openly-licensed, redistributable sources.

## The shape of the data

The catalog is a hierarchy:

- **Franchise** — a brand grouping related **Series** (e.g. *Fate*).
- **Series** — one continuity / storyline, the base unit. Holds **Seasons**
  (numbered TV installments), **Movies** and **Specials**.
- **Episode** — one episode, with its aired number and, where the series is
  linearly numbered, an absolute number spanning the whole series.

Each node carries titles in multiple languages and cross-IDs to AniList, AniDB,
TMDB, TVDB and Wikidata.

## Try it

The dataset is live on a hosted, read-only API. One `curl`, no setup:

```sh
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/GetHealth \
  -H 'Content-Type: application/json' -d '{}'
```

```json
{"status":"ok","version":"<commit>","stats":{"franchises":1,"series":3,"seasons":9,"episodes":124}}
```

## Start here

- **[Using the API]({{< relref "/docs/using-the-api" >}})** — query the hosted
  service with a plain HTTP `POST` + JSON. Start here if you just want the data.
- **[Using the dataset]({{< relref "/docs/using-the-dataset" >}})** — read the
  committed YAML directly, and understand the data model in full.
- **[Building the dataset]({{< relref "/docs/building-the-dataset" >}})** — run
  the `builder` CLI and author your own entries.

For the full source, see the
[repository on GitHub](https://github.com/michael-freling/anime-metadata-db).
