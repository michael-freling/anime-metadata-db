---
title: "Characters & Staff Data Model"
date: 2026-06-19
weight: 3
---

# Characters & Staff Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)
**Related:** [Anime Series Data Model](../data-model-anime-series/) — the R1 spine this joins onto.

**Characters** (fictional) and **Staff** (the real people who make anime — voice actors and crew)
are both **R2** enrichment (research note §2.1). Unlike the strict `Franchise → Series → Season →
Episode` tree, they cut **across** it — the same character or person appears in many Series and
many Franchises — so each is a **global**, **many-to-many** node onto the
[Anime Series Data Model](../data-model-anime-series/). They are also tightly linked: a character's
voice actor *is* a `Staff` member, which is why they share one doc.

> **Scope & storage.** R2. Per facts-vs-expression (research note §5.1a), we store **facts** — IDs,
> names, the appearance/credit graph, and voice-actor *associations* — and **fetch expression live**
> (character `role`, bios, images), which AniList's ToS forbids warehousing. IDs below are illustrative.

---

## Part 1 — The model

### 1.1 Entities

```text
Character               GLOBAL fictional entity — owned by no Franchise or Series
  id
  names                 { english, romaji, native, aka[]? }
  externalIds           { anilistId, … }
  voiceActors[]         DEFAULT cast: { staffId, language } — the usual VA across appearances (§2.3)
  appearances[]         CharacterAppearance

CharacterAppearance     a Character ↔ Series link (many-to-many edge)
  seriesId              the Series — the rollup association (always)
  scope[]?              optional specific nodes: { seasonId } | { movieId } | { specialId } (§2.2)
  voiceActors[]?        optional override of the default cast (a recast, or an added dub) (§2.3)
  externalIds?          optional per-appearance AniList id override (§2.6)

Staff                   GLOBAL real person — voice actor and/or production crew
  id
  names                 { native, romaji, english?, aka[]? }
  externalIds           { anilistId, … }
  credits[]             StaffCredit — PRODUCTION roles only (voice roles live on Character, §2.4)

StaffCredit             a Staff ↔ Series/node production credit (many-to-many edge)
  seriesId              the work — the rollup association (always)
  scope[]?              optional specific nodes: { seasonId } | { movieId } | { specialId } (§2.2)
  role                  controlled production role — "Director" | "Series Composition" | "Music" | … (§2.5)
  externalIds?          optional per-credit AniList id override (§2.6)
```

Neither entity is nested under a Series; each reaches the franchise model through its edges, which
name a `seriesId` from the [Anime Series Data Model](../data-model-anime-series/). The
`staffId` on `Character.voiceActors` resolves to a `Staff` node — that is the join between the two.

### 1.2 Field reference

**Character**

| Field | Entity | Why it exists |
|---|---|---|
| `names {english,romaji,native,aka}` | Character | *Saber* (en) vs *Seibā* (romaji); `aka` for aliases/spoiler names |
| `externalIds.anilistId` | Character | The character id — join key and live-fetch handle |
| `voiceActors[]` | Character | Default cast — `{ staffId, language }`, shared across appearances (§2.3) |
| `appearances[]` | Character | The many-to-many links into the Series spine |
| `seriesId` / `scope[]` | CharacterAppearance | Associated Series, narrowed to specific nodes if needed (§2.2) |
| `voiceActors[]` | CharacterAppearance | Optional override of the default cast (recast / added dub) (§2.3) |

**Staff**

| Field | Entity | Why it exists |
|---|---|---|
| `names {native,romaji,english,aka}` | Staff | *澤野弘之* (native) vs *Hiroyuki Sawano* (romaji) |
| `externalIds.anilistId` | Staff | The staff id — what `voiceActors.staffId` points at |
| `credits[]` | Staff | Many-to-many production-credit links into the Series spine |
| `seriesId` / `scope[]` | StaffCredit | The credited work, narrowed to a node when season-specific (§2.2) |
| `role` | StaffCredit | The factual job title — controlled vocabulary (§2.5) |

`externalIds` on either edge is an optional per-edge id override (§2.6).

---

