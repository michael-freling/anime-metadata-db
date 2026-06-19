---
title: "Franchise Data Model & Examples"
date: 2026-06-19
weight: 2
---

# Franchise / Anime Series Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)

This note refines the flat `Franchise` / `TimelineEntry` sketch from §5.2 of the
[research note](../anime-metadata-research/) into a **three-tier model** and grounds
it in two concrete franchises so the fields are easy to reason about:

- **Rascal Does Not Dream** — the motivating case: **two TV seasons plus movies**
  that only sort correctly with a franchise-wide absolute number.
- **Demon Slayer** — the dedup case: *Mugen Train* exists **both** as a theatrical
  movie **and** as a recompiled TV series, so the model has to decide which release
  carries the canonical numbers.

> **Scope.** This model owns *ordering and grouping* (R1). Per-series content (R2)
> stays in AniList; per-episode content (R3) is a known gap. See the research note
> §4 for why. AniList IDs, episode counts, and the Rascal Season 2 details below are
> **illustrative** — they are seeded/verified from `anime-offline-database` at build
> time (§5.3), not hand-kept.

## 1. Why three tiers

The §5.2 sketch had two levels (`Franchise` → `TimelineEntry`) and pushed an
`anilistId` onto every entry. Two problems fell out of that:

1. **No "series" level.** A *franchise* can contain several **distinct** stories —
   the *Fate* franchise spans *Fate/stay night*, *Fate/Zero*, … — while a TV
   episode is far too granular to enrich. The natural unit AniList actually
   describes (one TV season, or one movie) had nowhere to live.
2. **Duplicated `anilistId`.** All 13 episodes of a season repeated the same media
   id, and the `Franchise.anilistIds[]` list was just the distinct set of those —
   redundant, derivable, drift-prone.

Splitting out an **`AnimeSeries`** tier fixes both: it *is* the AniList media node
(so it owns the `anilistId` exactly once and is the R2 enrichment unit), and
`Franchise` becomes a thin umbrella that can group one or many series.

```text
Franchise            umbrella; can hold several distinct AnimeSeries (e.g. Fate)
  id                 our stable id (e.g. "rascal-does-not-dream")
  titles             { english, romaji, native }
  series[]           ordered list of AnimeSeries (in watch / release order)

AnimeSeries          ONE AniList media node — a single TV season OR a movie/OVA
  id                 our stable id
  kind               TV | MOVIE | OVA | SPECIAL
  titles             { english, romaji, native }   (the part subtitle)
  airedSeason        int?   (TV only — the franchise's Nth TV season)
  releaseDate        date
  sourceRefs         { anilistId, anidbId?, tmdbId?, tvdbId? }   (one media node)
  timeline[]         ordered list of TimelineEntry

TimelineEntry        ONE watchable unit — a single TV episode, or a movie
  kind               TV_EPISODE | MOVIE | OVA | SPECIAL
  absoluteNumber     int   <-- franchise-wide sort key, across ALL series
  airedEpisode       int?  (TV only — local episode number within its season)
  releaseDate        date
  episodeTitle       string?  (R3 — only if curated / non-commercial TMDB)
```

`absoluteNumber` is assigned by walking `series[]` in watch order and numbering
every `TimelineEntry` across all of them — so it spans seasons *and* movies. A movie
is an `AnimeSeries` (`kind: MOVIE`) containing exactly one `TimelineEntry`, which
keeps the leaf list a single, uniformly sortable sequence.

### 1.1 Field reference

