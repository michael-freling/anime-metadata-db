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

A Go CLI named **`builder`** that compiles a hand-authored input into the full open dataset. There are
**two committed layers**, in **separate files**, so a rebuild can never clobber what we wrote:

- **`overrides/*.yaml`** — **authored** by us; the builder **never writes to it**. Our structure and
  decisions: which anime form a `Franchise`/`Series`/`Season`, the ordering, alt-cut, `WatchOrder`s.
- **`data/*.yaml`** — **generated** by `builder build`. The open dataset = our overrides **+** facts
  (titles, season/year, episodes from open data) **+** computed `absoluteNumber`. Never hand-edited.

The output is YAML committed to the repo, so the dataset is itself **open data**.

> **TL;DR**
> - Two files, never mixed: hand-edit **`overrides/`**, the builder generates **`data/`** from it +
>   open sources. So `build` is **idempotent** and **can't lose authored data**.
> - Commands: **`builder init`** (pull pinned sources) · **`builder build`** (incremental — add new) ·
>   **`builder refresh`** (update sources + rebuild all). Sources aren't committed.
> - **GitHub is the database, diff, history, and management layer** — no extra subcommands, **no API**.
> - **No AniList** — its ToS forbids redistribution, incompatible with open data (Part 2).

---

## Part 1 — The two layers

| Layer | Who writes it | Committed? | Holds |
|---|---|:--:|---|
| `overrides/*.yaml` | **us** (hand-edited) | ✅ | Structure + decisions the sources can't express — Series/Franchise boundaries, membership (anime ids), `number`/`part`, `alternateCutOf`, `WatchOrder`s |
| `data/*.yaml` | **`builder build`** (generated) | ✅ | The full resolved records: our overrides **+** filled facts **+** computed numbers — the public dataset |

`builder build` reads `overrides/` + the open sources and **writes `data/`**. It treats `overrides/` as
**read-only**, so:

- You only ever hand-edit `overrides/` — small files, just the decisions.
- `data/` is regenerated every build; never edit it by hand. `git diff data/` shows what the build did.
- A rebuild is **deterministic and idempotent** — same overrides + same pinned sources ⇒ same `data/`.
  **No clobbering**, because input and output are different files.

We commit *both*: `overrides/` is the maintainer surface; `data/` is the dataset consumers fetch without
running the builder. CI rebuilds and asserts `data/` is unchanged (no drift).

---

## Part 2 — Inputs

| Input | Source | Committed? | Used for | License |
|---|---|:--:|---|---|
| `overrides/*.yaml` | **us** | ✅ | The authored structure + decisions | ours |
| `anime-offline-database.json` | manami-project (pulled by `init`) | ❌ cache | Fill facts (titles + synonyms, season/year, episodes) + cross-IDs, by id | **ODbL** — storable + redistributable |
| `anime-list.xml` | Anime-Lists/anime-lists (`init`) | ❌ cache | Compute `absoluteNumber` (season offsets) | open — numbering facts |
| `anime-movieset-list.xml` | ScudLee/anime-lists (`init`) | ❌ cache | Movie-set grouping | open — numbering facts |
| `config.yaml` | repo | ✅ | Source URLs + **pinned versions** + settings | ours |

We **don't commit the vendor sources** — `init` downloads them into a gitignored cache (`.sources/`).
Pins + checksums in `config.yaml` keep builds reproducible.

> **Why not AniList?** It has the richest data, but its ToS forbids using the API "as a backup or data
> storage service" and "mass collection." Fetching it live in an *app* (storing nothing) is fine;
> baking it into a **redistributed open dataset** is exactly what the ToS prohibits. So AniList can't be
> a build source. We use anime-offline-database, whose ODbL license grants storage + redistribution.

---

## Part 3 — Commands

```
builder init                 # download the open-data sources into the gitignored cache, at the PINNED versions
builder build                # incremental: generate data/ for NEW or changed overrides only
builder build <franchise>    # build / rebuild just one franchise
builder refresh              # update sources to LATEST (bump pins) + rebuild ALL of data/
```