## Part 2 — Rules & concepts

### 2.1 Both are global and many-to-many

A `Character` and a `Staff` are each their own node, not nested under the franchise tree. The same
character or person spans **many Series and many Franchises**; its Franchises are **derived** from
its edges, never stored. Identity is keyed by `externalIds.anilistId` — edges across media collapse
to one node with many appearances/credits.

### 2.2 Attaching to the spine: Series rollup + optional node scope

Both edges carry `seriesId` as the rollup association, plus an optional `scope[]` of typed media
nodes — `{seasonId}` / `{movieId}` / `{specialId}` — to narrow when the link isn't whole-Series:

- **Characters** are usually consistent across a Series' seasons and movies, so whole-Series is the
  default; `scope[]` covers a character who debuts in one film or a single OVA.
- **Staff credits are often node-scoped** — crew changes between seasons (a new director for S2, a
  film with a different composer) — so `scope[]` is the *common* case here, not the exception.

### 2.3 Voice actors: default on the Character, overridable per appearance

A character usually keeps the **same voice actor across series** (Saber is Ayako Kawasumi in every
Fate work), so the default cast lives on the `Character` as `voiceActors[]`, one entry per
`language`. When an appearance **recasts** or **adds a dub**, it overrides with its own
`voiceActors[]`; otherwise it inherits the default. Each `staffId` resolves to a `Staff` node.

### 2.4 Voice roles vs production credits — where each lives

A staff member does two kinds of work, kept apart so neither is duplicated:

- **Voice roles** (a VA voices a Character) are character-centric → they live on the **character**
  side as `CharacterAppearance.voiceActors`. "Characters voiced by this Staff" is a query over those
  edges by `staffId`; they are **not** repeated in `Staff.credits`.
- **Production credits** (director, composition, music, …) aren't character-centric → they live on
  the **staff** side as `Staff.credits`.

So a pure voice actor has an empty `credits[]` and is reached only from the character side; a
composer has `credits` and no voice edges.

### 2.5 `role`: editorial for characters, factual for staff

The word "role" means opposite things on the two sides, so they're treated differently:

- **Character role** (main / supporting) is an *editorial* per-media classification — not a fact, and
  it ships with the character payload we fetch live — so it is **not stored**.
- **Staff role** (director / music / …) is a *factual job title* — this person directed this work —
  so it **is stored**, as a controlled vocabulary normalized from AniList's free-text role strings.

### 2.6 Per-edge AniList id overrides

We unify a character/person under one node (our `id`) even when AniList doesn't — AniList sometimes
lists the "same" one under **different ids** in different media (alternate forms, data splits). The
edge then carries its own `externalIds` so the live fetch for that media hits the right node, while
everything still rolls up to one `Character` / `Staff`. Omit it to inherit the node's canonical id.

### 2.7 Storage: facts vs expression

| Data | Store? | Why |
|---|:--:|---|
| `id`, `externalIds`, `names` (both) | ✅ | Facts / our keys |
| Appearance & credit graph (`seriesId`, `scope`, id overrides) | ✅ | Factual associations we derive and own |
| Voice-actor links (`staffId` + `language`) and staff `role` | ✅ | Factual "who voices / did what" |
| Character `role` (main/supporting) | ❌ fetch live | Editorial per-media classification (§2.5) |
| Bios, descriptions, images (character & staff) | ❌ fetch live | AniList "expression" — ToS forbids warehousing (§5.1a) |

---

## Part 3 — Worked examples

### 3.1 Saber — one Character, many Series in one Franchise

