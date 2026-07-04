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
  names                 { original, translations }   (see §1.2)
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
  names                 { original, translations }   (see §1.2)
  externalIds           { anilistId, … }
```

Neither entity is nested under a Series. A `Character` reaches the franchise model through its
`appearances`, which name a `seriesId` from the [Anime Series Data Model](../data-model-anime-series/).
A `Staff` is reached only from the character side: `Character.voiceActors.staffId` resolves to it.

### 1.2 Naming fields

Every title/name uses the same language-agnostic shape (works call it `titles`, people call it
`names`):

| Key | Meaning | Example |
|---|---|---|
| `original` | The title/name in its original language & script — **required** | セイバー |
| `translations` | Map of every other rendering by **BCP-47** code — `en`, `ja-Latn` (romanization), `ko`, … | `{ en: "Saber", ja-Latn: "Seibā", ko: "세이버" }` |

The English name is just `translations.en` — not a privileged field. There is **no Japan-specific
`romanized` field** either: romanization is the `ja-Latn` entry (Korean would be `ko-Latn`, etc.).

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

Node-level (Season / Movie / Special) is the **finest** scope — per-Episode is intentionally not
supported (too granular).

### 2.3 Voice actors: default on the Character, overridable per appearance

A character usually keeps the **same voice actor across series** (Saber is Ayako Kawasumi in every
Fate work), so the default cast lives on the `Character` as `voiceActors[]`, one entry per
`language`. When an appearance **recasts** or **adds a dub**, it overrides with its own
`voiceActors[]`; otherwise it inherits the default. Each `staffId` resolves to a `Staff` node.

### 2.4 Voice roles live on the character side

A `Staff` node holds only the person (names + ids) — the **voice roles live on the character side**
as `CharacterAppearance.voiceActors`. (When production credits are added later, they'll hang off
`Staff`; voice roles stay here.)

**Reverse index (wanted).** "Which characters does this voice actor play?" is a first-class lookup
we want — an index over the `voiceActors` edges keyed by `staffId`. It stores nothing new: it's
derived from the same edges, just queried from the staff side.

### 2.5 Identity: one node per AniList id, merge by override

**Default — trust AniList:** one `Character` / `Staff` per AniList id. So alternate-form or "what-if"
versions that AniList lists separately (e.g. *Saber* vs *Saber Alter*) stay as **separate** nodes
unless we deliberately decide otherwise.

**Override — merge:** when curation decides two AniList ids really are the same node, the edge carries
its own `externalIds` so that media's live fetch still hits the right AniList entry while everything
rolls up to one node. (This also covers AniList listing one character under different ids across media
— a plain data split.) Omit `externalIds` to inherit the node's canonical id.

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
  names: { original: "セイバー", translations: { en: "Saber (Artoria Pendragon)", ja-Latn: "Seibā" } }
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
  names: { original: "ナツキ・スバル", translations: { en: "Subaru Natsuki", ja-Latn: "Natsuki Subaru" } }
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
  names: { original: "川澄綾子", translations: { en: "Ayako Kawasumi" } }
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

- **Deferred: production credits & studios** — `Staff` currently covers only voice actors; crew
  credits (director, music, …) and a `Studio` model (organizations, also many-to-many) are future
  sibling work.

Settled during design: **scope granularity** — node-level (Season / Movie / Special) is the finest;
per-Episode is intentionally out (§2.2). **VA reverse-index** — wanted; an index/query over the
`voiceActors` edges by `staffId`, storing nothing new (§2.4). **Identity** — one node per AniList id
by default, merge differing ids only via curation override (§2.5).
