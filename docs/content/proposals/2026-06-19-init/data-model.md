---
title: "Franchise Data Model & Examples"
date: 2026-06-19
weight: 2
---

# Franchise Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)

This note expands the `Franchise` / `TimelineEntry` model sketched in §5.2 of the
[research note](../anime-metadata-research/) and grounds it in two concrete
franchises so the fields are easy to reason about:

- **Rascal Does Not Dream** — the motivating case: subtitled TV parts plus movies
  that only sort correctly with a franchise-wide absolute number.
- **Demon Slayer** — the dedup case: *Mugen Train* exists **both** as a theatrical
  movie **and** as a recompiled TV arc, so the model has to decide which release
  carries the canonical numbers.

> **Scope.** This model owns *ordering and grouping* (R1). Per-title content (R2)
> stays in AniList; per-episode content (R3) is a known gap. See the research note
> §4 for why. AniList IDs and episode counts below are **illustrative** — they are
> seeded/verified from `anime-offline-database` at build time (§5.3), not hand-kept.

## 1. Entities

```text
Franchise
  id                 (our stable id)
  canonicalTitle
  anilistIds[]       (member media ids — TV, movies, OVAs)
  timeline[]         ordered list of TimelineEntry

TimelineEntry
  kind               TV_EPISODE | MOVIE | OVA | SPECIAL
  partTitle          display-only subtitle (e.g. "Dreaming Girl")
  absoluteNumber     int   <-- primary sort key across the whole franchise
  airedSeason        int?  (TV only)
  airedEpisode       int?  (TV only)
  releaseDate        date  (used to interleave movies into watch order)
  episodeTitle       string?  (R3 — only if curated / non-commercial TMDB)
  sourceRefs         { anilistId, anidbId?, tmdbId?, tvdbId? }
```

### 1.1 Field reference

| Field | Entity | Why it exists | Storable? (research note §5.1a) |
|---|---|---|---|
| `id` | Franchise | Our stable key, independent of any upstream | ✅ ours |
| `canonicalTitle` | Franchise | Display name for the whole series | ✅ fact/ours |
| `anilistIds[]` | Franchise | Members to enrich live from AniList (R2) | ✅ IDs are facts |
| `kind` | TimelineEntry | Distinguishes an episode from a movie/OVA/special | ✅ fact |
| `partTitle` | TimelineEntry | The subtitled part, e.g. *Dreaming Girl* | ✅ fact |
| **`absoluteNumber`** | TimelineEntry | **The one field no free API gives us** — franchise-wide sort key | ✅ our derived data |
| `airedSeason` / `airedEpisode` | TimelineEntry | Local per-release numbering (what AniList exposes) | ✅ fact |
| `releaseDate` | TimelineEntry | Interleaves movies into watch order | ✅ fact |
| `episodeTitle` | TimelineEntry | R3 content — optional, only if curated/non-commercial | ⚠️ only if curated/owned |
| `sourceRefs` | TimelineEntry | Cross-source IDs for live fetch (synopsis, art) | ✅ IDs are facts |

The model **stores facts** (IDs, numbers, dates, our computed `absoluteNumber`) and
**fetches expression** (synopsis, cover art, stills) live — never warehousing
AniList/TMDB content (research note §5.1a).

## 2. Example A — Rascal Does Not Dream (the motivating case)

### 2.1 The problem, in data

This is what the current AniList-only world looks like — four independent media
nodes, each numbered locally, with no field tying them into one order:

| AniList node | kind | local numbering | release |
|---|---|---|---|
| *Bunny Girl Senpai* (TV) | TV, 13 eps | episodes **1–13** | 2018-10 |
| *Dreaming Girl* (movie) | Movie | — (its own node) | 2019-06 |
| *Sister Venturing Out* (movie) | Movie | — | 2023-06 |
| *Knapsack Kid* (movie) | Movie | — | 2025-06 |

Nothing here says the *Dreaming Girl* movie is watched **after** TV episode 13, or
that all four are one series. That is the gap `absoluteNumber` closes.

