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
[research note](../anime-metadata-research/) into a four-tier grouping that can
express everything from a single-story franchise to *Fate*. It is grounded in three
cases:

- **Fate** — multi-storyline grouping: one franchise, several distinct **Series**,
  each with its own anime + movies (including parallel-route adaptations).
- **Demon Slayer** — the numbering mechanics: an **alternate-cut film** (*Mugen
  Train*), **split-cour** seasons, and **standalone movies** (*Infinity Castle*).
- **Rascal Does Not Dream** — the basic two-season + movies case.

> **Scope.** This model owns *ordering and grouping* (R1). Per-anime content (R2)
> stays in AniList; per-episode content (R3) is a known gap (research note §4).
> AniList IDs, episode counts, and 2025+ release details below are **illustrative** —
> seeded/verified from `anime-offline-database` at build time (§5.3).

## 1. The hierarchy

```text
Franchise            brand umbrella; holds one or MANY Series (this is why it's a "franchise")
  id
  titles             { english, romaji, native }
  series[]           Series

Series               ONE storyline / continuity (Fate/stay night, Fate/Zero, Demon Slayer)
  id                 absoluteNumber is scoped to a Series, not the whole franchise
  titles             { english, romaji, native }
  seasons[]          AnimeSeries — the TV/OVA anime that make up this storyline
  movies[]           Movie — films belonging to this storyline

AnimeSeries          ONE produced anime = one AniList media node (a TV cour / part / OVA)
  id
  titles             { english, romaji, native }
  airedSeason        int    the storyline's Nth season
  part               int?   split-cour index within the season (1, 2, …); null if one part
  releaseDate        date
  sourceRefs         { anilistId, anidbId?, tmdbId?, tvdbId? }   (one media node)
  episodes[]         Episode

Episode              ONE TV episode
  absoluteNumber     int    sort key within its Series — spans that storyline's anime + original movies
  airedEpisode       int    local number within this part
  releaseDate        date
  episodeTitle       string?  (R3 — curated / non-commercial TMDB only)

Movie                ONE film = one AniList media node
  id
  titles             { english, romaji, native }
  releaseDate        date
  sourceRefs         { anilistId, … }
  absoluteNumber     int?   original films only — their slot in the Series watch order
  altCutOf           { animeSeriesId, episodes }?   set when a TV anime is the canonical
                                                    numbering carrier for this film's content
```

> **Series vs AnimeSeries.** A **Series** is a *storyline* (*Fate/stay night*). An
> **AnimeSeries** is *one produced anime within it* (*Unlimited Blade Works*). A
> single-story franchise like Demon Slayer is one `Franchise` → one `Series` → many
> `AnimeSeries`; *Fate* is one `Franchise` → many `Series`.

### 1.1 Numbering rules

- **`absoluteNumber` is scoped to a Series.** *Fate/Zero* and *Fate/stay night* number
  independently; Demon Slayer's single Series numbers 1…63+.
- **Movies:** an *original* film (unique content) takes its own `absoluteNumber`; an
  *alternate-cut* film whose content also airs as a TV anime sets `altCutOf` and takes
  **no** number — **the TV anime carries the numbers** (per-episode granularity).
- **Split-cour:** Part 1 / Part 2 of a season are separate `AnimeSeries` sharing
  `airedSeason`, differing by `part` + `releaseDate` (§4).

### 1.2 Field reference (selected)

| Field | Entity | Why it exists |
|---|---|---|
| `titles {english,romaji,native}` | Franchise / Series / AnimeSeries / Movie | Multi-name display — *Bunny Girl Senpai* (en) vs *Seishun Buta Yarō* (romaji) |
| `series[]` | Franchise | The distinct storylines (1 for Demon Slayer, many for Fate) |
| `seasons[]` / `movies[]` | Series | Members of a storyline, typed: TV anime vs films |
| `airedSeason` / `part` | AnimeSeries | Season index, and split-cour part within it (§4) |
| `sourceRefs.anilistId` | AnimeSeries / Movie | **The media id**, once per node — the R2 enrichment key |
| **`absoluteNumber`** | Episode / Movie | **The one field no free API gives us** — sort key within a Series |
| `altCutOf` | Movie | Marks a film a TV anime numbers canonically |

The model **stores facts** (ids, numbers, dates, our `absoluteNumber`) and **fetches
expression** (synopsis, art, stills) live (research note §5.1a).

## 2. Example A — Fate (one franchise, many series)

*Fate* is the case that forces the `Series` tier: one franchise containing several
distinct storylines, each with its own anime and films.

