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

A Go CLI named **`builder`**. The structured dataset — the `data/*.yaml` files — **is the thing we
build by hand**: which anime form a `Franchise`/`Series`, the season ordering, the alt-cut and
`WatchOrder` decisions. That structure is the project's work product; no open source contains it. The
builder's job is **narrow**: for each anime we reference by id, **fill in the factual fields** (titles,
season/year, episode counts) from open data, and **compute `absoluteNumber`** from the ordering XML —
then validate. The output is **YAML committed to this repo**, so the dataset is itself **open data**.

> **TL;DR**
> - We author **structure**; the builder fills **facts**. There is **no separate "curation" layer** —
>   the `data/*.yaml` files *are* the authored dataset, enriched in place.
> - Two commands: **`builder init`** pulls the open-data sources locally (not committed); **`builder
>   build`** fills facts + numbering and validates.
> - **GitHub is the database, diff, history, and management layer** — no extra subcommands, **no API**.
> - **No AniList** — its ToS forbids redistribution, which is incompatible with open data (Part 2).

---

## Part 1 — Who owns which fields

A `data/*.yaml` file has two kinds of fields. We write one kind; the builder writes the other and never
touches ours:

| Field group | Owner | Where it comes from |
|---|---|---|
| Structure — `Franchise`/`Series`/`Season` boundaries, membership (anime ids), `part` labels, `alternateCutOf`, `WatchOrder`s | **us (authored)** | the work product — our decisions |
| Facts — `title { original, translations }`, `releaseYear`/`releaseSeason`, episode counts, `externalIds` | **builder** | anime-offline-database (by id) |
| `absoluteNumber` | **builder** | `anime-list.xml` offsets |

So `builder build` is essentially "**resolve every referenced id, fill the fact fields, compute the
numbers, validate, and rewrite the file deterministically**." Re-running it refreshes facts when
sources update; it preserves everything we authored. (Like a generated lockfile section: you edit
structure, the tool fills the rest, the `git diff` shows what changed.)

---

## Part 2 — Inputs

| Input | Source | Committed? | Used for | License |
|---|---|:--:|---|---|
| `data/*.yaml` (structure fields) | **us** | ✅ | The dataset we author | ours |
| `anime-offline-database.json` | manami-project (pulled by `init`) | ❌ cache | Fill facts (titles + synonyms, season/year, episodes) and cross-IDs, by id | **ODbL** — storable + redistributable |
| `anime-list.xml` | Anime-Lists/anime-lists (`init`) | ❌ cache | Compute `absoluteNumber` (season offsets) | open — numbering facts |
| `anime-movieset-list.xml` | ScudLee/anime-lists (`init`) | ❌ cache | Movie-set grouping | open — numbering facts |
| `config.yaml` | repo | ✅ | Source URLs + **pinned versions** + settings | ours |

We **don't commit the vendor sources** — `init` downloads them into a gitignored cache (`.sources/`).
Only the dataset and config live in git. Pins + checksums in `config.yaml` keep builds reproducible.

> **Why not AniList?** It has the richest data, but its ToS forbids using the API "as a backup or data
> storage service" and "mass collection." Fetching it live in an *app* (storing nothing) is fine;
> baking it into a **redistributed open dataset** is exactly what the ToS prohibits. So AniList can't be
> a build source. We use anime-offline-database, whose ODbL license grants the storage + redistribution
> we need.

---

## Part 3 — Two commands

```
builder init                 # download the open-data sources into the gitignored cache, at pinned versions
builder build                # fill facts + numbering across data/*.yaml, validate, rewrite
builder build <franchise>    # just one franchise while iterating
```

That's the whole surface. We **don't** need `fetch` / `validate` / `diff` / `inspect`:

| Would-be command | Why it's unnecessary |
|---|---|
| `fetch` | That's `init` (re-run it to refresh the cache to new pins) |
| `diff` | `git diff` shows exactly what changed in `data/` |
| `inspect` / history | Open the YAML; `git log` / `git blame` for history |
| `validate` | Validation is **intrinsic to `build`** — it aborts on missing ids, dangling refs, or schema violations (CI just runs `build` and checks the tree is clean) |

