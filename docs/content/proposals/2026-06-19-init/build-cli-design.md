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

A Go CLI named **`builder`** that builds the anime-metadata dataset from **redistribution-permissive**
open sources plus our curation, and writes it as **YAML files committed to this GitHub repo** — so the
dataset is itself **open data** anyone can browse, diff, fork, and download.

Because we redistribute the output, the builder may only consume sources whose licenses **allow
storage + redistribution** — chiefly **anime-offline-database** (ODbL) and the open **anime-lists**
XML. It does **not** use the AniList API (see the box below).

> **TL;DR**
> - Two commands: **`builder init`** pulls the open-data sources locally (not committed); **`builder
>   build`** turns sources + curation into `data/*.yaml`.
> - **GitHub is the database, diff, history, and management layer** — no `validate`/`diff`/`inspect`
>   subcommand and **no management API**.
> - **No AniList.** Its ToS forbids warehousing/redistribution, which is incompatible with shipping an
>   open dataset (see Part 2).

---

## Part 1 — What it builds, and where it runs

- **Output:** **YAML files committed to the repo** (Part 5) holding the resolved
  `Franchise → Series → Season → Episode` tree plus `Movie`, `Special`, and `WatchOrder` — assembled
  from anime-offline-database facts (titles, season/year, episode counts) + the open ordering XML +
  our curation. The committed files *are* the open dataset.
- **Shape:** a **batch CLI**, not a service. Workflow: `builder init` once → edit `curation/*.yaml` →
  `builder build` → review the `git diff` → PR. The build is **deterministic** (sorted keys, fixed
  style) so diffs reflect real changes only.
- **Not built here:** Characters & Staff. anime-offline-database is anime-level only, and AniList is
  off-limits for redistribution — so that dataset has no permissive bulk source. It stays curated or
  app-runtime for now (Part 8).

---

## Part 2 — Inputs

| Input | Source | Committed? | Used for | License |
|---|---|:--:|---|---|
| `anime-offline-database.json` | manami-project (pulled by `init`) | ❌ cache | Franchise clustering (relations), titles + synonyms, season/year, episode counts, cross-IDs | **ODbL** — storable + redistributable (attribution + share-alike) |
| `anime-list.xml` | Anime-Lists/anime-lists (`init`) | ❌ cache | Episode **absolute ordering** (season offsets) | open — numbering facts |
| `anime-movieset-list.xml` | ScudLee/anime-lists (`init`) | ❌ cache | **Movie** grouping into sets | open — numbering facts |
| `curation/*.yaml` | hand-authored | ✅ | Editorial decisions no source has (Part 2.1) | ours |
| `config.yaml` | repo | ✅ | Source URLs + **pinned versions**, build settings | ours |

We **don't commit the vendor sources** — `init` downloads them into a gitignored cache (`.sources/`).
Only *our* output, curation, and config live in git. Pins + checksums in `config.yaml` keep builds
reproducible.

> **Why not AniList?** AniList has the richest data (characters, voice actors, multilingual titles),
> but its ToS forbids using the API "as a backup or data storage service" and "mass collection." That
> was fine when the *app* fetched it live and stored nothing. But this builder's whole purpose is to
> **store and redistribute** a dataset — which is exactly what the ToS prohibits. Since the repo is
> OSS, **AniList can't be a build source.** We rely instead on anime-offline-database, whose ODbL
> license explicitly grants the storage + redistribution we need.

### 2.1 Why a `curation/` layer at all?

anime-offline-database gives us per-anime **facts** (titles, season, episodes) and **raw relations**
("these anime are related"). It does **not** give us the *structure* the data model needs — and no
open source does:

- Which related anime form **one Series** vs separate Series vs a separate Franchise.
- `absoluteNumber`, alt-cut vs original, split-cour `part` labels.
- `WatchOrder`s (chronological), and identity merges.

The build **auto-derives everything it can** (clusters from relations, titles/season/episodes from
facts, ordering from `anime-list.xml`); `curation/*.yaml` supplies only the **editorial decisions** the
sources can't express, and **wins** where it disagrees with auto-derivation. It's meant to stay small —
the exceptions, not the bulk.

---

## Part 3 — Two commands

```
builder init                 # download the open-data sources into the gitignored cache, at pinned versions
builder build                # sources + curation → write data/*.yaml  (validates as it builds)
builder build <franchise>    # build just one franchise while iterating
```

That's the whole surface. We **don't** need `fetch` / `validate` / `diff` / `inspect`, because the
output lives in git:

| Would-be command | Why it's unnecessary |
|---|---|
| `fetch` | That's `init` (and re-running `init` refreshes the cache to new pins) |
| `diff` | `git diff` shows exactly what changed in `data/` |
| `inspect` / history | Open the YAML file; `git log` / `git blame` for history |
| `validate` | Validation is **intrinsic to `build`** — it aborts on dangling refs, unknown curation targets, or schema violations (CI just runs `build` and checks the tree is clean) |

---

## Part 4 — The build pipeline

