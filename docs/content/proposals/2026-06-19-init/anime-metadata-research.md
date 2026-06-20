---
title: "Anime Series/Franchise Metadata"
date: 2026-06-19
weight: 1
---

# Research Note: Anime Series/Franchise Metadata for anime-image-viewer

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Research / decision input

## 1. Problem statement

anime-image-viewer currently sources metadata from the **AniList** GraphQL API
(`internal/anilist`). AniList is excellent for per-title data — titles, tags,
characters, staff, cover art — but its **data model cannot represent a franchise
as a single, ordered timeline**.

The issue is structural, not an API/protocol limitation. AniList (like MyAnimeList
and Kitsu) models anime as a **graph of media nodes + relation edges**: every
season, movie, OVA, and subtitled part is its own node, and episodes are numbered
`1..N` *locally inside each node*. There is no field anywhere that expresses
"this is episode 23 of the franchise" or "these five releases are one series."

**Canonical failing case — *Rascal Does Not Dream* (Seishun Buta Yarō):**
- The franchise has subtitled parts (*Bunny Girl Senpai*, *Dreaming Girl*,
  *Sister Venturing Out*, *Knapsack Kid*, ...) spread across **two TV seasons
  plus several movies**.
- Episode numbering resets per part, and some parts are movies rather than
  episodes, so there is **no reliable way to sort the franchise into watch order**
  from AniList data alone.

## 2. Evaluation framework

Everything below is organized around **two axes**. Keeping them separate is the
whole point — earlier drafts conflated them and produced misleading conclusions.

- **Data requirements (R1–R3)** — *what data we need.* These are the **rows** of
  every evaluation table.
- **Acceptance gates (A/B/C)** — *qualities every candidate source must have.*
  These are the **columns**. They are themselves requirements (a source is
  unusable if it fails any one), which is why data-structure, maintenance, and
  licensing are defined here as gates rather than as separate survey sections.

> **The rule:** a source *qualifies for a requirement* only if it passes **all
> three gates (A, B, C) for that requirement**. We evaluate **per requirement, not
> per source** — the same source can qualify for one requirement and fail another.

### 2.1 Data requirements (the rows)

| ID | Requirement | What it means | Met by AniList today? |
|---|---|---|---|
| **R1** | **Franchise timeline ordering** | Represent a franchise as one ordered timeline — TV episodes given a franchise-wide **absolute number**, movies grouped and slotted in — so *Rascal* sorts into watch order. *Numbering/ordering.* | ❌ **No** — the motivating gap |
| **R2** | **Per-title enrichment** | Title-level metadata per release: titles, tags, characters, staff, cover art. | ✅ Yes |
| **R3** | **Per-episode content** | Content *of an individual episode*: title, air date, still image, synopsis. Distinct from R1 — R1 says *which* episode #16 is; R3 says *what* episode #16 is. | ⚠️ Partial only |

### 2.2 Acceptance gates (the columns)

Each gate is a pass/fail requirement applied to a source **for a given data
requirement**. The reference tables under each gate are the *evidence* the §3
evaluation draws on.

#### Gate A — Data structure (can the source even express the data?)

A is judged against the specific requirement, never softened. The structural model
of each database:

| Database | Data model | Franchise grouping (R1) | Episode ordering (R1) | Per-episode content (R3) |
|---|---|---|---|---|
| **AniList / MyAnimeList / Kitsu** | Media node + relation edge graph | ❌ none; traverse relations yourself | ❌ local per-node only | ⚠️ partial (AniList `streamingEpisodes`) |
| **AniDB** | Anime container + typed episodes (`epno`, S/C/T) | ⚠️ ~one entry per season | ✅ rigorous per-entry, fragmented across seasons | ✅ best (multi-lang titles, air dates) |
| **TMDB** | Series → Season → Episode, **+ movie `Collection`** | ✅ series + movie collection | ⚠️ single canonical order | ✅ titles, air dates, stills, overviews |
| **TVDB** | Series → Season → Episode with **parallel order types** (Aired/DVD/**Absolute**/Alt) | ✅ good | ✅ **best** — Absolute order flattens to `1..N` | ✅ titles, air dates, episode images |

**Key structural insight for R1:** the "subtitled parts split across seasons" sort
problem is solved by **TVDB Absolute order** (TV side) + **TMDB `Collection`**
(movie side). No single source unifies TV episodes *and* movies onto one timeline —
that composition is the application's job.