### 2.2 As a `Franchise` record

```yaml
Franchise:
  id: rascal-does-not-dream
  canonicalTitle: "Rascal Does Not Dream (Seishun Buta Yarō)"
  anilistIds: [101291, 104157, 143653, 162804]   # illustrative
  timeline:
    # --- Bunny Girl Senpai, TV season 1 (13 episodes) ---
    - { kind: TV_EPISODE, partTitle: "Bunny Girl Senpai", absoluteNumber: 1,
        airedSeason: 1, airedEpisode: 1,  releaseDate: 2018-10-03,
        sourceRefs: { anilistId: 101291 } }
    # … episodes 2–12 elided …
    - { kind: TV_EPISODE, partTitle: "Bunny Girl Senpai", absoluteNumber: 13,
        airedSeason: 1, airedEpisode: 13, releaseDate: 2018-12-27,
        sourceRefs: { anilistId: 101291 } }

    # --- Movie, slotted AFTER ep 13 by releaseDate ---
    - { kind: MOVIE, partTitle: "Dreaming Girl", absoluteNumber: 14,
        releaseDate: 2019-06-15, sourceRefs: { anilistId: 104157 } }

    - { kind: MOVIE, partTitle: "Sister Venturing Out", absoluteNumber: 15,
        releaseDate: 2023-06-23, sourceRefs: { anilistId: 143653 } }   # illustrative

    - { kind: MOVIE, partTitle: "Knapsack Kid", absoluteNumber: 16,
        releaseDate: 2025-06-06, sourceRefs: { anilistId: 162804 } }   # illustrative
```

### 2.3 The resulting watch order

| `absoluteNumber` | kind | part | local # | release |
|:--:|---|---|:--:|---|
| 1 | TV_EPISODE | Bunny Girl Senpai | S1E1 | 2018-10-03 |
| … | TV_EPISODE | Bunny Girl Senpai | … | … |
| 13 | TV_EPISODE | Bunny Girl Senpai | S1E13 | 2018-12-27 |
| 14 | MOVIE | Dreaming Girl | — | 2019-06-15 |
| 15 | MOVIE | Sister Venturing Out | — | 2023-06-23 |
| 16 | MOVIE | Knapsack Kid | — | 2025-06-06 |

Sorting by `absoluteNumber` yields the franchise watch order. The movies fall into
place because their `releaseDate` interleaves them against the TV episodes — exactly
the sort that AniList alone cannot produce.

## 3. Example B — Demon Slayer (the dedup case)

Demon Slayer stresses a different part of the model: **the same story content ships
twice** — *Mugen Train* was a 2020 theatrical movie, then re-cut into a 7-episode TV
arc in 2021. The model must pick one canonical carrier so the arc isn't double-counted.

### 3.1 The AniList view (locally numbered, content overlaps)

| AniList node | kind | local numbering | release | note |
|---|---|---|---|---|
| *Kimetsu no Yaiba* (TV S1) | TV, 26 eps | **1–26** | 2019-04 | AniList 101922 |
| *Mugen Train* (movie) | Movie | — | 2020-10 | AniList 112151 |
| *Mugen Train Arc* (TV) | TV, 7 eps | **1–7** | 2021-10 | **recompiles the movie** |
| *Entertainment District Arc* | TV, 11 eps | **1–11** | 2021-12 | AniList 142329 |
| *Swordsmith Village Arc* | TV, 11 eps | **1–11** | 2023-04 | AniList 145139 |
| *Hashira Training Arc* | TV, 8 eps | **1–8** | 2024-05 | AniList 166240 |

Two problems at once: (a) every season restarts at 1, and (b) *Mugen Train* appears
as both a movie and a TV arc covering the same events.

### 3.2 As a `Franchise` record (movie is canonical; TV recompile suppressed)

