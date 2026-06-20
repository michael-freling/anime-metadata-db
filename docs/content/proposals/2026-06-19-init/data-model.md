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
[research note](../anime-metadata-research/) into a small hierarchy —
**`Franchise → Series → Season → Episode`**, plus `Movie`, `Special`, and `WatchOrder`.
**Part 1** defines the entities, **Part 2** the rules that aren't obvious from the shape,
**Part 3** grounds it all in four worked franchises.

> **Scope.** This model owns *ordering and grouping* (R1). Per-season content (R2) stays in
> AniList; per-episode content (R3) is a known gap (research note §4). AniList IDs, episode
> counts, and 2024+ release details below are **illustrative** — seeded/verified from
> `anime-offline-database` at build time (§4).

---

## Part 1 — The model

### 1.1 Entities

```text
Franchise (OPTIONAL)   groups related Series under one brand — present only when there are several
  id
  titles               { english, romaji, native }
  series[]             Series
  watchOrders[]        WatchOrder — curated alternate orders, e.g. chronological (§2.5); release is the default

Series                 the base unit: ONE storyline / continuity (Demon Slayer, Fate/Zero)
  id
  titles               { english, romaji, native }
  seasons[]            Season — the numbered TV installments of this storyline
  movies[]             Movie — films belonging to this storyline
  specials[]           Special — OVAs / ONAs / specials (side content, no season number)

Season                 ONE numbered TV installment = one AniList media node (a TV cour / part)
  id
  titles               { english, romaji, native }
  number               int    the storyline's Nth season
  part                 int?   split-cour index within the season (1, 2, …); null if one part
  releaseDate          date
  releaseYear          int    premiere year, e.g. 2012   (the airing "season" — §2.4)
  releaseSeason        enum   WINTER | SPRING | SUMMER | FALL   (the airing quarter, e.g. Spring)
  externalIds          { anilistId, anidbId?, tmdbId?, tvdbId? }   (one media node)
  episodes[]           Episode

Episode                ONE TV episode
  absoluteNumber       int?   present only for numbered series — sort key across the storyline
  airedNumber          int    local number within this season/part
  releaseDate          date
  title                string?  (R3 — curated / non-commercial TMDB only)

Movie                  ONE film = one AniList media node
  id
  titles               { english, romaji, native }
  releaseDate          date
  releaseYear          int
  externalIds          { anilistId, … }
  absoluteNumber       int?   original films in a numbered series — its slot in watch order
  alternateCutOf       { seasonId, episodes }?   set when a TV Season, not this film,
                                                 is the canonical numbering carrier for the content

Special                ONE OVA / ONA / special = one AniList media node — side content
  id                   NOT part of the numbered run, so it has NO season number
  titles               { english, romaji, native }
  format               OVA | ONA | SPECIAL
  releaseDate          date
  releaseYear          int
  externalIds          { anilistId, … }
  episodes[]           Episode    (an OVA series may have several; a one-shot has one)
  absoluteNumber       int?   only if it's canon you want pinned into the numbered watch order

WatchOrder             a NAMED curated alternate order across a Franchise's Series (§2.5)
  name                 e.g. "Chronological" — an objective order NOT derivable from release dates
  entries[]            ordered refs: { ref: <series|season|movie id>, note? }
```

A **`Series`** is the thing you actually watch as one continuity. A single-storyline title
(Demon Slayer, Rascal) is just a top-level `Series`. A brand with several independent
storylines (Gundam, Fate, iDOLM@STER) is a `Franchise` wrapping those `Series`. Below a
Series: **Seasons** (the numbered TV run), **Movies**, and **Specials** (OVAs); a Season
holds **Episodes**.

### 1.2 Field reference

