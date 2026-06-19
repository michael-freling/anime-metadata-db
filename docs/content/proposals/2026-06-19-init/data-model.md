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
[research note](../anime-metadata-research/) into a model with **first-class series
and movies**, and grounds it in two concrete franchises:

- **Rascal Does Not Dream** — two TV seasons plus standalone movies that only sort
  correctly with a franchise-wide absolute number.
- **Demon Slayer** — the hard cases: a film that is an **alternate cut of a TV
  series** (*Mugen Train*), **split-cour** seasons (Part 1 / Part 2 in different
  years), and **standalone movies with no TV series** (the *Infinity Castle* trilogy).

> **Scope.** This model owns *ordering and grouping* (R1). Per-series content (R2)
> stays in AniList; per-episode content (R3) is a known gap. See the research note
> §4. AniList IDs, episode counts, and the 2025+ release details below are
> **illustrative** — seeded/verified from `anime-offline-database` at build time
> (§5.3), not hand-kept.

## 1. The shape: series and movies are both first-class

A *franchise* groups **series** (multi-episode TV/OVA broadcasts) and **movies**
(theatrical films). They are different kinds of thing, so the model keeps them in
separate lists rather than pretending a movie is a one-episode series:

```text
Franchise              umbrella; groups the series + movies of one title
  id                   our stable id (e.g. "demon-slayer")
  titles               { english, romaji, native }
  series[]             AnimeSeries — TV / OVA broadcast units (incl. split-cour parts)
  movies[]             Movie — theatrical films (standalone OR alternate cuts)

AnimeSeries            ONE AniList media node — a single TV/OVA broadcast unit (one cour / part)
  id
  titles               { english, romaji, native }
  airedSeason          int    the franchise's Nth season
  part                 int?   split-cour index within the season (1, 2, …); null if one part
  releaseDate          date   this part's premiere
  sourceRefs           { anilistId, anidbId?, tmdbId?, tvdbId? }   (one media node)
  episodes[]           Episode

Episode                ONE TV episode
  absoluteNumber       int    franchise-wide sort key — spans series AND original movies
  airedEpisode         int    local number within this part/series
  releaseDate          date
  episodeTitle         string?  (R3 — curated / non-commercial TMDB only)

Movie                  ONE AniList media node — a theatrical film
  id
  titles               { english, romaji, native }
  releaseDate          date
  sourceRefs           { anilistId, anidbId?, tmdbId?, tvdbId? }
  absoluteNumber       int?   set ONLY for original movies — its slot in watch order
  altCutOf             { seriesId, episodes }?   set when a TV series is the canonical
                                                 numbering carrier for this film's content
```

### 1.1 How a movie participates in numbering

A movie is one of two things, and that decides whether it takes an `absoluteNumber`:

| Movie kind | Example | `absoluteNumber`? | `altCutOf`? |
|---|---|:--:|:--:|
| **Original** — unique content, no TV equivalent | *Dreaming Girl*, *Infinity Castle* | ✅ its own slot | — |
| **Alternate cut** — same content also airs as a TV series | *Mugen Train* (film) | ❌ none | ✅ → the TV series |

> **Numbering rule.** When content exists as **both** a film and a TV series, the
> **TV series carries the numbers** (it has per-episode granularity), and the film
> sets `altCutOf` and takes no `absoluteNumber`. Original movies — including whole
> trilogies with no TV series — take their own `absoluteNumber` slots, interleaved by
> `releaseDate`.

`absoluteNumber` is assigned franchise-wide by walking every TV episode and every
*original* movie in release order. `airedSeason` / `part` / `airedEpisode` keep the
local broadcast structure; `absoluteNumber` is the single sort key for watch order.

### 1.2 Field reference (selected)

| Field | Entity | Why it exists |
|---|---|---|
| `titles {english,romaji,native}` | Franchise / AnimeSeries / Movie | Multi-name display — *Bunny Girl Senpai* (en) vs *Seishun Buta Yarō* (romaji) |
| `series[]` / `movies[]` | Franchise | Members, typed: TV broadcasts vs films |
| `airedSeason` | AnimeSeries | Which franchise season this is (1, 2, …) |
| `part` | AnimeSeries | Split-cour index — Part 1 / Part 2 of the same season (§4) |
| `sourceRefs.anilistId` | AnimeSeries / Movie | **The media id**, kept once per node — the R2 enrichment key |
| **`absoluteNumber`** | Episode / Movie | **The one field no free API gives us** — franchise-wide sort key |
| `airedEpisode` | Episode | Local per-part episode number (what AniList exposes) |
| `altCutOf` | Movie | Marks a film whose content a TV series numbers canonically |

The model **stores facts** (ids, numbers, dates, our computed `absoluteNumber`) and
**fetches expression** (synopsis, art, stills) live (research note §5.1a).

## 2. Example A — Rascal Does Not Dream (two seasons + movies)