Saber appears across several *Fate* Series (see
[Anime Series Data Model §3.2](../data-model-anime-series/#32-fate--numbered-vs-date-ordered-series-in-one-franchise)).

```yaml
Character:
  id: artoria-pendragon
  names: { english: "Saber (Artoria Pendragon)", romaji: "Seibā", native: "セイバー" }
  externalIds: { anilistId: 497 }                              # illustrative
  voiceActors: [ { staffId: ayako-kawasumi, language: ja } ]   # default cast across all Fate works
  appearances:
    - { seriesId: fate-stay-night }                            # inherits the default VA
    - { seriesId: fate-zero }
    # … also Fate/Grand Order, etc. — same Character node; default VA applies unless overridden
```

### 3.2 Subaru — the same Character across two Franchises

*Isekai Quartet* is a crossover anime; Subaru is a lead in both the **Re:Zero** and **Isekai
Quartet** Franchises — one `Character`, appearances into Series of different Franchises.

```yaml
Character:
  id: subaru-natsuki
  names: { english: "Subaru Natsuki", romaji: "Natsuki Subaru", native: "ナツキ・スバル" }
  externalIds: { anilistId: 119377 }                           # illustrative
  voiceActors: [ { staffId: yusuke-kobayashi, language: ja } ]
  appearances:
    - { seriesId: re-zero }        # Franchise: Re:Zero
    - { seriesId: isekai-quartet } # Franchise: Isekai Quartet  ← different franchise
```

### 3.3 Ayako Kawasumi — a voice actor (credited from the character side)

```yaml
Staff:
  id: ayako-kawasumi
  names: { native: "川澄綾子", romaji: "Ayako Kawasumi" }
  externalIds: { anilistId: 95012 }     # illustrative
  credits: []                           # pure VA — no production crew credits
```

This is the `staffId` that Saber's appearance points at (§3.1:
`voiceActors: [ { staffId: ayako-kawasumi, … } ]`). Her voice roles — Saber and many others — are
read from those character edges, **not** stored in `credits` (§2.4).

### 3.4 Hiroyuki Sawano — a composer across Franchises

```yaml
Staff:
  id: hiroyuki-sawano
  names: { native: "澤野弘之", romaji: "Hiroyuki Sawano" }
  externalIds: { anilistId: 109139 }    # illustrative
  credits:
    - { seriesId: attack-on-titan, role: "Music" }                      # Franchise: Attack on Titan
    - { seriesId: gundam-uc, scope: [ { seasonId: gundam-unicorn } ], role: "Music" }  # Franchise: Gundam (UC)
    - { seriesId: kill-la-kill, role: "Music" }                         # Franchise: Kill la Kill
```

One `Staff`, `credits` spanning three Franchises (derived, never stored). The Gundam credit is
`scope`d to the *Unicorn* node within the UC Series (§2.2), since he scored that entry specifically.

---

## Part 4 — Building the records

1. **Seed** characters and staff from each media node's AniList character/staff lists — the media
   IDs are already in the [Anime Series Data Model](../data-model-anime-series/) (`externalIds.anilistId`).
2. **Dedup** into one `Character` / `Staff` per AniList id; collect an appearance/credit per Series,
   `scope`d to the node when warranted, with a per-edge `externalIds` override when a media uses a
   different id (§2.6).
3. **Resolve voice actors** — the VA shared across a character's appearances becomes the
   `Character.voiceActors` default; per-series differences become appearance overrides.
4. **Normalize staff roles** into the controlled vocabulary (§2.5).
5. **Store** the nodes + appearance/credit graph + names + VA links + staff roles (facts) next to
   the franchise records; **fetch** character `role`, bios, and images live (never warehoused).
6. **Refresh** with the franchise pipeline; curation overrides win.

---

## Part 5 — Open questions

- **Scope granularity** — `scope[]` reaches Season / Movie / Special; per-Episode credits exist on
  AniList (episode director, storyboard). Is node-level enough, or do we ever scope to an Episode?
- **Merge vs split identity** — the per-edge `externalIds` override (§2.6) lets one node span
  differing AniList ids, but *deciding* whether an alternate-form / what-if version is the same
  character (or a split staff entry is the same person) is a curation call — what's the default?
- **Role vocabulary** — how tightly to normalize AniList's free-text staff roles (Director vs Chief
  Director vs Episode Director…)? A flat controlled set, or a hierarchy?
- **VA reverse-index** — "characters voiced by this Staff" is a query over the character edges
  (§2.4); materialize it for performance, or resolve on demand?
- **Studios** — studios are *organizations*, not people, so they're out of scope here; a `Studio`
  model (also many-to-many onto the spine) is a future sibling doc.