1. **Load** the cached sources (from `init`).
2. **Cluster** franchises/series from anime-offline-database **relations**, bucketing media into
   `Season` / `Movie` / `Special` by type.
3. **Fill facts** from anime-offline-database: titles `{ original, translations }` (best-effort from
   `title` + `synonyms`), `releaseYear` / `releaseSeason`, episode counts, `externalIds`.
4. **Order** each linear Series: `absoluteNumber` from `anime-list.xml` offsets; movie sets from
   `anime-movieset-list.xml`.
5. **Apply curation** (`curation/*.yaml`): Series/Franchise boundaries, ordering decisions, alt-cut vs
   original, `part` labels, `WatchOrder`s. **Curation wins.**
6. **Validate** (referential integrity, schema, curation targets) — the build **aborts** on failure,
   so a successful build is always a valid dataset.
7. **Write** `data/*.yaml` deterministically.

**Scope:** the build only produces the **franchises we curate** — not all ~40k anime in the dump. The
sources are bulk files processed locally (no per-anime API calls, so no rate-limit concerns); a
franchise enters the dataset when you add its curation entry (or you pass one as `builder build
<franchise>` to iterate).

---

## Part 5 — Output layout

YAML files, one per top-level record, committed to the repo:

```
data/
  franchises/<franchise-id>.yaml   # franchise + its series / seasons / episodes / movies / specials / watchOrders
config.yaml                        # source pins + settings        (input, committed)
curation/                          # hand-authored decisions        (input, committed)
.sources/                          # vendored open data, pinned     (gitignored — pulled by `init`)
```

- **YAML, not JSON** — readable, comment-able, and what the data-model examples already use. The writer
  emits canonical YAML (sorted keys, stable style) so diffs are clean and reviewable.
- **One file per franchise** keeps PRs focused (file granularity confirmed good).
- **No `index.json`** — the directory layout *is* the index (`data/franchises/<id>.yaml` is
  predictable). If a consumer ever needs to enumerate without the git tree, a manifest can be generated
  on demand, but it isn't committed (it would just churn on every change).

(`data/characters/` and `data/staff/` are absent until that dataset has a permissive source — Part 8.)

---

## Part 6 — CLI, or an API too?

Because the dataset is **open files in a GitHub repo**, git already provides what a management API
would: storage, review (PRs), history, blame, and rollback.

| Concern | Interface | Why |
|---|---|---|
| **Build** | **CLI** (`builder init` + `build`) | Batch, reproducible, runs in CI; writes the data files |
| **Curation / data management** | **GitHub itself** — edit `curation/*.yaml`, open a PR | Review, history, rollback, audit are built in; no service to operate |
| **Serving to consumers** | The raw YAML in the repo (or a CDN / GitHub Pages mirror) | The committed files *are* the public dataset |
| **Admin write API / UI** | **Deferred** | Premature until curation outgrows hand-edited YAML |

**So: CLI-first, and GitHub is the database.** A management/write API is **not required**. Reach for an
API only when (a) consumers want server-side querying rather than fetching whole files (a thin
**read-only** API over the YAML), or (b) curation grows enough to want a web admin UI — which would
wrap the same `curation` + `builder build` pipeline behind a PR, not replace it.

---

## Part 7 — Go package layout (sketch)

```
cmd/builder/                 # cobra entrypoint: `init` and `build`
internal/
  config/                    # config.yaml: source pins + checksums + settings
  sources/
    offlinedb/               # anime-offline-database loader (relations, facts, cross-IDs)
    animelists/              # anime-list.xml + anime-movieset-list.xml parsers
  curation/                  # curation/*.yaml loader + schema
  model/                     # the entities from the data-model docs
  resolve/                   # cluster → fill facts → order → apply curation
  validate/                  # referential integrity + schema checks (run inside build)
  writer/                    # deterministic YAML writer → data/*.yaml
```

Suggested libraries: **cobra** (`init`/`build`), **koanf**/**viper** (config), a canonical YAML
encoder. No GraphQL client, no database driver — sources are bulk files, output is YAML.

---

## Part 8 — Open questions

- **Characters & Staff sourcing — the big one.** anime-offline-database is anime-level only and AniList
  can't be redistributed, so that dataset has **no permissive bulk source**. Options: hand-curate it,
  find a redistribution-friendly source (Kitsu? Jikan/MAL? — licensing is murky, needs checking), or
  keep characters/voice-actors **runtime-only** in the app (fetched live, stored nowhere) and leave
  them out of the committed dataset.
- **Title mapping** — anime-offline-database `synonyms` aren't language-tagged, so deriving
  `{ original, translations }` cleanly is fuzzy. How much is auto-derivable vs curated?
- **File granularity at the extreme** — one file per franchise is good; do very large franchises
  (long-running shōnen) ever need episodes split into their own files?
- **Incremental vs full rebuild** — start with deterministic **full** rebuilds; add incremental only if
  build time becomes a problem.
- **Source pinning & drift** — `init` records checksums; how often to bump pins, and how to surface
  upstream schema changes in CI.