| Field | Entity | Why it exists |
|---|---|---|
| `series[]` | Franchise (optional) | The distinct storylines of a multi-story brand (Gundam, Fate) |
| `watchOrders[]` | Franchise | Curated alternate orders across its Series, e.g. chronological (release is the default) — §2.5 |
| `titles {english,romaji,native}` | all named entities | *Bunny Girl Senpai* (en) vs *Seishun Buta Yarō* (romaji) |
| `seasons[]` / `movies[]` / `specials[]` | Series | Members: numbered TV run, films, OVAs/specials |
| `number` / `part` | Season | Season index, and split-cour part within it (§2.3) |
| `releaseYear` / `releaseSeason` | Season | The airing "season" — e.g. Spring 2012; a primary browse axis (§2.4) |
| `externalIds.anilistId` | Season / Movie / Special | **The media id**, once per node — the R2 enrichment key |
| **`absoluteNumber`** | Episode / Movie | **The one field no free API gives us** — sort key within a numbered Series |
| `alternateCutOf` | Movie | "Alternate cut of" — links a film to the Season that carries its numbers |

The model **stores facts** (ids, numbers, dates, our `absoluteNumber`) and **fetches
expression** (synopsis, art, stills) live (research note §5.1a).

---

## Part 2 — Rules & concepts

### 2.1 Watch order within a Series

A Series' watch order is **derived, not configured** — there is no `ordering` flag, because
the presence or absence of `absoluteNumber` already *is* the signal (so the two can never
disagree):

- If its episodes/movies carry **`absoluteNumber`**, that is the order — a curated continuous
  count (Demon Slayer, Rascal, *Fate/Zero*). This is the costly R1 data; assign it only where
  a clean linear order actually exists. It is **scoped to one Series** — *Fate/Zero* numbers
  independently of *Fate/stay night*.
- If they **don't**, the storyline has no single linear stream (parallel-route adaptations
  like *Fate/stay night*), so its seasons/movies/specials just sort by **`releaseDate`**.

For *Fate/stay night* (no numbers), release order gives Fate route (2006) → Unlimited Blade
Works (2014) → Heaven's Feel (2017) — also the intended order, since the Saber-route
adaptation aired first.

### 2.2 Movies, specials & numbering

- **Original vs alternate-cut movies.** An *original* film (unique content) takes its own
  `absoluteNumber`; an *alternate-cut* film whose content also airs as a Season sets
  `alternateCutOf` and takes **no** number — the **Season carries the numbers** (it has
  per-episode granularity).
- **OVAs / specials.** Side content in `specials[]`, placed by `releaseDate` with **no season
  `number`**; given an `absoluteNumber` only when pinned into the canon watch order.

### 2.3 Split-cour seasons ("Part 1 / Part 2")

Many seasons air in two cours months — or years — apart, often as **separate AniList nodes**
(*Attack on Titan: The Final Season* Parts 1–3; *Re:Zero* S2; *Fate/Zero* and Demon Slayer S2
in §3.3). Each part is its own `Season` sharing `number`, differing by `part` + `releaseDate`:

```yaml
seasons:
  - { id: show-s2-part1, number: 2, part: 1, releaseDate: 2020-07-08,
      externalIds: { anilistId: 11111 }, episodes: [ "… airedNumber 1..13 …" ] }
  - { id: show-s2-part2, number: 2, part: 2, releaseDate: 2022-01-09,   # different year
      externalIds: { anilistId: 22222 }, episodes: [ "… airedNumber may continue or reset …" ] }
```

- A broadcast **"season"** is the set of `Season`s sharing `number`; `part` orders them. (So
  `number` is *not* unique per `Season` — `number` + `part` is.)
- `airedNumber` follows the broadcast; `absoluteNumber` is unaffected — it flows by release
  order.
- **If both cours are a single AniList node**, it's one `Season` whose episodes span two air
  windows — `releaseDate` captures the gap and `part` stays null.

### 2.4 Release season & year

Anime are browsed by their **airing season** — "Spring 2012", "Fall 2019". Each `Season`
records it as `releaseYear` (`2012`) and `releaseSeason` (`WINTER | SPRING | SUMMER | FALL`).

- **Term overload (flagged on purpose):** this airing *season* is a calendar quarter — it is
  **not** the `Season` entity (a TV installment). `Season.releaseSeason` = the quarter the
  installment premiered in.
- **Normally derived** from `releaseDate`: year from the date, quarter from the month (Jan–Mar
  = Winter, Apr–Jun = Spring, Jul–Sep = Summer, Oct–Dec = Fall). Taken from AniList's
  `seasonYear` / `season` when those are authoritative — they settle late-December / boundary
  premieres the naive month-map gets wrong.