```yaml
Franchise:
  id: rascal-does-not-dream
  titles:
    english: "Rascal Does Not Dream"
    romaji:  "Seishun Buta Yarō"
    native:  "青春ブタ野郎"
  series:
    - id: rascal-bunny-girl-senpai           # TV Season 1
      kind: TV
      titles:
        english: "Rascal Does Not Dream of Bunny Girl Senpai"
        romaji:  "Seishun Buta Yarō wa Bunny Girl Senpai no Yume wo Minai"
      airedSeason: 1
      releaseDate: 2018-10-03
      sourceRefs: { anilistId: 101291 }
      episodes:
        - { absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2018-10-03 }
        # … episodes 2–12 elided …
        - { absoluteNumber: 13, airedEpisode: 13, releaseDate: 2018-12-27 }

    - id: rascal-season-2                     # TV Season 2 — airs AFTER the movies
      kind: TV
      titles:
        english: "Rascal Does Not Dream (Season 2)"
        romaji:  "Seishun Buta Yarō (Season 2)"
      airedSeason: 2
      releaseDate: 2025-07-05                 # illustrative
      sourceRefs: { anilistId: 162804 }       # illustrative
      episodes:
        - { absoluteNumber: 16, airedEpisode: 1, releaseDate: 2025-07-05 }
        # … Season 2 continues 17, 18, … …

  movies:
    - id: rascal-dreaming-girl                # original film → own slot
      titles:
        english: "Rascal Does Not Dream of a Dreaming Girl"
        romaji:  "Seishun Buta Yarō wa Yumemiru Shōjo no Yume wo Minai"
      releaseDate: 2019-06-15
      sourceRefs: { anilistId: 104157 }
      absoluteNumber: 14

    - id: rascal-sister-venturing-out         # original film → own slot
      titles:
        english: "Rascal Does Not Dream of a Sister Venturing Out"
        romaji:  "Seishun Buta Yarō wa Odekake Sister no Yume wo Minai"
      releaseDate: 2023-06-23
      sourceRefs: { anilistId: 143653 }       # illustrative
      absoluteNumber: 15
```

### 2.1 The resulting watch order

| `absoluteNumber` | member (kind) | `airedSeason` | local # | release |
|:--:|---|:--:|:--:|---|
| 1 | Bunny Girl Senpai (TV) | 1 | E1 | 2018-10-03 |
| … | Bunny Girl Senpai (TV) | 1 | … | … |
| 13 | Bunny Girl Senpai (TV) | 1 | E13 | 2018-12-27 |
| 14 | Dreaming Girl (movie) | — | — | 2019-06-15 |
| 15 | Sister Venturing Out (movie) | — | — | 2023-06-23 |
| 16 | Season 2 (TV) | 2 | E1 | 2025-07-05 |
| … | Season 2 (TV) | 2 | … | … |

Two TV seasons get a **continuous** absolute count even though each restarts its
`airedEpisode` at 1, and the original movies interleave by `releaseDate` — including
a TV season that airs *after* the movies.

## 3. Example B — Demon Slayer (alt-cut film + standalone movies)

Demon Slayer exercises all three hard cases at once.

### 3.1 As a `Franchise` record

```yaml
Franchise:
  id: demon-slayer
  titles:
    english: "Demon Slayer: Kimetsu no Yaiba"
    romaji:  "Kimetsu no Yaiba"
    native:  "鬼滅の刃"
  series:
    - id: ds-s1                               # Season 1 → absolute 1–26
      kind: TV
      titles: { english: "Demon Slayer", romaji: "Kimetsu no Yaiba" }
      airedSeason: 1
      releaseDate: 2019-04-06
      sourceRefs: { anilistId: 101922 }
      episodes:
        - { absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2019-04-06 }
        # … through absoluteNumber 26 …

    - id: ds-mugen-train-arc                  # Season 2, Part 1 → absolute 27–33
      kind: TV                                #   THIS carries Mugen Train's numbers
      titles: { english: "Mugen Train Arc", romaji: "Kimetsu no Yaiba: Mugen Ressha-hen" }
      airedSeason: 2
      part: 1
      releaseDate: 2021-10-10
      sourceRefs: { anilistId: 142984 }
      episodes:
        - { absoluteNumber: 27, airedEpisode: 1, releaseDate: 2021-10-10 }
        # … through absoluteNumber 33 (7 episodes) …

    - id: ds-entertainment-district          # Season 2, Part 2 → absolute 34–44
      kind: TV
      titles: { english: "Entertainment District Arc", romaji: "Kimetsu no Yaiba: Yūkaku-hen" }
      airedSeason: 2
      part: 2
      releaseDate: 2021-12-05
      sourceRefs: { anilistId: 142329 }
      episodes:
        - { absoluteNumber: 34, airedEpisode: 1, releaseDate: 2021-12-05 }
        # … through absoluteNumber 44 (11 episodes) …

    # … Swordsmith Village (S3) → 45–55, Hashira Training (S4) → 56–63 …

  movies:
    - id: ds-mugen-train-film                # ALTERNATE CUT — no absoluteNumber
      titles: { english: "Mugen Train", romaji: "Kimetsu no Yaiba: Mugen Ressha-hen" }
      releaseDate: 2020-10-16
      sourceRefs: { anilistId: 112151 }
      altCutOf: { seriesId: ds-mugen-train-arc, episodes: "1-7" }

    - id: ds-infinity-castle-1               # ORIGINAL standalone trilogy — own slots
      titles: { english: "Infinity Castle (Part 1)", romaji: "Mugen Jō-hen" }
      releaseDate: 2025-07-18                 # illustrative
      sourceRefs: { anilistId: 178680 }       # illustrative
      absoluteNumber: 64
    # … Infinity Castle Part 2 → 65, Part 3 → 66 …
```

