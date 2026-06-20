---
title: "Franchise Data Model & Examples"
date: 2026-06-19
weight: 2
---

# Anime Series Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)

This note refines the flat `Franchise` / `TimelineEntry` sketch from §5.2 of the
[research note](../anime-metadata-research/). The base unit is a **`Series`**
(one storyline); **`Franchise`** is an *optional* parent that groups several Series.
Worked cases:

- **Gundam** — the optional `Franchise` tier: one brand, many independent Series.
- **Fate** — two **ordering modes** in one franchise (*Fate/stay night* sorts by
  release date, *Fate/Zero* uses curated absolute numbers), plus a curated
  **chronological** watch order across its Series (release is the default).
- **Demon Slayer** — a standalone Series with the numbering edge cases: an
  alternate-cut film, split-cour seasons, and standalone movies.
- **Rascal Does Not Dream** — the basic two-season + movies case.

> **Scope.** This model owns *ordering and grouping* (R1). Per-season content (R2)
> stays in AniList; per-episode content (R3) is a known gap (research note §4).
> AniList IDs, episode counts, and 2024+ release details below are **illustrative** —
> seeded/verified from `anime-offline-database` at build time (§5.3).

## 1. The hierarchy

```text
Franchise (OPTIONAL)   groups related Series under one brand — present only when there are several
  id
  titles               { english, romaji, native }
  series[]             Series
  watchOrders[]        WatchOrder — curated alternate orders, e.g. chronological (§7); release is the default

Series                 the base unit: ONE storyline / continuity (Demon Slayer, Fate/Zero)
  id
  titles               { english, romaji, native }
  ordering             ABSOLUTE — curated absoluteNumber │ RELEASE_DATE — sort members by date
  seasons[]            Season — the numbered TV installments of this storyline
  movies[]             Movie — films belonging to this storyline
  specials[]           Special — OVAs / ONAs / specials (side content, no seasonNumber)

Season                 ONE numbered TV installment = one AniList media node (a TV cour / part)
  id
  titles               { english, romaji, native }
  seasonNumber         int    the storyline's Nth season
  part                 int?   split-cour index within the season (1, 2, …); null if one part
  releaseDate          date
  sourceRefs           { anilistId, anidbId?, tmdbId?, tvdbId? }   (one media node)
  episodes[]           Episode

Episode                ONE TV episode
  absoluteNumber       int?   ABSOLUTE-ordered Series only — sort key across the storyline
  airedEpisode         int    local number within this season/part
  releaseDate          date
  episodeTitle         string?  (R3 — curated / non-commercial TMDB only)

Movie                  ONE film = one AniList media node
  id
  titles               { english, romaji, native }
  releaseDate          date
  sourceRefs           { anilistId, … }
  absoluteNumber       int?   original films in an ABSOLUTE-ordered Series — its slot in watch order
  altCutOf             { seasonId, episodes }?   set when a Season carries this film's numbers

Special                ONE OVA / ONA / special = one AniList media node — side content
  id                   NOT part of the numbered run, so it has NO seasonNumber
  titles               { english, romaji, native }
  format               OVA | ONA | SPECIAL
  releaseDate          date
  sourceRefs           { anilistId, … }
  episodes[]           Episode    (an OVA series may have several; a one-shot has one)
  absoluteNumber       int?   only if it's canon you want pinned into an ABSOLUTE watch order

WatchOrder             a NAMED curated alternate order across a Franchise's Series (§7)
  name                 e.g. "Chronological" — an objective order NOT derivable from release dates
  entries[]            ordered refs: { ref: <series|season|movie id>, note? }
```

A **`Series`** is the thing you actually watch as one continuity. A single-storyline
title (Demon Slayer, Rascal) is just a top-level `Series`. A brand with several
independent storylines (Gundam, Fate, iDOLM@STER) is a `Franchise` wrapping those
`Series`. Below a Series: **Seasons** (the numbered TV run), **Movies**, and
**Specials** (OVAs); a Season holds **Episodes**.

