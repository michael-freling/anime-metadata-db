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

A Go CLI named **`builder`** that builds the anime-metadata dataset from open-data sources plus our
curation overrides, and writes it as **data files committed to this GitHub repo** — so the dataset is
itself **open data**. It assembles the **R1** ordering/grouping model and the **storable R2 facts**
(ids, names, the character/voice-actor graph). It deliberately does **not** fetch or store
"expression" (synopses, art, episode stills) — per the research note's facts-vs-expression rule
(§5.1a), the *app* fetches those live at display time.

> **TL;DR:** it's **one command** — `builder build` — that turns config + overrides + sources into
> JSON data files in the repo. **Git is the database, the diff, the history, and the management
> layer**, so there's no `validate`/`diff`/`inspect`/`fetch` subcommand and **no management API** (Part 3, Part 6).

---

## Part 1 — What it builds, and where it runs

- **Output:** **diff-friendly JSON files committed to the repo** (Part 5) holding the resolved
  `Franchise → Series → Season → Episode` tree plus `Movie`, `Special`, `WatchOrder`, and the
  `Character` / `Staff` graph from the data-model docs — **facts only**. The committed files *are*
  the open dataset; anyone can browse, diff, or download them straight from GitHub.
- **Shape:** a **batch CLI**, not a long-running service. The workflow is: edit overrides → run
  `builder build` → review the `git diff` → commit/PR. The build is **deterministic and reproducible**:
  same pinned inputs + same overrides ⇒ byte-identical output (stable key order, fixed formatting), so
  diffs reflect real changes only.

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

## Part 3 — One command

The whole tool is essentially one command — it writes the dataset into the repo:

```
builder build                       # config + overrides + sources → write data/*.json (validates as it builds)
builder build --update-sources      # refresh the vendored open-data sources first, then build
builder build --scope franchise=fate  # build a subset while iterating
```

We **don't** need the usual `fetch` / `validate` / `diff` / `inspect` subcommands, because the output
lives in git:

| Would-be command | Why it's unnecessary |
|---|---|
| `diff` | `git diff` already shows exactly what changed in `data/` |
| `inspect` / history | Open the JSON file; `git log` / `git blame` for history |
| `validate` | Validation is **intrinsic to `build`** — it fails on dangling refs, unknown override targets, or schema violations (so CI just runs `build` and checks the tree is clean) |
| `fetch` | A `--update-sources` flag on `build`; refreshing pinned sources isn't a separate workflow |

Curation is just: **edit `overrides/*.yaml` → `builder build` → review the `git diff` → commit/PR.**

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
6. **Validate** referential integrity (every edge resolves), schema, and override targets — the build
   **aborts** here on any failure, so a successful build is always a valid dataset.
7. **Write** the resolved records to `data/*.json` (Part 5), deterministically — stable key order and
   fixed formatting so the `git diff` shows only real changes.

> Expression (synopses, cover art, stills, bios) is **out of scope** here — the app fetches it live
> from AniList/TMDB keyed by the `externalIds` this tool stores.

---

## Part 5 — Storage / output

**Recommendation: JSON files committed to the repo.** The dataset is open data, so it should be
**human-readable, diffable, and downloadable straight from GitHub** — which rules out SQLite (a binary
blob produces no meaningful `git diff` and can't be reviewed in a PR). One JSON file per top-level
record keeps diffs small and PRs focused:

```
data/
  franchises/<franchise-id>.json   # franchise + its series / seasons / episodes / movies / specials / watchOrders
  characters/<character-id>.json
  staff/<staff-id>.json
  index.json                       # manifest: ids → paths, for consumers
overrides/                         # hand-authored YAML  (input)
sources/                           # vendored open data, pinned  (input)
config.yaml                        # source pins + build settings  (input)
```

JSON (not YAML) for the **generated** files: canonical, stable formatting gives clean diffs and it's
the obvious format for consumers to fetch. Overrides stay **YAML** (hand-authored, friendlier).

If a consumer later wants a queryable database, building a **SQLite** file *from* these JSON files is a
trivial, optional secondary step — but the committed JSON remains the source of truth.

---

## Part 6 — CLI, or an API too?

Because the dataset is **open files in a GitHub repo**, git already provides what a management API
would: storage, review (PRs), history, blame, and rollback.

| Concern | Interface | Why |
|---|---|---|
| **Build** | **CLI** (`builder build`) | Batch, reproducible, runs in CI; writes the data files |
| **Curation / data management** | **GitHub itself** — edit `overrides/*.yaml`, open a PR | Review, history, rollback, and audit are built into git/GitHub; no service to operate |
| **Serving to consumers** | The raw JSON files in the repo (or a CDN/GitHub Pages mirror) | The committed files *are* the public dataset — fetch them directly |
| **Admin write API / UI** | **Deferred** | Premature until curation outgrows hand-edited YAML |

**So: CLI-first, and GitHub is the database.** A management/write API is **not required**. Reach for an
API only when (a) consumers want server-side querying/filtering rather than fetching whole files (a
thin **read-only** API over the JSON), or (b) curation grows enough to want a web admin UI — which
would wrap the same `overrides` + `builder build` pipeline behind a PR, not replace it.

---

## Part 7 — Go package layout (sketch)

```
cmd/builder/                 # cobra entrypoint: the single `build` command (+ flags)
internal/
  config/                    # config.yaml + source pins + lockfile
  sources/
    offlinedb/               # anime-offline-database loader
    animelists/              # anime-list.xml + anime-movieset-list.xml parsers
    anilist/                 # GraphQL client (facts only) + on-disk cache + rate limiter
  overrides/                 # YAML override loader + schema
  model/                     # the entities from the data-model docs
  resolve/                   # seed → order → apply overrides → resolve facts
  validate/                  # referential integrity + schema checks (run inside build)
  writer/                    # deterministic JSON writer → data/*.json
```

Suggested libraries: **cobra** (the one command + flags), **koanf**/**viper** (config), a GraphQL
client for AniList. Source adapters share a small interface so a source can be swapped or pinned
independently. No database driver needed — the writer emits canonical JSON.

---

## Part 8 — Open questions

- **File granularity** — one JSON file per franchise/character/staff (Part 5) keeps PR diffs small; do
  any huge franchises (long-running shōnen) need splitting further (e.g. episodes in their own file)?
- **Incremental vs full rebuild** — start with deterministic **full** rebuilds (simple, reproducible);
  add incremental only if build time becomes a problem.
- **AniList fact-fetch budget** — keyless ~90 req/min; the `anilist` cache + a rate limiter keep builds
  polite. How aggressively to pre-warm vs fetch lazily?
- **Override authoring at scale** — hand-edited YAML + PRs is fine now; a future web admin UI would wrap
  the same `overrides` + `builder build` pipeline behind a PR (Part 6), not replace it.
- **Source pinning & drift** — `fetch` records checksums; how often to bump pins, and how to surface
  upstream schema changes in CI.
