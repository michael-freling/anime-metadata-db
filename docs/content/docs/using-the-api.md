---
title: Using the API
weight: 1
---


The dataset is served read-only by a [Connect RPC](https://connectrpc.com)
service. You can call it with an ordinary HTTP `POST` and a JSON body — no client
library or codegen required.

## Your first call

```sh
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/GetHealth \
  -H 'Content-Type: application/json' -d '{}'
```

```json
{"status":"ok","version":"<commit>","stats":{"franchises":1,"series":3,"seasons":9,"episodes":124}}
```

That's the whole contract: `POST` a JSON body to
`/{package}.{Service}/{Method}`, get JSON back.

## Endpoint

```
POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/{Method}
Content-Type: application/json
```

`https://anime-metadata-db.vercel.app` is the hosted public instance. To run your
own, start the server locally (`go run ./cmd/api`, see
[Building the dataset]({{< relref "/docs/building-the-dataset" >}})) and use
`http://localhost:8080` instead — the paths are identical.

## Methods

| Method | Body | Returns |
|---|---|---|
| `GetHealth` | `{}` | liveness, build version, dataset stats |
| `ListFranchises` | `{}` | every multi-series franchise, fully nested |
| `GetFranchise` | `{"id": "..."}` | one franchise with its nested series |
| `GetSeries` | `{"id": "..."}` | one series (standalone or under a franchise) |
| `Search` | `{"query": "...", "limit": 10}` | matching franchises and series |

`ListFranchises` returns only multi-series **franchises**; a standalone series is
reachable through `Search` and `GetSeries`. For `Search`, `limit` is optional
(defaults to 50) and a blank `query` matches nothing.

## Examples

Search — case-insensitive substring over titles:

```sh
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/Search \
  -H 'Content-Type: application/json' -d '{"query": "demon", "limit": 5}'
```

```json
{"results":[{"kind":"ENTRY_KIND_SERIES","id":"demon-slayer","title":"Demon Slayer"}]}
```

Fetch one series with its full structure — the `id` comes from a search result:

```sh
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/GetSeries \
  -H 'Content-Type: application/json' -d '{"id": "demon-slayer"}'
```

```json
{
  "series": {
    "id": "demon-slayer",
    "title": "Demon Slayer",
    "seasons": [
      {
        "id": "demon-slayer-s1",
        "number": 1,
        "releaseYear": 2019,
        "releaseSeason": "RELEASE_SEASON_SPRING",
        "externalIds": {"anilistId": 101922, "anidbId": 14107, "tvdbId": 348545},
        "episodes": [
          {"absoluteNumber": 1, "airedNumber": 1},
          {"absoluteNumber": 2, "airedNumber": 2}
        ]
      }
    ],
    "movies": [
      {"id": "demon-slayer-mugen-train-film", "title": "Mugen Train", "releaseYear": 2020}
    ]
  }
}
```

A `Franchise` nests `series`, which nests `seasons`, `movies` and `specials`,
each carrying `episodes`. Dates are `YYYY-MM-DD` strings; `externalIds`
cross-maps each node to AniList, AniDB, TMDB, TVDB and Wikidata. Field names in
the JSON are camelCase (`releaseYear`, `externalIds`, `absoluteNumber`). See
[Using the dataset]({{< relref "/docs/using-the-dataset" >}}) for the full model.

## Localized titles

Every title-bearing node returns a single `title` string, resolved from the
request's `Accept-Language` header (default `en`). For each title, resolution
tries, in order:

1. the requested language,
2. for a non-English request, the native `original`,
3. English,
4. the native `original`,
5. any available translation.

```sh
# Japanese titles where available, else the native original → "鬼滅の刃":
curl -X POST https://anime-metadata-db.vercel.app/anime.v1.AnimeService/GetSeries \
  -H 'Content-Type: application/json' -H 'Accept-Language: ja' \
  -d '{"id": "demon-slayer"}'
```

Send `Accept-Language: *` to additionally receive the full `localizedTitle` (the
native `original` plus every translation) on every node.

> **Scope.** The API serves the structural catalog — franchises, series,
> seasons, movies, specials and episodes. Characters and staff are present in the
> committed dataset but not yet exposed over the API.

---

**Next:** [Using the dataset]({{< relref "/docs/using-the-dataset" >}}) — the
same data as committed YAML, and the full model.