The `overrides/*.yaml` files **are** "the list of anime we want," so:

- **`build` is incremental** — it only (re)generates `data/` for overrides that are new or changed (no
  up-to-date output). Adding a franchise is fast: write its override, run `build`.
- **`refresh` is the full update** — it re-pulls the sources to their latest versions (bumping the pins
  + checksums in `config.yaml`) and rebuilds **every** `data/` file, so upstream changes (new episodes,
  corrected titles) flow in. Run it periodically (e.g. a scheduled CI job).
- **`init`** fetches only the *pinned* sources — for a fresh clone or CI, reproducibly (no bump).

We still **don't** need `fetch` / `validate` / `diff` / `inspect`:

| Would-be command | Why it's unnecessary |
|---|---|
| `fetch` | `init` fetches pinned sources; `refresh` bumps them |
| `diff` | `git diff data/` shows exactly what changed |
| `inspect` / history | Open the YAML; `git log` / `git blame` for history |
| `validate` | Intrinsic to `build`/`refresh` — aborts on missing ids, dangling refs, or schema violations (CI just runs `build` and checks the tree is clean) |

A new franchise = create `overrides/franchises/<id>.yaml` and run `build`. (A future `--scaffold` helper
could seed membership from anime-offline-database relations — optional sugar, Part 8.)

---

## Part 4 — The build pipeline

`builder build` does, for each `overrides/*.yaml`:

1. **Load** the cached sources (from `init`) and the override file.
2. **Resolve** every referenced anime id against anime-offline-database; **fail** on an unknown id.
3. **Fill facts**: `title { original, translations }` (best-effort from `title` + `synonyms`),
   `releaseYear` / `releaseSeason`, episode counts, the `externalIds` cross-map.
4. **Compute `absoluteNumber`** for linear Series from `anime-list.xml`; group movie sets from
   `anime-movieset-list.xml`. (Non-linear Series get no numbers — release-date order.)
5. **Apply** the override's structure + decisions on top — **overrides win** on any conflict.
6. **Validate** (schema, referential integrity, `alternateCutOf`/`WatchOrder` targets) — **aborts** on
   failure, so a successful build is always a valid dataset.
7. **Write** the resolved record to `data/<...>.yaml` deterministically (sorted keys, stable style).

Sources are **bulk files processed locally** — no per-anime API calls, so no rate limits. Scope is
exactly the `overrides/*.yaml` files that exist (all, or one passed as an argument).

**Build report.** Where the builder *guesses* (chiefly title language tagging — Part 8), it emits a
**report of low-confidence decisions** — warnings on stdout plus an optional gitignored
`build-report.yaml` (which synonym it chose as `original`, Latin titles it couldn't split into `en`
vs `ja-Latn`). That's the review surface: you fix only the flagged cases with an override, instead of
eyeballing every title.

---

## Part 5 — Repo layout

```
overrides/
  franchises/<franchise-id>.yaml   # AUTHORED structure + decisions (committed; builder never writes)
data/
  franchises/<franchise-id>.yaml   # GENERATED dataset (committed; never hand-edit)
config.yaml                        # source pins + settings        (committed)
.sources/                          # vendored open data, pinned     (gitignored — pulled by `init`)
```

- **YAML** both layers — readable, comment-able, what the data-model examples use. The writer emits
  canonical YAML so `data/` diffs are clean.
- **One file per franchise** in each layer keeps PRs focused.
- **No `index.json`** — the directory layout *is* the index. A manifest can be generated on demand if a
  consumer needs to enumerate, but it isn't committed (it would just churn).

(`overrides/characters/` + `data/characters/` are absent until that dataset has a permissive source — Part 8.)

---

## Part 6 — CLI, or an API too?

Because the dataset is **open files in a GitHub repo**, git already provides what a management API
would: storage, review (PRs), history, blame, and rollback.