### 1.1 Ordering: `ABSOLUTE` vs `RELEASE_DATE`

`absoluteNumber` only makes sense when a storyline is a single linear episode stream,
so it is **optional** and chosen per Series via `ordering`:

- **`ABSOLUTE`** — a clean linear watch order exists → curate a continuous
  `absoluteNumber` across the Series' episodes + original movies (Demon Slayer, Rascal,
  *Fate/Zero*). The costly R1 data — assign it only where it pays off.
- **`RELEASE_DATE`** — no single episode stream (parallel-route adaptations) → do **not**
  fabricate `absoluteNumber`; the app sorts the Series' seasons/movies/specials by
  `releaseDate` (*Fate/stay night*).

For *Fate/stay night*, `RELEASE_DATE` yields Fate route (2006) → Unlimited Blade Works
(2014) → Heaven's Feel (2017) — also the intended watch order, since the Saber-route
adaptation aired first.

`ordering` is the *micro* order **within** one Series. The *macro* order **across**
Series is a separate concern handled by the franchise's `watchOrders` (§7) — they layer.

### 1.2 Numbering rules (ABSOLUTE series)

- **`absoluteNumber` is scoped to a Series.** *Fate/Zero* numbers independently of
  *Fate/stay night*; Demon Slayer's Series numbers 1…63+.
- **Movies:** an *original* film takes its own `absoluteNumber`; an *alternate-cut* film
  whose content also airs as a Season sets `altCutOf` and takes **no** number — the
  **Season carries the numbers** (per-episode granularity).
- **Split-cour:** Part 1 / Part 2 of a season are separate `Season`s sharing
  `seasonNumber`, differing by `part` + `releaseDate` (§5).
- **OVAs / specials:** side content in `specials[]`, placed by `releaseDate` with **no
  `seasonNumber`**; given an `absoluteNumber` only when pinned into the canon watch order.

### 1.3 Field reference (selected)

| Field | Entity | Why it exists |
|---|---|---|
| `series[]` | Franchise (optional) | The distinct storylines of a multi-story brand (Gundam, Fate) |
| `ordering` | Series | `ABSOLUTE` (curated numbers) vs `RELEASE_DATE` (sort by date) — *within* a Series |
| `watchOrders[]` | Franchise | Curated alternate orders across its Series, e.g. chronological (release is the default) — §7 |
| `titles {english,romaji,native}` | all named entities | *Bunny Girl Senpai* (en) vs *Seishun Buta Yarō* (romaji) |
| `seasons[]` / `movies[]` / `specials[]` | Series | Members: numbered TV run, films, OVAs/specials |
| `seasonNumber` / `part` | Season | Season index, and split-cour part within it (§5) |
| `sourceRefs.anilistId` | Season / Movie / Special | **The media id**, once per node — the R2 enrichment key |
| **`absoluteNumber`** | Episode / Movie | **The one field no free API gives us** — sort key in an ABSOLUTE Series |
| `altCutOf` | Movie | Marks a film a Season numbers canonically |

The model **stores facts** (ids, numbers, dates, our `absoluteNumber`) and **fetches
expression** (synopsis, art, stills) live (research note §5.1a).

## 2. Example A — Gundam (the optional `Franchise` tier)

Gundam is the textbook multi-storyline brand: independent continuities sharing only the
name, each numbering on its own. This is *why* `Franchise` exists.

```yaml
Franchise:                                   # present only because Gundam has many storylines
  id: gundam
  titles: { english: "Gundam", romaji: "Gundam" }
  series:
    - id: gundam-uc                          # Universal Century — one big linear continuity
      titles: { english: "Mobile Suit Gundam (Universal Century)" }
      ordering: ABSOLUTE
      seasons: [ "0079 → Zeta → ZZ → …" ]
      movies:  [ "Char's Counterattack, F91, …" ]
    - id: gundam-wing                        # After Colony — independent
      titles: { english: "Mobile Suit Gundam Wing" }
      ordering: ABSOLUTE
      movies:  [ "Endless Waltz" ]
    - id: gundam-seed                        # Cosmic Era — independent
      titles: { english: "Mobile Suit Gundam SEED" }
      ordering: ABSOLUTE
      seasons: [ "SEED, SEED Destiny" ]
      movies:  [ "SEED Freedom (2024)" ]
    - id: gundam-ibo                         # Post Disaster — independent
      titles: { english: "Mobile Suit Gundam: Iron-Blooded Orphans" }
      ordering: ABSOLUTE
    - id: gundam-witch                       # Ad Stella — independent
      titles: { english: "The Witch from Mercury" }
      ordering: ABSOLUTE
```