- Movies and specials carry a `releaseYear` from their own `releaseDate`; the quarter is
  meaningful mainly for seasonal TV.

### 2.5 Cross-Series watch orders

`absoluteNumber` orders a single linear storyline. It cannot express how *Series* relate
across a franchise. *Fate* (§3.2) has two such orders, and crucially **both are objective
facts, not opinions**:

- **Release** — by air date: stay night (2006) → Fate/Zero (2011) → UBW (2014) → Heaven's Feel
  (2017…). Fully **derivable** by sorting on `releaseDate`, so it is the **default** and is
  never stored.
- **Chronological** — by in-universe timeline: the prequel first, Fate/Zero → Fate/stay night.
  An objective fact about the story, but **not** in release dates or any free file, so it is
  the order that must be recorded.

Subjective "best newcomer path" recommendations are deliberately **out of scope**. So the only
thing stored is the non-derivable order(s) — release is implicit:

```yaml
Franchise:
  id: fate
  series: [ … ]                               # as in §3.2
  watchOrders:
    - name: "Chronological"                   # in-universe; curated, not derivable
      entries: [ { ref: fate-zero }, { ref: fate-stay-night } ]
  # Release order is the default — derived from releaseDate, never stored.
```

How it composes:

- **Macro vs micro.** A `watchOrder` sequences whole Series / Seasons / Movies; *inside* each
  referenced node you fall back to that Series' own order (`absoluteNumber` if present, else
  `releaseDate`). The watch order = the cross-node order; `absoluteNumber` = the within-Series
  order. They layer, they don't compete.
- **Mixed granularity.** An entry can point at a whole Series (*all of Fate/Zero*), a single
  Season (*UBW*), or a Movie — whatever the order needs.
- **Lives under the Franchise.** Cross-Series order only exists when a brand has several
  Series. Release is the default everywhere (derived from `releaseDate`); only curated
  alternates like chronological are stored.
- **A standalone `Series` needs no `watchOrders`.** It already carries two orders for free —
  **release** (via `releaseDate`) and its **canonical** order (via `absoluteNumber`). They
  agree for most shows and diverge exactly when a single storyline has two legit orders:
  *Monogatari*'s broadcast order is `releaseDate`, its chronological order is `absoluteNumber`.

---

## Part 3 — Worked examples

### 3.1 Gundam — the optional `Franchise` tier

Gundam is the textbook multi-storyline brand: independent continuities sharing only the name,
each numbering on its own. This is *why* `Franchise` exists.

```yaml
Franchise:                                   # present only because Gundam has many storylines
  id: gundam
  titles: { english: "Gundam", romaji: "Gundam" }
  series:
    - id: gundam-uc                          # Universal Century — one big linear continuity
      titles: { english: "Mobile Suit Gundam (Universal Century)" }
      seasons: [ "0079 → Zeta → ZZ → …" ]
      movies:  [ "Char's Counterattack, F91, …" ]
    - id: gundam-wing                        # After Colony — independent
      titles: { english: "Mobile Suit Gundam Wing" }
      movies:  [ "Endless Waltz" ]
    - id: gundam-seed                        # Cosmic Era — independent
      titles: { english: "Mobile Suit Gundam SEED" }
      seasons: [ "SEED, SEED Destiny" ]
      movies:  [ "SEED Freedom (2024)" ]
    - id: gundam-ibo                         # Post Disaster — independent
      titles: { english: "Mobile Suit Gundam: Iron-Blooded Orphans" }
    - id: gundam-witch                       # Ad Stella — independent
      titles: { english: "The Witch from Mercury" }
```

Each Series is a self-contained watch order — there is **no** franchise-wide `absoluteNumber`
across Wing and SEED. The `Franchise` is grouping + titling only. (Other brands of this shape:
*iDOLM@STER*, *Love Live!*, *Precure*, *Yu-Gi-Oh!*, *Macross*, *Digimon*.)

### 3.2 Fate — numbered vs date-ordered series in one franchise