```yaml
Franchise:
  id: demon-slayer
  canonicalTitle: "Demon Slayer: Kimetsu no Yaiba"
  anilistIds: [101922, 112151, 142329, 145139, 166240]   # 142984 (TV recompile) excluded
  timeline:
    # --- Season 1 (26 episodes) ---
    - { kind: TV_EPISODE, partTitle: "Kimetsu no Yaiba", absoluteNumber: 1,
        airedSeason: 1, airedEpisode: 1,  releaseDate: 2019-04-06,
        sourceRefs: { anilistId: 101922 } }
    # … episodes 2–25 elided …
    - { kind: TV_EPISODE, partTitle: "Kimetsu no Yaiba", absoluteNumber: 26,
        airedSeason: 1, airedEpisode: 26, releaseDate: 2019-09-28,
        sourceRefs: { anilistId: 101922 } }

    # --- Mugen Train: the THEATRICAL MOVIE is the canonical carrier ---
    - { kind: MOVIE, partTitle: "Mugen Train", absoluteNumber: 27,
        releaseDate: 2020-10-16, sourceRefs: { anilistId: 112151 } }

    # NOTE: the 2021 "Mugen Train Arc" TV recompile (AniList 142984) is the SAME
    # content. It is dropped from the timeline via franchise-overrides.yaml so the
    # arc is counted once. (Decision recorded below.)

    # --- Entertainment District Arc (11 eps) continues the absolute count ---
    - { kind: TV_EPISODE, partTitle: "Entertainment District", absoluteNumber: 28,
        airedSeason: 2, airedEpisode: 1,  releaseDate: 2021-12-05,
        sourceRefs: { anilistId: 142329 } }
    # … through absoluteNumber 38 …

    # --- Swordsmith Village (11) → Hashira Training (8) follow, 39–49, 50–57 ---
```

### 3.3 Why this exercises the model

| Concern | How the model handles it |
|---|---|
| Seasons restart at 1 | `absoluteNumber` provides the continuous franchise count; `airedSeason`/`airedEpisode` keep the local numbers for reference |
| Movie ↔ TV duplicate (*Mugen Train*) | Pick one `kind: MOVIE` carrier; suppress the recompiled TV arc in `franchise-overrides.yaml` (research note §5.3 step 4) |
| Movie placement in watch order | `releaseDate: 2020-10-16` slots it between S1 (2019) and Entertainment District (2021) |
| Which IDs to enrich (R2) | `anilistIds[]` lists the canonical members; the excluded recompile ID is simply not a member |

This dedup decision is a **curation override**, not something any upstream file
decides for us — it is exactly the "thin franchise/ordering layer" the research note
recommends owning (§5.5).

## 4. How these records get built

Maps to the research note §5.3 pipeline:

1. **Seed** membership from `anime-offline-database` → which AniList IDs cluster as
   one franchise (gives `anilistIds[]`).
2. **Order** the TV side from `anime-list.xml` offsets → `absoluteNumber` for episodes.
3. **Slot movies** from `anime-movieset-list.xml`, interleaving by `releaseDate`.
4. **Override** the gaps the open data misses — e.g. *suppress Demon Slayer's Mugen
   Train TV recompile*, or fix a movie's slot — in `franchise-overrides.yaml`.
5. **Store** the resolved `Franchise` records next to the existing
   `internal/db/anime.go` schema.
6. **Refresh** upstream files on a schedule; overrides always win.

## 5. Open questions

- **Absolute numbering across movies** — do movies consume an `absoluteNumber` in the
  same sequence as episodes (as shown), or live in a parallel movie index? Shown here
  as a single shared sequence so one sort key drives the whole timeline.
- **Recompiled arcs** — always prefer the movie as canonical, or prefer the TV
  recompile when it adds new footage (the *Mugen Train* TV cut has an extra episode 1)?
  Currently a per-franchise override decision.
- **OVA / specials placement** — by `releaseDate`, or pinned to a story slot via override?
- **R3 `episodeTitle`** — left empty here; only populated if curated or sourced from a
  non-commercial build (research note §3.3).