Each Series is a self-contained watch order — there is **no** franchise-wide
`absoluteNumber` across Wing and SEED. The `Franchise` is grouping + titling only.
(Other brands of this shape: *iDOLM@STER*, *Love Live!*, *Precure*, *Yu-Gi-Oh!*,
*Macross*, *Digimon*.)

## 3. Example B — Fate (two ordering modes in one franchise)

```yaml
Franchise:
  id: fate
  titles: { english: "Fate", native: "フェイト" }
  series:
    - id: fate-stay-night
      titles: { english: "Fate/stay night" }
      ordering: RELEASE_DATE                 # parallel routes → sort by date, NO absoluteNumber
      seasons:
        - { id: fsn-2006, titles: { english: "Fate/stay night (2006)" }, seasonNumber: 1,
            releaseDate: 2006-01-07, sourceRefs: { anilistId: 356 } }       # Fate/Saber route
        - { id: fsn-ubw,  titles: { english: "Unlimited Blade Works" }, seasonNumber: 2,
            releaseDate: 2014-10-12, sourceRefs: { anilistId: 20716 } }     # UBW route (itself split-cour)
      movies:
        - { id: fsn-hf-1, titles: { english: "Heaven's Feel I" }, releaseDate: 2017-10-14,
            sourceRefs: { anilistId: 20724 } }     # no absoluteNumber under RELEASE_DATE
        # … Heaven's Feel II (2019-01), III (2020-08) …

    - id: fate-zero
      titles: { english: "Fate/Zero" }
      ordering: ABSOLUTE                      # single linear story (split-cour) → curated numbers
      seasons:
        - { id: fz-s1, seasonNumber: 1, part: 1, releaseDate: 2011-10-02,
            sourceRefs: { anilistId: 10087 }, episodes: [ "… absolute 1–13 …" ] }
        - { id: fz-s2, seasonNumber: 1, part: 2, releaseDate: 2012-04-08,
            sourceRefs: { anilistId: 11741 }, episodes: [ "… absolute 14–25 …" ] }
```

- **`Fate/stay night` → `RELEASE_DATE`.** The 2006 route, UBW, and Heaven's Feel adapt
  different visual-novel routes — not a linear sequence — so no `absoluteNumber`. Sorting
  by `releaseDate` gives Fate route → UBW → Heaven's Feel, the intended watch order.
- **`Fate/Zero` → `ABSOLUTE`.** A single linear story (just split across two cours), so
  its episodes carry continuous `absoluteNumber` 1–25.
