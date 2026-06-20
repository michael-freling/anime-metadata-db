---
title: "Build CLI Design (Go)"
date: 2026-06-19
weight: 4
---

# Build CLI Design (Go)

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)
**Related:** [Anime Series Data Model](../data-model-anime-series/) ·
[Characters & Staff Data Model](../data-model-characters-staff/) — the schema this tool produces.

A Go **CLI** that builds the anime-metadata database from open-data sources plus our curation
overrides. It assembles the **R1** ordering/grouping model and the **storable R2 facts**
(ids, names, the character/voice-actor graph) into one deterministic artifact. It deliberately does
**not** fetch or store "expression" (synopses, art, episode stills) — per the research note's
facts-vs-expression rule (§5.1a), the *app* fetches those live at display time.

> **TL;DR on the CLI-vs-API question (Part 6):** build with a **CLI**, keep curation in **YAML files
> in git**, and let the app read the built DB directly. A write/management **API is not needed now**.

---

## Part 1 — What it builds, and where it runs

- **Output:** one portable database artifact (SQLite recommended — Part 5) holding the resolved
  `Franchise → Series → Season → Episode` tree plus `Movie`, `Special`, `WatchOrder`, and the
  `Character` / `Staff` graph from the data-model docs — **facts only**.
- **Shape:** a **batch CLI**, not a long-running service. It runs locally, in CI, or on a schedule
  (refresh sources → rebuild → review diff → commit). The build is **deterministic and
  reproducible**: same pinned inputs + same overrides ⇒ same DB.

---

## Part 2 — Inputs

| Input | Source | Used for | Licensing / notes |
|---|---|---|---|
| `anime-offline-database.json` | manami-project (download) | Franchise/Series clustering; cross-source ID map | ODbL — **storable** (research note §5.1a) |
| `anime-list.xml` | Anime-Lists/anime-lists | Episode **absolute ordering** (season offsets) | open XML — storable facts (numbers) |
| `anime-movieset-list.xml` | ScudLee/anime-lists | **Movie** grouping into sets | open XML |
| AniList GraphQL | anilist.co (keyless) | Storable **facts**: ids, names, releaseDate/season, the character + voice-actor graph | ToS: store facts only — **never warehouse expression** |
| `overrides/*.yaml` | hand-authored, in git | Franchise/Series boundaries, ordering mode, alt-cut, `WatchOrder`s, identity merges | our curation — **overrides win** |
| `config.yaml` | repo | Source URLs + pinned versions, output target, build scope | — |

The first three are **vendored** (downloaded once, version-pinned, committed or cached) so builds are
reproducible and offline-capable. AniList is queried for facts; results are cached to respect rate
limits. `overrides/` is the layer we own — everything a source gets wrong or can't express.

---

## Part 3 — Commands

```
animedb fetch              # download / refresh vendored sources to the pinned versions
animedb build              # run the full pipeline → write the DB artifact
animedb validate           # check overrides + resolved model (schema, referential integrity)
animedb diff               # show what changed vs the current DB (review before committing)
animedb inspect <id>       # print a resolved Franchise / Series / Character (debug)
```

| Command | What it does |
|---|---|
| `fetch` | Pulls source files to `config.yaml` pins; updates a lockfile of checksums |
| `build` | The pipeline (Part 4); `--scope franchise=fate` to build a subset, `--out db.sqlite` |
| `validate` | Fails CI on dangling refs, unknown override targets, or schema violations |
| `diff` | Prints added/changed/removed records vs the previous DB — the curation review surface |
| `inspect` | Renders one resolved record as YAML/JSON for debugging |

Curation is **editing `overrides/*.yaml` + `validate` + `build` + `diff`** — no bespoke admin tool.

---

## Part 4 — The build pipeline

Implements research note §5.3 and the "Building the records" sections of the data-model docs:

1. **Fetch / load** the vendored sources (pinned) and the AniList cache.
2. **Seed** membership from `anime-offline-database` clustering → `Franchise`/`Series` grouping and
   the media nodes (`Season`/`Movie`/`Special`) bucketed by AniList `format`.
3. **Order** each linear Series: assign `absoluteNumber` from `anime-list.xml` offsets; slot original
   movies by `releaseDate`; mark alternate cuts (`alternateCutOf`, no number). Non-linear Series get
   no numbers (release-date order).