**The R1 structure already exists as open files.** The offset math between AniDB's
per-season numbering and TVDB's absolute/season numbering is encoded in
**`anime-list.xml`** ([Anime-Lists/anime-lists](https://github.com/Anime-Lists/anime-lists)):
`defaulttvdbseason`, `episodeoffset`, `<mapping start= end= offset=>`, and
per-episode overrides `;1-5;2-6;`. A sibling **`anime-movieset-list.xml`** groups
movies into sets. **Important:** these files carry *numbering only* — **no** episode
titles/air dates (so they satisfy R1, not R3).

#### Gate B — Maintenance (is the data current, mid-2026?)

| Source | Status | Notes |
|---|---|---|
| **TMDB** | ✅ Best maintained | Huge contributor base |
| **AniList** | ✅ Healthy | Active edits |
| **anime-offline-database** (manami) | ✅ Updated ~weekly | ID cross-refs across 10 providers |
| **Fribb/anime-lists** | ✅ Auto-regenerated | Flat ID map |
| **Anime-Lists/anime-lists** | ✅ Apr 2026 | The live `anime-list.xml` fork |
| **AniDB** | ⚠️ Alive but slow | Volunteer, niche |
| **TVDB** | ⚠️ Data current, API paywalled | See Gate C |
| **MyAnimeList** (via Jikan) | ⚠️ Active | No grouping benefit over AniList |
| ~~ScudLee/anime-lists~~ | ❌ Superseded | Use the Anime-Lists fork |

#### Gate C — Licensing / free-use (can we use it the way we'd ship it?)

| Source | Access | Commercial use? | Catch |
|---|---|---|---|
| **AniList** | GraphQL, no key, ~90 req/min | ✅ Yes | None meaningful |
| **Jikan** (MAL) | REST, no key | ✅ Yes | Unofficial scraper; cache it |
| **Kitsu** | JSON:API, no key | ✅ Yes | Thin metadata |
| **anime-offline-database** | Downloadable JSON | ✅ **Yes (ODbL + DbCL)** | Share-alike + attribution |
| **Anime-Lists `*.xml`** | Raw GitHub files | ✅ Yes | Vendor + refresh |
| **Fribb/anime-lists** | Downloadable JSON/XML | ✅ Yes | Derived dataset |
| **AniDB** | HTTP/UDP API | ⚠️ Non-commercial only | Client registration + harsh limits |
| **TMDB** | REST + key | ❌ Non-commercial only | Commercial needs agreement; attribution string |
| **TVDB** | REST + key | 💲 Paid | <$50k rev free w/ attribution, else $1k–$10k/yr |

> TMDB attribution string, if used:
> "This product uses the TMDb API but is not endorsed or certified by TMDb."

## 3. Evaluation (each requirement × the three gates)

Applying §2.2's gates to §2.1's requirements. A source qualifies only with ✅ in all
three gate columns.

> ⚠️ **AniList — currently in use — passes R2 but FAILS R1.** Its node+edge model
> cannot express a franchise timeline (§2.2 Gate A), so it fails for R1 regardless
> of being maintained (B) and free (C). **AniList is not a solution to the
> motivating problem (R1).** It is retained only for R2, and partially R3.

### 3.1 R1 — franchise timeline ordering (the motivating problem)

| Source | (A) Structure | (B) Maintained | (C) Free-use | Qualifies for R1? |
|---|---|---|---|---|
| **AniList** (current) | ❌ no franchise/ordering model | ✅ | ✅ | ❌ **fails A** |
| **Anime-Lists `anime-list.xml`** | ✅ absolute episode ordering | ✅ Apr 2026 | ✅ open, commercial-OK | ✅ |
| **`anime-movieset-list.xml`** | ✅ movie-set grouping | ✅ | ✅ open | ✅ |
| **anime-offline-database** | ✅ franchise ID clustering | ✅ ~weekly | ✅ ODbL | ✅ |
| **TVDB** | ✅ best ordering model | ✅ | ❌ paid API | ❌ **fails C** (value mirrored in the XML) |
| **TMDB** | ✅ series + movie `Collection` | ✅ | ❌ non-commercial only | ❌ commercial; ⚠️ non-commercial |
| **AniDB** | ✅ rigorous episodes | ⚠️ slow | ❌ non-commercial + limits | ❌ (already baked into the XML) |

**Only the open-data files qualify for R1.** R1 is solved by vendored open data —
**not by any single live API, and not by AniList.**

### 3.2 R2 — per-title enrichment

| Source | (A) Structure | (B) Maintained | (C) Free-use | Qualifies for R2? |
|---|---|---|---|---|
| **AniList** | ✅ rich tags/characters/art | ✅ | ✅ keyless, commercial-OK | ✅ |
| **Jikan (MAL)** | ✅ comparable | ✅ | ✅ | ✅ (no gain over AniList) |
| **Kitsu** | ⚠️ thinner | ✅ | ✅ | ⚠️ |

AniList qualifies for R2 cleanly — the **only** thing it is recommended for.

### 3.3 R3 — per-episode content

| Source | (A) Structure | (B) Maintained | (C) Free-use | Qualifies for R3? |
|---|---|---|---|---|
| **AniDB** | ✅ best (titles, air dates, types) | ⚠️ slow | ❌ non-commercial + limits | ❌ |
| **TVDB** | ✅ titles, air dates, images | ✅ | ❌ paid API | ❌ |
| **TMDB** | ✅ titles, air dates, stills | ✅ | ❌ non-commercial only | ❌ commercial; ⚠️ non-commercial |
| **AniList** | ⚠️ partial (`streamingEpisodes`, `airingSchedule`) | ✅ | ✅ | ⚠️ **incomplete** |
| **Open-data files** (R1 set) | ❌ numbering only | ✅ | ✅ | ❌ |

**Finding: no source qualifies for R3.** Rich per-episode content is either
incomplete (AniList) or licensing-blocked (TVDB paid; TMDB/AniDB non-commercial).
R3 is the **one requirement with no free + commercial-safe source** — handle it
explicitly (drop, AniList-partial, TMDB-if-non-commercial, or curate; see §5).

## 4. Recommendation

**Bottom line: no single source meets all three requirements.** The minimal
combination that covers what *can* be covered is **AniList (R2) + open-data files &
`Franchise` model (R1)**; **R3 has no free + commercial-safe source and stays a
gap.** TMDB is optional and only for non-commercial builds.

### 4.1 Coverage at a glance (which source meets which requirement)

| Source | R1 ordering | R2 per-title | R3 per-episode | Commercial-safe? | Recommended for |
|---|:--:|:--:|:--:|:--:|---|
| **Open-data files** + `Franchise` model | ✅ | ❌ | ❌ | ✅ | **R1** (the *Rascal* fix) |
| **AniList** (already integrated) | ❌ | ✅ | ⚠️ partial | ✅ | **R2** |
| **TMDB** (optional) | ✅ | ➖ | ✅ | ❌ non-commercial only | R3 + art, **non-commercial only** |
| *anything free + commercial* | — | — | ❌ | — | **R3 unsolved → gap** |

Read it as: **R1 ✅ AniList-free files · R2 ✅ AniList · R3 ❌ no one.**

### 4.2 The recommended stack
- **R1** — vendor `anime-list.xml` (ordering) + `anime-movieset-list.xml` (movies) +
  `anime-offline-database` (ID clustering), joined by a first-class **`Franchise`
  model** (§5). This — not any API — is what makes the franchise sortable.
- **R2** — keep **AniList** for titles/tags/characters/art, keyed by the IDs the R1
  layer clusters. Not used for ordering.
- **R3** — **gap; decide per build:** AniList's partial `streamingEpisodes`, add
  **TMDB only if non-commercial**, or curate titles/dates in the override layer (§5).

### 4.3 What we explicitly stop doing
- Treating AniList as a franchise/ordering source — fails Gate A for R1.
- Calling TVDB or AniDB live — fails Gate C; their structure is already in the open files.

## 5. Option: build the database ourselves

If the curated open data is insufficient (coverage gaps, our own arc/part
definitions, image-collection-specific grouping), we own the franchise layer. This
is **aggregation + curation**, not crawling from scratch.

### 5.1 What "building it" actually means
We do **not** re-collect raw metadata (titles, air dates, characters) — AniList/
MAL/AniDB do that well. We build the **franchise/ordering layer on top**: the
mapping that says *these N releases are one series, in this order, with movies
slotted at these points*.

### 5.1a What we may legally STORE (storage ≠ API access)

Storing into our own DB is *caching + redistribution*, governed by data licenses —
**not** the same as being allowed to call an API. Decisive constraint:

> ⚠️ **AniList's ToS prohibits using the API "as a backup or data storage service"
> and prohibits "hoarding or mass collection."** We may query + display AniList at
> runtime, but **must not warehouse or redistribute AniList content.**

| Source | Store in own DB? | Redistribute? | Notes |
|---|:--:|:--:|---|
| **anime-offline-database** | ✅ | ✅ | ODbL — explicit grant; share-alike + attribution |
| **Anime-Lists XML / Fribb** | ⚠️ | ⚠️ | No explicit license, but pure ID/number *mappings* (facts); de-facto vendored everywhere |
| **Our derived data** (`Franchise`, `absoluteNumber`, overrides) | ✅ | ✅ | We author it |
| **AniList** | ❌ | ❌ | ToS forbids storage/mass collection — runtime only |
| **TMDB / TVDB / AniDB / MAL** | ❌ | ❌ | Ownership retained / paid / non-commercial |

**The governing principle — facts vs. expression:**
- **Storable even from a "no" source:** *facts* — an **ID**, **episode number**,
  **air date**, **numeric mapping**. Not copyrightable.
- **Not storable:** *creative expression* — synopses/descriptions, curated tag
  *compilations*, cover art, episode stills.

**Design consequence:** the `Franchise` table stores **IDs + ODbL data + our own
computed ordering**, and the app **fetches protected content (synopsis, art) live**
from AniList/TMDB at display time rather than warehousing it. `sourceRefs` holds the
IDs (facts, OK to store); `episodeTitle`/descriptions/images are fetched, not stored
(unless curated by us or sourced from an ODbL/owned dataset).

### 5.2 Data model (proposed)

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

The single most valuable field we add is **`absoluteNumber`** — the thing no free
API gives us directly (R1). `episodeTitle` is optional and only populated if R3 is
sourced (see §3.3).

### 5.3 Build pipeline
1. **Seed** franchise membership from `anime-offline-database` (cross-refs +
   synonyms + relation graph) → which AniList IDs cluster together.
2. **Order** the TV side from `anime-list.xml` (`defaulttvdbseason` + offsets →
   absolute numbers).
3. **Group + slot movies** from `anime-movieset-list.xml`, interleaving by
   `releaseDate`.
4. **Curate overrides** in a small `franchise-overrides.yaml` for what the open
   data gets wrong/doesn't cover, or custom arc-level grouping for the viewer.
5. **Store** resolved `Franchise` records alongside the existing
   `internal/db/anime.go` schema.
6. **Refresh** upstream files on a schedule; re-resolve; overrides always win.

### 5.4 Effort vs. payoff
- **Low effort, high payoff:** steps 1–3 parse well-specified, license-clear files
  and alone fix the *Rascal* sort (R1).
- **Ongoing cost:** the override file (step 4) is the only real burden, growing only
  with franchises we care about.
- **Risk:** upstream schema/coverage drift — mitigated by pinning versions and
  keeping overrides authoritative.

### 5.5 Recommendation on building
**Do the lightweight version:** open files as source + a thin `Franchise`
resolution/override layer. Do **not** maintain a from-scratch metadata database — it
duplicates AniList/AniDB for no benefit and is a perpetual commitment. Own the
*ordering/grouping*, borrow everything else.

## 6. Decision summary

| Question | Answer |
|---|---|
| Why is metadata "thin"? | AniList models per-release; no franchise/ordering field |
| Does AniList solve the problem (R1)? | **No** — fails Gate A (structure) for franchise ordering; kept only for R2 |
| What qualifies for R1 (ordering)? | **Only** the open-data files (`anime-list.xml`, `anime-movieset-list.xml`, ODbL DB) + our `Franchise` model |
| What qualifies for R2 (per-title)? | AniList (what it's actually for) |
| Do the open files include season/episode metadata? | **Numbering/ordering yes; episode *content* (titles/air dates/stills) no** |
| What qualifies for R3 (per-episode content)? | **None** — gap; AniList partial, TVDB paid, TMDB/AniDB non-commercial |
| Best ordering model | TVDB Absolute order — paywalled; used via `anime-list.xml` |
| Best movie grouping | TMDB `Collection` (non-commercial) / `anime-movieset-list.xml` (open) |
| Commercial-safe? | R1+R2 yes (open files + AniList); R3 has no commercial-safe source |
| What may we **store** in our own DB? | ODbL data + ID/number mappings (facts) + our derived ordering. **Not** AniList/TMDB/TVDB/AniDB/MAL content (AniList ToS forbids storage) |
| Store vs. fetch rule | Store *facts* (IDs, episode #, air date, mappings); fetch *expression* (synopsis, art) live |
| Build our own? | Only the thin franchise/ordering layer; not raw metadata |

## 7. Sources
- AniList API rate limiting — https://docs.anilist.co/guide/rate-limiting
- anime-offline-database (ODbL + DbCL) — https://github.com/manami-project/anime-offline-database
- Anime-Lists/anime-lists (`anime-list.xml`, maintained fork) — https://github.com/Anime-Lists/anime-lists
- `anime-movieset-list.xml` — https://github.com/ScudLee/anime-lists/blob/master/anime-movieset-list.xml
- Fribb/anime-lists (ID mapping) — https://github.com/Fribb/anime-lists
- TheTVDB order types (Kodi wiki) — https://kodi.wiki/view/Add-on:The_TVDB_v4
- TheTVDB API pricing — https://www.tinymediamanager.org/blog/tvdb-api-v4/
- TMDB API Terms of Use — https://www.themoviedb.org/api-terms-of-use
- TMDB Rascal Does Not Dream Collection — https://www.themoviedb.org/collection/1073290
- TMDB Rascal TV series (2018–2025) — https://www.themoviedb.org/tv/82739