```yaml
Franchise:
  id: fate
  titles: { english: "Fate", native: "フェイト" }
  series:
    - id: fate-stay-night                         # storyline 1
      titles: { english: "Fate/stay night", romaji: "Fate/stay night" }
      seasons:
        - id: fsn-2006
          titles: { english: "Fate/stay night (2006)" }       # Studio DEEN, Fate route
          airedSeason: 1
          releaseDate: 2006-01-07
          sourceRefs: { anilistId: 356 }                      # illustrative
          episodes: [ "… 24 eps …" ]
        - id: fsn-unlimited-blade-works
          titles: { english: "Unlimited Blade Works", romaji: "Unlimited Blade Works" }
          airedSeason: 2                                       # UBW route; itself split-cour
          part: 1
          releaseDate: 2014-10-12
          sourceRefs: { anilistId: 20716 }
          episodes: [ "… part 1 …" ]
        - id: fsn-ubw-part2
          titles: { english: "Unlimited Blade Works (Part 2)" }
          airedSeason: 2
          part: 2
          releaseDate: 2015-04-05
          sourceRefs: { anilistId: 21001 }                    # illustrative
          episodes: [ "… part 2 …" ]
      movies:                                                 # Heaven's Feel route = a film trilogy
        - { id: fsn-hf-1, titles: { english: "Heaven's Feel I" }, releaseDate: 2017-10-14,
            sourceRefs: { anilistId: 20724 }, absoluteNumber: 1 }
        - { id: fsn-hf-2, titles: { english: "Heaven's Feel II" }, releaseDate: 2019-01-12,
            sourceRefs: { anilistId: 100173 }, absoluteNumber: 2 }   # illustrative
        - { id: fsn-hf-3, titles: { english: "Heaven's Feel III" }, releaseDate: 2020-08-15,
            sourceRefs: { anilistId: 106562 }, absoluteNumber: 3 }   # illustrative

    - id: fate-zero                               # storyline 2 (prequel) — numbers on its own
      titles: { english: "Fate/Zero", romaji: "Fate/Zero" }
      seasons:
        - id: fz-s1
          titles: { english: "Fate/Zero" }
          airedSeason: 1
          part: 1
          releaseDate: 2011-10-02
          sourceRefs: { anilistId: 10087 }
          episodes: [ "… season 1 …" ]
        - id: fz-s2
          titles: { english: "Fate/Zero Season 2" }
          airedSeason: 1
          part: 2                                             # split-cour, 2012
          releaseDate: 2012-04-08
          sourceRefs: { anilistId: 11741 }                    # illustrative
          episodes: [ "… season 2 …" ]
```

What this demonstrates:

- **The `Series` tier exists.** *Fate/stay night* and *Fate/Zero* are siblings under
  one `Franchise`, each grouping its own anime + films.
- **Parallel adaptations.** Within *Fate/stay night*, the 2006 route, *Unlimited Blade
  Works*, and *Heaven's Feel* adapt **different visual-novel routes** — they are *not* a
  linear sequence. So a single `absoluteNumber` across the whole Series does **not**
  apply; numbering is per linear run (see Open Questions). Here the Heaven's Feel
  trilogy numbers 1–3 among themselves; the routes are grouping-only.

## 3. Example B — Demon Slayer (numbering mechanics)

One `Franchise` → one `Series` → the numbering edge cases.

```yaml
Franchise:
  id: demon-slayer
  titles: { english: "Demon Slayer: Kimetsu no Yaiba", romaji: "Kimetsu no Yaiba", native: "鬼滅の刃" }
  series:
    - id: demon-slayer-main
      titles: { english: "Demon Slayer", romaji: "Kimetsu no Yaiba" }
      seasons:
        - id: ds-s1                               # → absolute 1–26
          airedSeason: 1
          releaseDate: 2019-04-06
          sourceRefs: { anilistId: 101922 }
          episodes:
            - { absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2019-04-06 }
            # … through 26 …
        - id: ds-mugen-train-arc                  # Season 2 Part 1 → absolute 27–33
          titles: { english: "Mugen Train Arc" }  #   THIS carries Mugen Train's numbers
          airedSeason: 2
          part: 1
          releaseDate: 2021-10-10
          sourceRefs: { anilistId: 142984 }
          episodes:
            - { absoluteNumber: 27, airedEpisode: 1, releaseDate: 2021-10-10 }
            # … through 33 (7 eps) …
        - id: ds-entertainment-district           # Season 2 Part 2 → absolute 34–44
          titles: { english: "Entertainment District Arc" }
          airedSeason: 2
          part: 2
          releaseDate: 2021-12-05
          sourceRefs: { anilistId: 142329 }
          episodes:
            - { absoluteNumber: 34, airedEpisode: 1, releaseDate: 2021-12-05 }
            # … through 44 (11 eps); Swordsmith Village (S3) 45–55, Hashira Training (S4) 56–63 …
      movies:
        - id: ds-mugen-train-film                 # ALTERNATE CUT — no absoluteNumber
          titles: { english: "Mugen Train" }
          releaseDate: 2020-10-16
          sourceRefs: { anilistId: 112151 }
          altCutOf: { animeSeriesId: ds-mugen-train-arc, episodes: "1-7" }
        - id: ds-infinity-castle-1                # ORIGINAL standalone trilogy → own slots
          titles: { english: "Infinity Castle (Part 1)", romaji: "Mugen Jō-hen" }
          releaseDate: 2025-07-18                  # illustrative
          sourceRefs: { anilistId: 178680 }        # illustrative
          absoluteNumber: 64
        # … Infinity Castle Part 2 → 65, Part 3 → 66 …
```