| Field | Entity | Why it exists | Storable? (research note §5.1a) |
|---|---|---|---|
| `id` | Franchise / AnimeSeries | Our stable key, independent of any upstream | ✅ ours |
| `titles {english,romaji,native}` | Franchise / AnimeSeries | Multi-name display — *Bunny Girl Senpai* (en) vs *Seishun Buta Yarō* (romaji) | ✅ facts |
| `series[]` | Franchise | The member series, in watch order | ✅ ours |
| `kind` | AnimeSeries / TimelineEntry | TV season vs movie vs OVA/special | ✅ fact |
| `airedSeason` | AnimeSeries | Which franchise TV season this is (1, 2, …) | ✅ fact |
| `sourceRefs.anilistId` | AnimeSeries | **The one media id**, kept once per series — the R2 enrichment key | ✅ ids are facts |
| **`absoluteNumber`** | TimelineEntry | **The one field no free API gives us** — franchise-wide sort key | ✅ our derived data |
| `airedEpisode` | TimelineEntry | Local per-season episode number (what AniList exposes) | ✅ fact |
| `releaseDate` | AnimeSeries / TimelineEntry | Interleaves movies into watch order | ✅ fact |
| `episodeTitle` | TimelineEntry | R3 content — optional, only if curated/non-commercial | ⚠️ only if curated/owned |

The model **stores facts** (ids, numbers, dates, our computed `absoluteNumber`) and
**fetches expression** (synopsis, cover art, stills) live — never warehousing
AniList/TMDB content (research note §5.1a).

## 2. Example A — Rascal Does Not Dream (two seasons + movies)

### 2.1 The problem, in data

This is what the current AniList-only world looks like — independent media nodes,
each numbered locally, with no field tying them into one order:

| AniList node | kind | local numbering | release |
|---|---|---|---|
| *Bunny Girl Senpai* (TV S1) | TV, 13 eps | episodes **1–13** | 2018-10 |
| *Dreaming Girl* (movie) | Movie | — (its own node) | 2019-06 |
| *Sister Venturing Out* (movie) | Movie | — | 2023-06 |
| *Season 2* (TV) | TV | episodes **1–N** | 2025 |

Nothing here says the *Dreaming Girl* movie is watched **after** TV episode 13, that
Season 2 comes after the movies, or that all of them are one series. That is the gap
`absoluteNumber` closes.

### 2.2 As a `Franchise` record

```yaml
Franchise:
  id: rascal-does-not-dream
  titles:
    english: "Rascal Does Not Dream"
    romaji:  "Seishun Buta Yarō"
    native:  "青春ブタ野郎"
  series:
    # ---- TV Season 1 ----
    - id: rascal-bunny-girl-senpai
      kind: TV
      titles:
        english: "Rascal Does Not Dream of Bunny Girl Senpai"
        romaji:  "Seishun Buta Yarō wa Bunny Girl Senpai no Yume wo Minai"
      airedSeason: 1
      releaseDate: 2018-10-03
      sourceRefs: { anilistId: 101291 }
      timeline:
        - { kind: TV_EPISODE, absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2018-10-03 }
        # … episodes 2–12 elided …
        - { kind: TV_EPISODE, absoluteNumber: 13, airedEpisode: 13, releaseDate: 2018-12-27 }

    # ---- Movie, slotted AFTER S1 by releaseDate ----
    - id: rascal-dreaming-girl
      kind: MOVIE
      titles:
        english: "Rascal Does Not Dream of a Dreaming Girl"
        romaji:  "Seishun Buta Yarō wa Yumemiru Shōjo no Yume wo Minai"
      releaseDate: 2019-06-15
      sourceRefs: { anilistId: 104157 }
      timeline:
        - { kind: MOVIE, absoluteNumber: 14, releaseDate: 2019-06-15 }

    # ---- Movie ----
    - id: rascal-sister-venturing-out
      kind: MOVIE
      titles:
        english: "Rascal Does Not Dream of a Sister Venturing Out"
        romaji:  "Seishun Buta Yarō wa Odekake Sister no Yume wo Minai"
      releaseDate: 2023-06-23
      sourceRefs: { anilistId: 143653 }     # illustrative
      timeline:
        - { kind: MOVIE, absoluteNumber: 15, releaseDate: 2023-06-23 }

    # ---- TV Season 2, slotted AFTER the movies by releaseDate ----
    - id: rascal-season-2
      kind: TV
      titles:
        english: "Rascal Does Not Dream (Season 2)"
        romaji:  "Seishun Buta Yarō (Season 2)"
      airedSeason: 2
      releaseDate: 2025-07-05               # illustrative
      sourceRefs: { anilistId: 162804 }     # illustrative
      timeline:
        - { kind: TV_EPISODE, absoluteNumber: 16, airedEpisode: 1, releaseDate: 2025-07-05 }
        # … further Season 2 episodes continue 17, 18, … …
```