### 3.2 Why this exercises the model

| Concern | How the model handles it |
|---|---|
| **Mugen Train: film vs TV** | The **TV `ds-mugen-train-arc` carries episodes 27–33**; the film sets `altCutOf` and takes no absolute number — your "use the TV series, not the movie" rule |
| **Standalone movies** (*Infinity Castle*) | First-class `Movie` entries with **no series**, each taking its own `absoluteNumber` (64, 65, 66) interleaved by `releaseDate` |
| **Split-cour S2** | Mugen Train Arc (`part: 1`) and Entertainment District (`part: 2`) share `airedSeason: 2` but have distinct `part` + `releaseDate` (§4) |
| **Seasons restart at episode 1** | `absoluteNumber` is the continuous count; `airedEpisode` keeps the local numbers |

> **Chronology note.** The *Mugen Train* film (2020) actually predates its TV cut
> (2021). We still pick the TV series as the numbering carrier per the rule above; the
> film stays reachable via `altCutOf`, so a *release-date* watch list can still surface
> it. Which view (numbering vs release-date) wins is a per-app choice, not a data one.

## 4. Split-cour: "Part 1 / Part 2" in the same season

Many seasons air in two cours months — or years — apart, often as **separate AniList
nodes** (e.g. *Attack on Titan: The Final Season* Parts 1–3 in 2020/2022/2023, *Re:Zero*
Season 2 Parts 1–2 in 2020/2021, and Demon Slayer's S2 above).

Each part is its **own `AnimeSeries`** — its own `anilistId`, `releaseDate`, and
episode list — tagged with the **same `airedSeason`** and a distinct **`part`**:

```yaml
series:
  - id: show-s2-part1
    airedSeason: 2
    part: 1
    releaseDate: 2020-07-08
    sourceRefs: { anilistId: 11111 }
    episodes:
      - { absoluteNumber: 51, airedEpisode: 1,  releaseDate: 2020-07-08 }
      # … airedEpisode 1..13 …
  - id: show-s2-part2
    airedSeason: 2
    part: 2
    releaseDate: 2022-01-09          # different year
    sourceRefs: { anilistId: 22222 }
    episodes:
      - { absoluteNumber: 64, airedEpisode: 1,  releaseDate: 2022-01-09 }
      # … airedEpisode may continue (14..) or reset (1..) per the broadcast …
```

Rules of thumb:

- A **"season"** is the set of `AnimeSeries` sharing `airedSeason`; `part` orders them.
- `airedEpisode` follows whatever the broadcast used (some split-cours continue the
  count, some reset). `absoluteNumber` is unaffected — it always flows by release order.
- **If the two parts are actually a single AniList node** (one media entry for both
  cours), it's just **one `AnimeSeries`** whose episodes span two air windows — the
  per-episode `releaseDate` already captures the gap, and `part` stays null.

## 5. How these records get built

Maps to the research note §5.3 pipeline:

1. **Seed** the `Franchise`, its `series[]`, and `movies[]` from
   `anime-offline-database` clustering — one `AnimeSeries`/`Movie` per AniList media id.
2. **Order** each series' episodes from `anime-list.xml`, then assign franchise-wide
   `absoluteNumber` across all episodes + original movies in release order.
3. **Slot movies** from `anime-movieset-list.xml`: original films get a number;
   films flagged as alternate cuts get `altCutOf` and no number.
4. **Override** the judgement calls — alt-cut vs original, `airedSeason`/`part`
   labels — in `franchise-overrides.yaml`.
5. **Store** the resolved records next to `internal/db/anime.go`.
6. **Refresh** upstream on a schedule; overrides always win.

## 6. Open questions

- **Original vs alternate-cut detection** — no open file flags this; it's a manual
  `altCutOf` override per film. Acceptable, since it's rare.
- **Do we need an explicit `Season` entity?** Today a season is implied by shared
  `airedSeason`. A real entity would be warranted only if seasons need their own
  titles/art beyond their parts.
- **Numbering vs release-date order** — alt-cut films (Mugen Train) make these differ.
  The data supports both; the app picks which to show.
- **Franchise vs. AnimeSeries boundary** — for *Fate*-style umbrellas with several
  distinct stories, do we ever need a level *above* `Franchise`?
- **R3 `episodeTitle`** — empty here; only populated if curated or from a
  non-commercial build (research note §3.3).