- Each Series numbers (or doesn't) on its own; the `Franchise` only groups them.

## 4. Example C — Demon Slayer (standalone Series, numbering mechanics)

Demon Slayer is a single storyline, so it's a **top-level `Series`** — no `Franchise`
wrapper.

```yaml
Series:
  id: demon-slayer
  titles: { english: "Demon Slayer: Kimetsu no Yaiba", romaji: "Kimetsu no Yaiba", native: "鬼滅の刃" }
  ordering: ABSOLUTE
  seasons:
    - id: ds-s1                               # → absolute 1–26
      seasonNumber: 1
      releaseDate: 2019-04-06
      sourceRefs: { anilistId: 101922 }
      episodes:
        - { absoluteNumber: 1,  airedEpisode: 1,  releaseDate: 2019-04-06 }
        # … through 26 …
    - id: ds-mugen-train-arc                  # Season 2 Part 1 → absolute 27–33
      titles: { english: "Mugen Train Arc" }  #   THIS carries Mugen Train's numbers
      seasonNumber: 2
      part: 1
      releaseDate: 2021-10-10
      sourceRefs: { anilistId: 142984 }
      episodes:
        - { absoluteNumber: 27, airedEpisode: 1, releaseDate: 2021-10-10 }
        # … through 33 (7 eps) …
    - id: ds-entertainment-district           # Season 2 Part 2 → absolute 34–44
      titles: { english: "Entertainment District Arc" }
      seasonNumber: 2
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
      altCutOf: { seasonId: ds-mugen-train-arc, episodes: "1-7" }
    - id: ds-infinity-castle-1                # ORIGINAL standalone trilogy → own slots
      titles: { english: "Infinity Castle (Part 1)", romaji: "Mugen Jō-hen" }
      releaseDate: 2025-07-18                  # illustrative
      sourceRefs: { anilistId: 178680 }        # illustrative
      absoluteNumber: 64
    # … Infinity Castle Part 2 → 65, Part 3 → 66 …
```

| Concern | How the model handles it |
|---|---|
| **Mugen Train: film vs TV** | The Season `ds-mugen-train-arc` carries episodes 27–33; the film sets `altCutOf` and takes no number — "use the TV series, not the movie" |
| **Standalone movies** (*Infinity Castle*) | First-class `Movie` with no season, each taking its own `absoluteNumber` (64–66) |
| **Split-cour S2** | Mugen Train Arc (`part: 1`) + Entertainment District (`part: 2`) share `seasonNumber: 2` |
| **Seasons restart at episode 1** | `absoluteNumber` is the continuous count; `airedEpisode` keeps local numbers |

> **Chronology note.** The *Mugen Train* film (2020) predates its TV cut (2021). We still
> pick the Season as the numbering carrier; the film stays reachable via `altCutOf`.

## 5. Split-cour: "Part 1 / Part 2" in the same season

Many seasons air in two cours months — or years — apart, often as **separate AniList
nodes** (*Attack on Titan: The Final Season* Parts 1–3; *Re:Zero* S2; *Fate/Zero* and
Demon Slayer S2 above). Each part is its own `Season` sharing `seasonNumber`, differing
by `part` + `releaseDate`:

```yaml
seasons:
  - { id: show-s2-part1, seasonNumber: 2, part: 1, releaseDate: 2020-07-08,
      sourceRefs: { anilistId: 11111 }, episodes: [ "… airedEpisode 1..13 …" ] }
  - { id: show-s2-part2, seasonNumber: 2, part: 2, releaseDate: 2022-01-09,   # different year
      sourceRefs: { anilistId: 22222 }, episodes: [ "… airedEpisode may continue or reset …" ] }
```

- A broadcast **"season"** is the set of `Season`s sharing `seasonNumber`; `part` orders
  them. (So `seasonNumber` is *not* unique per `Season` — `seasonNumber` + `part` is.)
- `airedEpisode` follows the broadcast; `absoluteNumber` is unaffected — it flows by
  release order.
- **If both cours are a single AniList node**, it's one `Season` whose episodes span two
  air windows — `releaseDate` captures the gap and `part` stays null.

## 6. Example D — Rascal Does Not Dream (basic two seasons + movies)

The motivating case: a standalone `Series` (`ordering: ABSOLUTE`), two `Season`s,
original movies interleaved by `releaseDate`.

| `absoluteNumber` | member (kind) | `seasonNumber` | release |
|:--:|---|:--:|---|
| 1–13 | Bunny Girl Senpai (Season) | 1 | 2018-10 … 12 |
| 14 | Dreaming Girl (movie) | — | 2019-06-15 |
| 15 | Sister Venturing Out (movie) | — | 2023-06-23 |
| 16… | Rascal Does Not Dream of Santa Claus (Season) | 2 | 2025-07 (illustrative) |

Season 2 is *Rascal Does Not Dream of Santa Claus* (romaji *Seishun Buta Yarō wa Santa
Claus no Yume wo Minai*). Two seasons get a continuous absolute count even though each
restarts `airedEpisode` at 1, and the movies interleave by release date.

## 7. Cross-Series orders: `Franchise.watchOrders`

`absoluteNumber` orders a single linear storyline. It cannot express how *Series* relate
across a franchise. *Fate* (§3) has two such orders, and crucially **both are objective
facts, not opinions**:

- **Release** — by air date: stay night (2006) → Fate/Zero (2011) → UBW (2014) → Heaven's
  Feel (2017…). Fully **derivable** by sorting on `releaseDate`, so it is the **default**
  and is never stored.
- **Chronological** — by in-universe timeline: the prequel first, Fate/Zero → Fate/stay
  night. An objective fact about the story, but **not** in release dates or any free file,
  so it is the order that must be recorded.

Subjective "best newcomer path" recommendations are deliberately **out of scope**. So the
only thing stored is the non-derivable order(s) — release is implicit:

```yaml
Franchise:
  id: fate
  series: [ … ]                               # as in §3
  watchOrders:
    - name: "Chronological"                   # in-universe; curated, not derivable
      entries: [ { ref: fate-zero }, { ref: fate-stay-night } ]
  # Release order is the default — derived from releaseDate, never stored.
```

How it composes:

- **Macro vs micro.** A `watchOrder` sequences whole Series / Seasons / Movies; *inside*
  each referenced node you fall back to that Series' own `ordering` (absoluteNumber or
  release date). The watch order = the cross-node order; `absoluteNumber` = the
  within-Series order. They layer, they don't compete.