### 2.3 The resulting watch order

| `absoluteNumber` | series (kind) | `airedSeason` | local # | release |
|:--:|---|:--:|:--:|---|
| 1 | Bunny Girl Senpai (TV) | 1 | E1 | 2018-10-03 |
| … | Bunny Girl Senpai (TV) | 1 | … | … |
| 13 | Bunny Girl Senpai (TV) | 1 | E13 | 2018-12-27 |
| 14 | Dreaming Girl (MOVIE) | — | — | 2019-06-15 |
| 15 | Sister Venturing Out (MOVIE) | — | — | 2023-06-23 |
| 16 | Season 2 (TV) | 2 | E1 | 2025-07-05 |
| … | Season 2 (TV) | 2 | … | … |

Sorting by `absoluteNumber` yields the franchise watch order. The two TV seasons get
a **continuous** absolute count even though each restarts its own `airedEpisode` at
1, and the movies fall into place by `releaseDate` — including a **TV season that
airs after the movies**, exactly the sort AniList alone cannot produce.

## 3. Example B — Demon Slayer (the dedup case)

Demon Slayer stresses a different part of the model: **the same story content ships
twice** — *Mugen Train* was a 2020 theatrical movie, then re-cut into a 7-episode TV
series in 2021. The model must pick one canonical `AnimeSeries` so the arc isn't
double-counted.

### 3.1 The AniList view (locally numbered, content overlaps)

| AniList node | kind | local numbering | release | note |
|---|---|---|---|---|
| *Kimetsu no Yaiba* (TV S1) | TV, 26 eps | **1–26** | 2019-04 | AniList 101922 |
| *Mugen Train* (movie) | Movie | — | 2020-10 | AniList 112151 |
| *Mugen Train Arc* (TV) | TV, 7 eps | **1–7** | 2021-10 | **recompiles the movie** (142984) |
| *Entertainment District Arc* | TV, 11 eps | **1–11** | 2021-12 | AniList 142329 |
| *Swordsmith Village Arc* | TV, 11 eps | **1–11** | 2023-04 | AniList 145139 |
| *Hashira Training Arc* | TV, 8 eps | **1–8** | 2024-05 | AniList 166240 |

Two problems at once: (a) every series restarts at episode 1, and (b) *Mugen Train*
appears as both a movie and a TV series covering the same events.

### 3.2 As a `Franchise` record (movie is canonical; TV recompile suppressed)