A new franchise = create `data/franchises/<id>.yaml` with the structure (referencing anime by id) and
run `build` to fill it in. (A future `--scaffold` helper could seed membership from
anime-offline-database relations, but it's optional sugar — see Part 7.)

---

## Part 4 — The build pipeline

For each `data/*.yaml`:

1. **Load** the cached sources (from `init`).
2. **Resolve** every referenced anime id against anime-offline-database; **fail** if an id is unknown.
3. **Fill facts**: `title { original, translations }` (best-effort from `title` + `synonyms`),
   `releaseYear` / `releaseSeason`, episode counts, and the `externalIds` cross-map.
4. **Compute `absoluteNumber`** for linear Series from `anime-list.xml` offsets; group movie sets from
   `anime-movieset-list.xml`. (Non-linear Series get no numbers — release-date order.)
5. **Validate** the merged record (schema, referential integrity, our `alternateCutOf`/`WatchOrder`
   targets) — the build **aborts** on failure.
6. **Rewrite** the file deterministically (sorted keys, stable style), preserving authored fields.

Sources are **bulk files processed locally** — no per-anime API calls, so no rate limits. The build's
scope is exactly the `data/*.yaml` files that exist (all of them, or one passed as an argument); it
never crawls the ~40k-anime dump beyond the ids we reference.

---

## Part 5 — Repo layout

```
data/
  franchises/<franchise-id>.yaml   # authored structure, enriched in place with facts + numbers
config.yaml                        # source pins + settings        (committed)
.sources/                          # vendored open data, pinned     (gitignored — pulled by `init`)
```

- **YAML** — readable, comment-able, and what the data-model examples already use. The writer emits
  canonical YAML (sorted keys, stable style) so diffs are clean.
- **One file per franchise** keeps PRs focused (file granularity confirmed good).
- **No `index.json`** — the directory layout *is* the index (`data/franchises/<id>.yaml` is
  predictable). A manifest can be generated on demand if a consumer needs to enumerate, but it isn't
  committed (it would just churn).

(`data/characters/` and `data/staff/` are absent until that dataset has a permissive source — Part 8.)

---

## Part 6 — CLI, or an API too?

Because the dataset is **open files in a GitHub repo**, git already provides what a management API
would: storage, review (PRs), history, blame, and rollback.

| Concern | Interface | Why |
|---|---|---|
| **Build** | **CLI** (`builder init` + `build`) | Batch, reproducible, runs in CI |
| **Authoring / data management** | **GitHub itself** — edit `data/*.yaml`, open a PR | Review, history, rollback, audit are built in; no service to operate |
| **Serving to consumers** | The raw YAML in the repo (or a CDN / GitHub Pages mirror) | The committed files *are* the public dataset |
| **Admin write API / UI** | **Deferred** | Premature until authoring outgrows hand-edited YAML |

**So: CLI-first, and GitHub is the database.** A management/write API is **not required**. Reach for an
API only when (a) consumers want server-side querying rather than fetching whole files (a thin
**read-only** API over the YAML), or (b) authoring grows enough to want a web admin UI — which would
wrap the same files + `builder build` behind a PR, not replace it.

---

## Part 7 — Go package layout (sketch)

```
cmd/builder/                 # cobra entrypoint: `init` and `build` (+ optional `--scaffold`)
internal/
  config/                    # config.yaml: source pins + checksums + settings
  sources/
    offlinedb/               # anime-offline-database loader (facts, cross-IDs, relations)
    animelists/              # anime-list.xml + anime-movieset-list.xml parsers
  model/                     # the entities from the data-model docs
  build/                     # resolve ids → fill facts → compute numbers → validate
  writer/                    # deterministic YAML reader/writer (preserves authored fields)
```

Libraries: **cobra** (`init`/`build`), **koanf**/**viper** (config), a canonical YAML encoder. No
GraphQL client, no database driver — sources are bulk files, output is YAML.

---

## Part 8 — Open questions

- **Characters & Staff sourcing — the big one.** anime-offline-database is anime-level only and AniList
  can't be redistributed, so that dataset has **no permissive bulk source**. Options: hand-author it
  too, find a redistribution-friendly source (Kitsu? Jikan/MAL? — licensing needs checking), or keep
  characters/voice-actors **runtime-only** in the app and out of the committed dataset.
- **Title auto-fill** — anime-offline-database `synonyms` aren't language-tagged, so filling
  `{ original, translations }` cleanly is fuzzy. How much can the builder fill vs how much do we author
  (e.g. just pick the native-script and `en` synonyms, leave the rest)?
- **Scaffolding new franchises** — worth a `--scaffold` that proposes membership from relations, or is
  hand-listing ids simpler and safer?
- **Incremental vs full rebuild** — start with deterministic **full** rebuilds; add incremental only if
  build time becomes a problem.
- **Source pinning & drift** — `init` records checksums; how often to bump pins, and how to surface
  upstream schema changes in CI.