- **Mixed granularity.** An entry can point at a whole Series (*all of Fate/Zero*), a
  single Season (*UBW*), or a Movie — whatever the order needs.
- **Lives under the Franchise.** Cross-Series order only exists when a brand has several
  Series; a standalone `Series` just uses its own `ordering`. Release is the default
  everywhere (derived from `releaseDate`); only curated alternates like chronological are stored.

## 8. How these records get built

Maps to the research note §5.3 pipeline:

1. **Seed** the `Series` (and an optional `Franchise` when a brand has several) plus each
   Series' `seasons[]`/`movies[]`/`specials[]` from `anime-offline-database`, bucketed by
   AniList `format` (TV → Season, MOVIE → Movie, OVA/ONA/SPECIAL → Special).
2. **Pick `ordering`** per Series; for `ABSOLUTE`, order episodes from `anime-list.xml`
   and assign `absoluteNumber` across episodes + original movies in release order.
3. **Slot movies** from `anime-movieset-list.xml`: original films get a number (ABSOLUTE
   only); alternate cuts get `altCutOf` and none.
4. **Override** the judgement calls — Series/Franchise boundaries, `ordering` mode,
   alt-cut vs original, `seasonNumber`/`part`, and any cross-Series `WatchOrder`s — in
   `franchise-overrides.yaml`.
5. **Store** next to `internal/db/anime.go`; **refresh** on a schedule, overrides win.

## 9. Open questions

- **Unify ordering?** Within-Series order (`absoluteNumber`) and cross-Series order
  (`watchOrders`) are two mechanisms. Keep both — number as the cheap materialized path,
  watch order as the curated one — or express everything as watch orders? Kept separate
  here so the common case stays a simple integer sort.
- **Picking the order** — release is the default; do users opt into a stored alternate
  (chronological) per session, and is that a catalog-wide setting or a per-user preference?
- **Alternate order for a standalone Series** — a Series outside a Franchise has only its
  own `ordering`. If one ever needs a second objective order (e.g. *Monogatari* broadcast
  vs chronological), do we wrap it in a degenerate one-Series Franchise, or revisit then?
- **Original vs alternate-cut detection** — no open file flags this; a manual `altCutOf`
  override per film.
- **OVA / special placement** — by `releaseDate` as side content (default), or pinned
  into the watch order with an `absoluteNumber` via override when an OVA is canon?
- **R3 `episodeTitle`** — empty here; only populated if curated or from a non-commercial
  build (research note §3.3).
