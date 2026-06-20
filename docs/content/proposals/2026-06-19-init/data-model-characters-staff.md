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

**Characters** (fictional) and the **Staff** who play them are both **R2** enrichment (research note
§2.1). Unlike the strict `Franchise → Series → Season → Episode` tree, they cut **across** it — the
same character or person appears in many Series and many Franchises — so each is a **global**,
**many-to-many** node onto the [Anime Series Data Model](../data-model-anime-series/). They are also
tightly linked: a character's voice actor *is* a `Staff` member, which is why they share one doc.

> **Scope.** Staff currently covers **only voice actors** — production credits (director, music, …)
> are deferred to a later iteration, so `Staff` is just the person a `voiceActors.staffId` resolves
> to. **Storage:** per facts-vs-expression (research note §5.1a) we store **facts** — IDs, names, the
> appearance graph, and voice-actor *associations* — and **fetch expression live** (character `role`,
> bios, images), which AniList's ToS forbids warehousing. IDs below are illustrative.

---

## Part 1 — The model

### 1.1 Entities

```text
Character               GLOBAL fictional entity — owned by no Franchise or Series
  id
  names                 { english?, romanized, original, aliases[]? }
  externalIds           { anilistId, … }
  voiceActors[]         DEFAULT cast: { staffId, language } — the usual VA across appearances (§2.3)
  appearances[]         CharacterAppearance

CharacterAppearance     a Character ↔ Series link (many-to-many edge)
  seriesId              the Series — the rollup association (always)
  scope[]?              optional specific nodes: { seasonId } | { movieId } | { specialId } (§2.2)
  voiceActors[]?        optional override of the default cast (a recast, or an added dub) (§2.3)
  externalIds?          optional per-appearance AniList id override (§2.5)

Staff                   GLOBAL real person — currently only voice actors (credits deferred)
  id
  names                 { english?, romanized, original, aliases[]? }
  externalIds           { anilistId, … }
```

Neither entity is nested under a Series. A `Character` reaches the franchise model through its
`appearances`, which name a `seriesId` from the [Anime Series Data Model](../data-model-anime-series/).
A `Staff` is reached only from the character side: `Character.voiceActors.staffId` resolves to it.

### 1.2 Naming fields

Every title/name uses the same language-agnostic triple (works call it `titles`, people call it
`names`):

| Key | Meaning | Example |
|---|---|---|
| `english` | The English title/name | *Saber* |
| `romanized` | The original transliterated to Latin script (romaji / pinyin / RR — not Japan-specific) | *Seibā* |
| `original` | The title/name in its original language & script | セイバー |
| `aliases[]` | Other known names / spoiler names (optional) | — |

### 1.3 Field reference

| Field | Entity | Why it exists |
|---|---|---|
| `names` | Character / Staff | Localized names (§1.2) |
| `externalIds.anilistId` | Character / Staff | The id — join key, live-fetch handle, and what `voiceActors.staffId` points at |
| `voiceActors[]` | Character | Default cast — `{ staffId, language }`, shared across appearances (§2.3) |
| `appearances[]` | Character | The many-to-many links into the Series spine |
| `seriesId` / `scope[]` | CharacterAppearance | Associated Series, narrowed to specific nodes if needed (§2.2) |
| `voiceActors[]` | CharacterAppearance | Optional override of the default cast — recast / added dub (§2.3) |
| `externalIds` | CharacterAppearance | Optional per-appearance AniList id override (§2.5) |

---

## Part 2 — Rules & concepts

### 2.1 Both are global and many-to-many

A `Character` and a `Staff` are each their own node, not nested under the franchise tree. The same
character or person spans **many Series and many Franchises**; its Franchises are **derived** from
its edges, never stored. Identity is keyed by `externalIds.anilistId` — edges across media collapse
to one node with many appearances.

### 2.2 Attaching to the spine: Series rollup + optional node scope

`CharacterAppearance.seriesId` is the rollup association, plus an optional `scope[]` of typed media
nodes — `{seasonId}` / `{movieId}` / `{specialId}` — to narrow when the character isn't in the whole
Series. Characters are usually consistent across a Series' seasons and movies, so whole-Series is the
default; `scope[]` covers a character who debuts in one film or a single OVA.

### 2.3 Voice actors: default on the Character, overridable per appearance

A character usually keeps the **same voice actor across series** (Saber is Ayako Kawasumi in every
Fate work), so the default cast lives on the `Character` as `voiceActors[]`, one entry per
`language`. When an appearance **recasts** or **adds a dub**, it overrides with its own
`voiceActors[]`; otherwise it inherits the default. Each `staffId` resolves to a `Staff` node.