```yaml
Franchise:
  id: fate
  titles: { english: "Fate", native: "フェイト" }
  series:
    - id: fate-stay-night
      titles: { english: "Fate/stay night" }
      # parallel routes → no absoluteNumber, so members sort by releaseDate
      seasons:
        - { id: fsn-2006, titles: { english: "Fate/stay night (2006)" }, number: 1,
            releaseDate: 2006-01-07, releaseYear: 2006, releaseSeason: WINTER,
            externalIds: { anilistId: 356 } }                                # Fate/Saber route
        - { id: fsn-ubw,  titles: { english: "Unlimited Blade Works" }, number: 2,
            releaseDate: 2014-10-12, releaseYear: 2014, releaseSeason: FALL,
            externalIds: { anilistId: 20716 } }                              # UBW route (itself split-cour)
      movies:
        - { id: fsn-hf-1, titles: { english: "Heaven's Feel I" }, releaseDate: 2017-10-14,
            releaseYear: 2017, externalIds: { anilistId: 20724 } }           # no absoluteNumber → sorts by date
        # … Heaven's Feel II (2019), III (2020) …

    - id: fate-zero
      titles: { english: "Fate/Zero" }
      # single linear story (split-cour) → episodes carry absoluteNumber
      seasons:
        - { id: fz-s1, number: 1, part: 1, releaseDate: 2011-10-02, releaseYear: 2011, releaseSeason: FALL,
            externalIds: { anilistId: 10087 }, episodes: [ "… absolute 1–13 …" ] }
        - { id: fz-s2, number: 1, part: 2, releaseDate: 2012-04-08, releaseYear: 2012, releaseSeason: SPRING,
            externalIds: { anilistId: 11741 }, episodes: [ "… absolute 14–25 …" ] }
```

- **`Fate/stay night` — no `absoluteNumber`.** The 2006 route, UBW, and Heaven's Feel adapt
  different visual-novel routes — not a linear sequence — so members sort by `releaseDate`,
  giving Fate route → UBW → Heaven's Feel (the intended order).
- **`Fate/Zero` — numbered.** A single linear story (just split across two cours), so its
  episodes carry continuous `absoluteNumber` 1–25.