```yaml
Franchise:
  id: demon-slayer
  titles:
    english: "Demon Slayer: Kimetsu no Yaiba"
    romaji:  "Kimetsu no Yaiba"
    native:  "鬼滅の刃"
  series:
    # ---- Season 1 (26 episodes) → absolute 1–26 ----
    - id: demon-slayer-s1
      kind: TV
      titles: { english: "Demon Slayer", romaji: "Kimetsu no Yaiba" }
      airedSeason: 1
      releaseDate: 2019-04-06
      sourceRefs: { anilistId: 101922 }
      timeline:
        - { kind: TV_EPISODE, absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2019-04-06 }
        # … episodes 2–25 elided …
        - { kind: TV_EPISODE, absoluteNumber: 26, airedEpisode: 26, releaseDate: 2019-09-28 }

    # ---- Mugen Train: the THEATRICAL MOVIE is the canonical carrier → absolute 27 ----
    - id: demon-slayer-mugen-train-movie
      kind: MOVIE
      titles: { english: "Mugen Train", romaji: "Kimetsu no Yaiba: Mugen Ressha-hen" }
      releaseDate: 2020-10-16
      sourceRefs: { anilistId: 112151 }
      timeline:
        - { kind: MOVIE, absoluteNumber: 27, releaseDate: 2020-10-16 }

    # NOTE: the 2021 "Mugen Train Arc" TV series (AniList 142984) is the SAME content.
    # It is dropped via franchise-overrides.yaml so the arc is counted once. Because
    # the dedup is now an AnimeSeries-level decision, we simply omit that one series.

    # ---- Entertainment District Arc (11 eps) → absolute 28–38 ----
    - id: demon-slayer-entertainment-district
      kind: TV
      titles: { english: "Entertainment District Arc", romaji: "Kimetsu no Yaiba: Yūkaku-hen" }
      airedSeason: 2
      releaseDate: 2021-12-05
      sourceRefs: { anilistId: 142329 }
      timeline:
        - { kind: TV_EPISODE, absoluteNumber: 28, airedEpisode: 1, releaseDate: 2021-12-05 }
        # … through absoluteNumber 38 …

    # ---- Swordsmith Village (11) → 39–49, Hashira Training (8) → 50–57 ----
```

### 3.3 Why this exercises the model

| Concern | How the model handles it |
|---|---|
| Series restart at episode 1 | `absoluteNumber` provides the continuous franchise count; `airedEpisode` keeps the local numbers |
| Movie ↔ TV duplicate (*Mugen Train*) | Keep the `kind: MOVIE` series as canonical; **omit the recompiled TV `AnimeSeries`** via `franchise-overrides.yaml` (research note §5.3 step 4) |
| Movie placement in watch order | `releaseDate: 2020-10-16` slots it between S1 (2019) and Entertainment District (2021) |
| Which node to enrich (R2) | Each `AnimeSeries.sourceRefs.anilistId` — one per media node; the excluded recompile is simply not a member series |

This dedup is a **curation override**, not something any upstream file decides for
us — exactly the "thin franchise/ordering layer" the research note recommends owning
(§5.5).

## 4. How these records get built

Maps to the research note §5.3 pipeline, now producing three tiers:

1. **Seed** the `Franchise` and its member `AnimeSeries` from
   `anime-offline-database` clustering → which AniList media ids belong together
   (one `AnimeSeries` per media id).
2. **Order** each TV series' episodes from `anime-list.xml` offsets, then assign
   franchise-wide `absoluteNumber` by walking `series[]` in watch order.
3. **Add movies** as single-`TimelineEntry` `AnimeSeries`, interleaved by `releaseDate`.
4. **Override** the gaps — *suppress Demon Slayer's Mugen Train TV recompile*, fix a
   movie's slot, or correct an `airedSeason` label — in `franchise-overrides.yaml`.
5. **Store** the resolved records next to the existing `internal/db/anime.go` schema.
6. **Refresh** upstream files on a schedule; overrides always win.

## 5. Open questions

- **Movie absolute numbering** — do movies consume an `absoluteNumber` in the same
  sequence as episodes (as shown), or live in a parallel movie index? Shown here as
  one shared sequence so a single sort key drives the whole timeline.
- **Recompiled series** — always prefer the movie as canonical, or prefer the TV
  recompile when it adds footage (the *Mugen Train* TV cut has an extra episode)?
  Currently a per-franchise override decision.
- **`airedSeason` labeling** — when official "seasons" split or merge (Demon Slayer's
  Mugen Train TV vs Entertainment District), the season index is itself an override
  call. Above, Entertainment District is labelled season 2 with the recompile suppressed.
- **Franchise vs. AnimeSeries boundary** — for *Fate*-style umbrellas with several
  distinct stories, where does one `AnimeSeries` end and another begin, and do we
  ever need a level *above* `Franchise`?
- **R3 `episodeTitle`** — left empty here; only populated if curated or sourced from a
  non-commercial build (research note §3.3).