### 2.4 Voice roles live on the character side

A `Staff` node holds only the person (names + ids) — the **voice roles live on the character side**
as `CharacterAppearance.voiceActors`. "Characters voiced by this Staff" is a query over those edges
by `staffId`, not a list stored on `Staff`. (When production credits are added later, they'll hang
off `Staff`; voice roles stay here.)

### 2.5 Per-edge AniList id overrides

We unify a character/person under one node (our `id`) even when AniList doesn't — AniList sometimes
lists the "same" one under **different ids** in different media (alternate forms, data splits). The
appearance then carries its own `externalIds` so the live fetch for that media hits the right node,
while everything still rolls up to one `Character`. Omit it to inherit the node's canonical id.

### 2.6 Storage: facts vs expression

| Data | Store? | Why |
|---|:--:|---|
| `id`, `externalIds`, `names` (both) | ✅ | Facts / our keys |
| Appearance graph (`seriesId`, `scope`, id overrides) | ✅ | Factual associations we derive and own |
| Voice-actor links (`staffId` + `language`) | ✅ | Factual "who voices whom" |
| Character `role` (main/supporting) | ❌ fetch live | Editorial per-media classification, not a fact |
| Bios, descriptions, images (character & staff) | ❌ fetch live | AniList "expression" — ToS forbids warehousing (§5.1a) |

---

## Part 3 — Worked examples

### 3.1 Saber — one Character, many Series in one Franchise

Saber appears across several *Fate* Series (see
[Anime Series Data Model §3.2](../data-model-anime-series/#32-fate--numbered-vs-date-ordered-series-in-one-franchise)).

```yaml
Character:
  id: artoria-pendragon
  names: { english: "Saber (Artoria Pendragon)", romanized: "Seibā", original: "セイバー" }
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
  names: { english: "Subaru Natsuki", romanized: "Natsuki Subaru", original: "ナツキ・スバル" }
  externalIds: { anilistId: 119377 }                           # illustrative
  voiceActors: [ { staffId: yusuke-kobayashi, language: ja } ]
  appearances:
    - { seriesId: re-zero }        # Franchise: Re:Zero
    - { seriesId: isekai-quartet } # Franchise: Isekai Quartet  ← different franchise
```

### 3.3 Ayako Kawasumi — the voice actor (Staff node)

```yaml
Staff:
  id: ayako-kawasumi
  names: { english: "Ayako Kawasumi", romanized: "Ayako Kawasumi", original: "川澄綾子" }
  externalIds: { anilistId: 95012 }     # illustrative
```

This is the `staffId` that Saber's appearance points at (§3.1:
`voiceActors: [ { staffId: ayako-kawasumi, … } ]`). Her voice roles — Saber and many others — are
read from those character edges (§2.4), not stored on the `Staff` node.

---

## Part 4 — Building the records

1. **Seed** characters and their voice actors from each media node's AniList character/VA edges —
   the media IDs are already in the [Anime Series Data Model](../data-model-anime-series/)
   (`externalIds.anilistId`).
2. **Dedup** into one `Character` / `Staff` per AniList id; collect an appearance per Series,
   `scope`d to the node when warranted, with a per-appearance `externalIds` override when a media
   uses a different character id (§2.5).
3. **Resolve voice actors** — the VA shared across a character's appearances becomes the
   `Character.voiceActors` default; per-series differences become appearance overrides.
4. **Store** the nodes + appearance graph + names + VA links (facts) next to the franchise records;
   **fetch** character `role`, bios, and images live (never warehoused).
5. **Refresh** with the franchise pipeline; curation overrides win.

---

## Part 5 — Open questions

- **Scope granularity** — `scope[]` reaches Season / Movie / Special; is anything finer
  (per-Episode) ever needed, or is node-level enough?
- **Merge vs split identity** — the per-edge `externalIds` override (§2.5) lets one node span
  differing AniList ids, but *deciding* whether an alternate-form / what-if version is the same
  character (or person) is a curation call — what's the default policy?
- **VA reverse-index** — "characters voiced by this Staff" is a query over the character edges
  (§2.4); materialize it for performance, or resolve on demand?
- **Deferred: production credits & studios** — `Staff` currently covers only voice actors; crew
  credits (director, music, …) and a `Studio` model (organizations, also many-to-many) are future
  sibling work.