- Each Series numbers (or doesn't) on its own; the `Franchise` only groups them, and adds the
  curated **chronological** order (§2.5).

### 3.3 Demon Slayer — standalone Series, numbering mechanics

Demon Slayer is a single storyline, so it's a **top-level `Series`** — no `Franchise` wrapper.
It exercises alternate-cut films, split-cour, and standalone movies.

```yaml
Series:
  id: demon-slayer
  titles: { english: "Demon Slayer: Kimetsu no Yaiba", romaji: "Kimetsu no Yaiba", native: "鬼滅の刃" }
  seasons:
    - id: ds-s1                               # → absolute 1–26
      number: 1
      releaseDate: 2019-04-06
      releaseYear: 2019
      releaseSeason: SPRING                   # "Spring 2019"
      externalIds: { anilistId: 101922 }
      episodes:
        - { absoluteNumber: 1,  airedNumber: 1,  releaseDate: 2019-04-06 }
        # … through 26 …
    - id: ds-mugen-train-arc                  # Season 2 Part 1 → absolute 27–33
      titles: { english: "Mugen Train Arc" }  #   THIS carries Mugen Train's numbers
      number: 2
      part: 1
      releaseDate: 2021-10-10
      externalIds: { anilistId: 142984 }
      episodes:
        - { absoluteNumber: 27, airedNumber: 1, releaseDate: 2021-10-10 }
        # … through 33 (7 eps) …
    - id: ds-entertainment-district           # Season 2 Part 2 → absolute 34–44
      titles: { english: "Entertainment District Arc" }
      number: 2
      part: 2
      releaseDate: 2021-12-05
      externalIds: { anilistId: 142329 }
      episodes:
        - { absoluteNumber: 34, airedNumber: 1, releaseDate: 2021-12-05 }
        # … through 44 (11 eps); Swordsmith Village (S3) 45–55, Hashira Training (S4) 56–63 …
  movies:
    - id: ds-mugen-train-film                 # ALTERNATE CUT — no absoluteNumber
      titles: { english: "Mugen Train" }
      releaseDate: 2020-10-16
      externalIds: { anilistId: 112151 }
      alternateCutOf: { seasonId: ds-mugen-train-arc, episodes: "1-7" }
    - id: ds-infinity-castle-1                # ORIGINAL standalone trilogy → own slots
      titles: { english: "Infinity Castle (Part 1)", romaji: "Mugen Jō-hen" }
      releaseDate: 2025-07-18                  # illustrative
      externalIds: { anilistId: 178680 }        # illustrative
      absoluteNumber: 64
    # … Infinity Castle Part 2 → 65, Part 3 → 66 …
```

| Concern | How the model handles it |
|---|---|
| **Mugen Train: film vs TV** | The Season `ds-mugen-train-arc` carries episodes 27–33; the film sets `alternateCutOf` and takes no number — "use the TV series, not the movie" (§2.2) |
| **Standalone movies** (*Infinity Castle*) | First-class `Movie` with no season, each taking its own `absoluteNumber` (64–66) |
| **Split-cour S2** | Mugen Train Arc (`part: 1`) + Entertainment District (`part: 2`) share season `number` 2 (§2.3) |
| **Seasons restart at episode 1** | `absoluteNumber` is the continuous count; `airedNumber` keeps local numbers |

> **Chronology note.** The *Mugen Train* film (2020) predates its TV cut (2021). We still pick
> the Season as the numbering carrier; the film stays reachable via `alternateCutOf`.

### 3.4 Rascal Does Not Dream — two seasons + movies

The motivating case: a standalone numbered `Series`, two `Season`s, original movies interleaved
by `releaseDate`.

| `absoluteNumber` | member (kind) | season `number` | release |
|:--:|---|:--:|---|
| 1–13 | Bunny Girl Senpai (Season) | 1 | Fall 2018 |
| 14 | Dreaming Girl (movie) | — | 2019-06-15 |
| 15 | Sister Venturing Out (movie) | — | 2023-06-23 |
| 16… | Rascal Does Not Dream of Santa Claus (Season) | 2 | 2025 (illustrative) |

Season 2 is *Rascal Does Not Dream of Santa Claus* (romaji *Seishun Buta Yarō wa Santa Claus no
Yume wo Minai*). Two seasons get a continuous absolute count even though each restarts
`airedNumber` at 1, and the movies interleave by release date.

---

## Part 4 — Building the records

Maps to the research note §5.3 pipeline:

1. **Seed** the `Series` (and an optional `Franchise` when a brand has several) plus each
   Series' `seasons[]`/`movies[]`/`specials[]` from `anime-offline-database`, bucketed by
   AniList `format` (TV → Season, MOVIE → Movie, OVA/ONA/SPECIAL → Special). Carry AniList
   `seasonYear`/`season` into `releaseYear`/`releaseSeason` (§2.4).
2. **Number the linear series.** Where a Series has a single linear order, assign
   `absoluteNumber` across its episodes + original movies (from `anime-list.xml`) in release
   order; otherwise leave it to release-date order (no numbers).
3. **Slot movies** from `anime-movieset-list.xml`: original films get a number (numbered series
   only); alternate cuts get `alternateCutOf` and none.
4. **Override** the judgement calls — Series/Franchise boundaries, whether a Series is numbered,
   alt-cut vs original, `number`/`part`, and any cross-Series `WatchOrder`s — in
   `franchise-overrides.yaml`.
5. **Store** next to `internal/db/anime.go`; **refresh** on a schedule, overrides win.

---

## Part 5 — Open questions

- **Unify ordering?** Within-Series order (`absoluteNumber`) and cross-Series order
  (`watchOrders`) are two mechanisms. Keep both — number as the cheap materialized path, watch
  order as the curated one — or express everything as watch orders? Kept separate here so the
  common case stays a simple integer sort.
- **Picking the order** (product/UX, not data) — release is the default; do users opt into a
  stored alternate (chronological) per session, and is that catalog-wide or per-user?

Settled during design (no longer open): **OVA / special placement** — the model already
supports both, side content by `releaseDate` or pinned with an `absoluteNumber` (§2.2);
**original vs alternate-cut** — decided as a hand-authored `alternateCutOf` per film (§3.3),
since no open file provides it; **R3 `title`** — an optional field, with the sourcing gap
documented in research note §3.3.