| Concern | How the model handles it |
|---|---|
| **Mugen Train: film vs TV** | The TV `ds-mugen-train-arc` carries episodes 27–33; the film sets `altCutOf` and takes no number — "use the TV series, not the movie" |
| **Standalone movies** (*Infinity Castle*) | First-class `Movie` with no anime, each taking its own `absoluteNumber` (64–66) |
| **Split-cour S2** | Mugen Train Arc (`part: 1`) + Entertainment District (`part: 2`) share `airedSeason: 2` |
| **Seasons restart at episode 1** | `absoluteNumber` is the continuous count; `airedEpisode` keeps local numbers |

> **Chronology note.** The *Mugen Train* film (2020) predates its TV cut (2021). We
> still pick the TV anime as the numbering carrier; the film stays reachable via
> `altCutOf`, so a *release-date* watch list can still surface it. Numbering-order vs
> release-order is a per-app choice, not a data one.

## 4. Split-cour: "Part 1 / Part 2" in the same season

Many seasons air in two cours months — or years — apart, often as **separate AniList
nodes** (*Attack on Titan: The Final Season* Parts 1–3; *Re:Zero* S2; *Fate/Zero* and
Demon Slayer S2 above). Each part is its own `AnimeSeries` sharing `airedSeason`,
differing by `part` + `releaseDate`:

```yaml
seasons:
  - { id: show-s2-part1, airedSeason: 2, part: 1, releaseDate: 2020-07-08,
      sourceRefs: { anilistId: 11111 }, episodes: [ "… airedEpisode 1..13 …" ] }
  - { id: show-s2-part2, airedSeason: 2, part: 2, releaseDate: 2022-01-09,   # different year
      sourceRefs: { anilistId: 22222 }, episodes: [ "… airedEpisode may continue or reset …" ] }
```

- A **"season"** is the set of `AnimeSeries` sharing `airedSeason`; `part` orders them.
- `airedEpisode` follows the broadcast (some split-cours continue the count, some reset);
  `absoluteNumber` is unaffected — it always flows by release order.
- **If both cours are a single AniList node**, it's one `AnimeSeries` whose episodes span
  two air windows — `releaseDate` captures the gap and `part` stays null.

## 5. Example C — Rascal Does Not Dream (basic two seasons + movies)

The motivating case from the research note: one `Series`, two TV seasons, original
movies interleaved by `releaseDate`.

| `absoluteNumber` | member (kind) | `airedSeason` | release |
|:--:|---|:--:|---|
| 1–13 | Bunny Girl Senpai (TV) | 1 | 2018-10 … 12 |
| 14 | Dreaming Girl (movie) | — | 2019-06-15 |
| 15 | Sister Venturing Out (movie) | — | 2023-06-23 |
| 16… | Season 2 (TV) | 2 | 2025-07 (illustrative) |

Two TV seasons get a continuous absolute count even though each restarts `airedEpisode`
at 1, and the original movies interleave by release date — including a season that airs
*after* the movies. Structurally identical to Demon Slayer's single Series, minus the
alt-cut and standalone-movie wrinkles.

## 6. How these records get built

Maps to the research note §5.3 pipeline:

1. **Seed** the `Franchise`, its `Series`, and each Series' `seasons[]`/`movies[]` from
   `anime-offline-database` clustering — one node per AniList media id.
2. **Order** each anime's episodes from `anime-list.xml`, then assign `absoluteNumber`
   per Series across its episodes + original movies in release order.
3. **Slot movies** from `anime-movieset-list.xml`: original films get a number;
   alternate cuts get `altCutOf` and none.
4. **Override** the judgement calls — Series boundaries, alt-cut vs original,
   `airedSeason`/`part` labels — in `franchise-overrides.yaml`.
5. **Store** next to `internal/db/anime.go`; **refresh** on a schedule, overrides win.

## 7. Open questions

- **Parallel-route ordering** — *Fate/stay night*'s routes (2006 / UBW / Heaven's Feel)
  aren't linear, so one `absoluteNumber` per Series breaks. Options: number per linear
  run (per route), or treat such a Series as grouping-only with no franchise number.
  Modeled here as the latter.
- **Single-story franchises** — Demon Slayer/Rascal have a `Franchise` wrapping exactly
  one `Series`. Acceptable boilerplate, or collapse the two when there's one storyline?
- **Original vs alternate-cut detection** — no open file flags this; a manual `altCutOf`
  override per film.
- **Do we need a `Season` entity?** Today a season is implied by shared `airedSeason`;
  only worth a real entity if seasons need their own titles/art beyond their parts.
- **R3 `episodeTitle`** — empty here; only populated if curated or from a non-commercial
  build (research note §3.3).