4. **Apply overrides** (`overrides/*.yaml`): Series/Franchise boundaries, ordering decisions, alt-cut
   vs original, `seasonNumber`/`part`, `WatchOrder`s, and `Character`/`Staff` identity merges.
   **Overrides always win.**
5. **Resolve facts** from AniList: titles `{ original, translations }`, `releaseYear`/`releaseSeason`,
   and the character + voice-actor graph (appearances, default VAs) — **facts only**.
6. **Validate** referential integrity (every edge resolves), schema, and override targets.
7. **Store** the resolved records to the artifact (Part 5), deterministically.

> Expression (synopses, cover art, stills, bios) is **out of scope** here — the app fetches it live
> from AniList/TMDB keyed by the `externalIds` this tool stores.

---

## Part 5 — Storage / output

**Recommendation: SQLite.** It's an embeddable, portable, read-mostly artifact — ideal for a metadata
DB the app can ship with and query directly, and a single file is trivial to diff, cache, and
distribute. The schema mirrors the data-model docs (tables for `franchise`, `series`, `season`,
`episode`, `movie`, `special`, `watch_order`, `character`, `character_appearance`, `staff`, and the
`character_voice_actor` link). Go can use a pure-Go driver (`modernc.org/sqlite`) so builds need no
cgo.

Alternatives, if needs change: **Postgres** (only if we need a hosted, multi-writer managed service),
or **flat JSON/YAML** committed to git (most diff-friendly, but weak for queries at scale). Start with
SQLite; revisit if a hosted service appears.

---

## Part 6 — CLI, or an API too?

| Concern | Recommended interface | Why |
|---|---|---|
| **Build / ETL** | **CLI** (`animedb build`) | Batch, reproducible, runs in CI/cron; output is a static artifact — no service to operate |
| **Curation / data management** | **YAML in git + `validate`/`diff`** | Overrides are files → review, history, rollback, and blame come free; `diff` is the review surface |
| **Serving to the app** | App reads the built DB; fetches expression live | Read-mostly; ship the SQLite file or expose a thin **read-only** API only if several consumers need it |
| **Admin write API / UI** | **Deferred** | Premature until curation volume outgrows hand-edited YAML |

**So: CLI-first.** A management/write API is **not required** — git + YAML + the CLI cover authoring,
review, and audit. Reach for an API only when (a) multiple independent services need to read the data
(a read-only query API), or (b) curation grows enough to want a web admin UI over the overrides (then
the API wraps the same override + build pipeline, it doesn't replace it).

---

## Part 7 — Go package layout (sketch)

```
cmd/animedb/                 # cobra entrypoint: fetch / build / validate / diff / inspect
internal/
  config/                    # config.yaml + source pins + lockfile
  sources/
    offlinedb/               # anime-offline-database loader
    animelists/              # anime-list.xml + anime-movieset-list.xml parsers
    anilist/                 # GraphQL client (facts only) + on-disk cache + rate limiter
  overrides/                 # YAML override loader + schema
  model/                     # the entities from the data-model docs
  resolve/                   # seed → order → apply overrides → resolve facts
  validate/                  # referential integrity + schema checks
  store/sqlite/              # schema + writer (modernc.org/sqlite)
```

Suggested libraries: **cobra** (commands), **koanf**/**viper** (config), **modernc.org/sqlite**
(cgo-free), a GraphQL client for AniList. Source adapters share a small interface so a source can be
swapped or pinned independently.

---

## Part 8 — Open questions

- **Store choice** — SQLite is the default (Part 5); revisit if a hosted multi-writer service is ever
  needed (then Postgres).
- **Incremental vs full rebuild** — start with deterministic **full** rebuilds (simple, reproducible);
  add incremental only if build time becomes a problem.
- **AniList fact-fetch budget** — keyless ~90 req/min; the `anilist` cache + a rate limiter keep builds
  polite. How aggressively to pre-warm vs fetch lazily?
- **Override authoring at scale** — hand-edited YAML is fine now; a future web admin UI would wrap the
  same `overrides` + `build` pipeline behind an API (Part 6), not replace it.
- **Source pinning & drift** — `fetch` records checksums; how often to bump pins, and how to surface
  upstream schema changes in CI.