| Concern | Interface | Why |
|---|---|---|
| **Build** | **CLI** (`builder init` + `build`) | Batch, reproducible, runs in CI |
| **Authoring / data management** | **GitHub itself** — edit `overrides/*.yaml`, open a PR | Review, history, rollback, audit built in; no service to operate |
| **Serving to consumers** | The raw `data/*.yaml` in the repo (or a CDN / Pages mirror) | The committed files *are* the public dataset |
| **Admin write API / UI** | **Deferred** | Premature until authoring outgrows hand-edited YAML |

**So: CLI-first, and GitHub is the database.** A management/write API is **not required**. Reach for an
API only when (a) consumers want server-side querying rather than fetching whole files (a thin
**read-only** API over `data/`), or (b) authoring grows enough to want a web admin UI — which would wrap
the same `overrides/` + `builder build` behind a PR, not replace it.

---

## Part 7 — Go package layout (sketch)

```
cmd/builder/                 # cobra entrypoint: `init` and `build` (+ optional `--scaffold`)
internal/
  config/                    # config.yaml: source pins + checksums + settings
  sources/
    offlinedb/               # anime-offline-database loader (facts, cross-IDs, relations)
    animelists/              # anime-list.xml + anime-movieset-list.xml parsers
  overrides/                 # overrides/*.yaml loader + schema
  model/                     # the entities from the data-model docs
  build/                     # resolve ids → fill facts → compute numbers → apply overrides → validate
  writer/                    # deterministic YAML writer → data/*.yaml
```

Libraries: **cobra** (`init`/`build`), **koanf**/**viper** (config), a canonical YAML encoder. No
GraphQL client, no database driver — sources are bulk files, output is YAML.

---

## Part 8 — Open questions

- **How much do overrides carry?** anime-offline-database `relations` can *propose* a franchise cluster,
  so overrides might only adjust boundaries — or, if clustering is unreliable, overrides list membership
  explicitly. Which is less work in practice? (Affects how big `overrides/` files get.)
- **Wikidata coverage** — using Wikidata for characters/VAs (below) means **lower coverage** than
  AniList; how much hand-authoring will the long-tail gaps actually need before it's useful?

Settled during review:

- **Characters & Staff source → Wikidata (CC0).** It's the one major source that's *freely
  redistributable* and models anime characters + voice actors — and it's **structured (SPARQL/JSON), so
  it's the easy one to consume**. It would be a builder **source adapter** (pulled by `init`/`refresh`
  like the others), *not* a separate command. Wikipedia/DBpedia (CC BY-SA) are a **noisy last resort** —
  prose / messy infobox extraction, plus share-alike obligations — so prefer Wikidata and hand-author
  the long-tail gaps. AniList / MAL (Jikan) / AniDB / Kitsu / TMDB don't grant redistribution — they
  stay **runtime-only** (the app may fetch them live for display, storing nothing). (Built in a later
  iteration — see the [Characters & Staff Data Model](../data-model-characters-staff/).)
- **Title auto-fill → auto-fill + report + override** (not hand-author every title). The builder fills
  `original` from the CJK/native-script synonym and keeps the dump's `title` as a Latin name; precise
  `translations` (`en` vs `ja-Latn` vs `ko`) are best-effort by script. It **reports** the low-confidence
  guesses (Part 4) and a `title` set in `overrides/` **wins** — so you review only the flagged cases.
- **Claude-assisted authoring (idea).** For the messy long-tail Wikidata won't cover, a Claude Code
  **slash command** could extract a title's cast from Wikipedia and *propose* `overrides/` YAML for a
  human to review — an **authoring aid that produces overrides**, kept separate from the deterministic
  `builder build`. Worth building once the characters dataset starts.
- **Incremental vs full → `build` (incremental, add new) + `refresh` (rebuild all)** — Part 3.
- **Source pinning & drift → `refresh`** bumps pins + checksums and rebuilds (run on a schedule);
  `init` stays pinned/reproducible — Part 3.
