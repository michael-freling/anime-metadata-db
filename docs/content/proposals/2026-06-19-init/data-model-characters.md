---
title: "Characters Data Model"
date: 2026-06-19
weight: 3
---

# Characters Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)
**Related:** [Anime Series Data Model](../data-model-anime-series/) (the R1 spine) ·
[Staff Data Model](../data-model-staff/) (voice actors + crew that `staffId` resolves to).

Characters are **R2** enrichment (research note §2.1). Unlike the franchise hierarchy — a strict
`Franchise → Series → Season → Episode` tree — a character cuts **across** it: the same character
appears in many Series and even many Franchises. So a `Character` is a **global** entity that
links **many-to-many** to `Series` in the [Anime Series Data Model](../data-model-anime-series/),
joined by the `externalIds` both models already carry.

> **Scope & storage.** This is R2. Per the research note's facts-vs-expression rule (§5.1a), we
> store **facts** — character IDs, names, the appearance graph (which character is in which
> Series), and voice-actor *associations* — and **fetch expression live** (role, bios, images),
> which AniList's ToS forbids warehousing. IDs below are illustrative.

---

## Part 1 — The model

### 1.1 Entities

```text
Character               GLOBAL — owned by no Franchise or Series
  id
  names                 { english, romaji, native, aka[]? }
  externalIds           { anilistId, … }
  voiceActors[]         DEFAULT cast: { staffId, language } — the usual VA across appearances (§2.3)
  appearances[]         CharacterAppearance

CharacterAppearance     a Character ↔ Series link (the many-to-many edge)
  seriesId              the Series the character appears in — the rollup association (always)
  scope[]?              optional — specific media nodes when NOT the whole Series,
                        each one of { seasonId } | { movieId } | { specialId }
  voiceActors[]?        optional — override the default cast (a recast, or an added dub) (§2.3)
  externalIds?          optional per-appearance override of the Character's AniList id, for
                        media where AniList lists this character under a different id (§2.4)
```

A `Character` is **not** nested under a Series. It stands alone and reaches into the franchise
model through its `appearances`, each of which names a `seriesId` from the
[Anime Series Data Model](../data-model-anime-series/). A character's Franchises are **derived** —
the distinct Franchises of the Series it appears in — not stored.

### 1.2 Field reference

| Field | Entity | Why it exists |
|---|---|---|
| `names {english,romaji,native,aka}` | Character | *Saber* (en) vs *Seibā* (romaji); `aka` for aliases/spoiler names |
| `externalIds.anilistId` | Character | The character id — the join key and live-fetch handle |
| `voiceActors[]` | Character | Default cast — `{ staffId, language }` per language, shared across appearances (§2.3) |
| `appearances[]` | Character | The many-to-many links into the Series spine |
| `seriesId` | CharacterAppearance | The associated Series — the rollup association (§2.2) |
| `scope[]` | CharacterAppearance | Optional specific nodes — `{seasonId}`/`{movieId}`/`{specialId}` — when not the whole Series (§2.2) |
| `voiceActors[]` | CharacterAppearance | Optional override of the default cast (recast / added dub) (§2.3) |
| `externalIds` | CharacterAppearance | Optional per-appearance AniList id override for split/alternate cases (§2.4) |

Store the IDs, names, appearance graph, and VA associations (facts); fetch role, bios, and art
live (research note §5.1a).

---

## Part 2 — Rules & concepts

### 2.1 Characters are global and many-to-many

- A `Character` is not owned by a Franchise or Series — it is its own node.
- The **same character appears in multiple Series** (Saber in *Fate/stay night* and *Fate/Zero*)
  and even **multiple Franchises** (a crossover character — §3.2). One `Character` node, many
  `appearances`.
- Identity is keyed by `externalIds.anilistId`: when several Series' character lists resolve to
  the same AniList character, they collapse to one `Character` with several appearances.

### 2.2 Where a character appears: Series, or specific nodes

The rollup association is `CharacterAppearance.seriesId` — a **Series**, not a Season or Episode —
because a character is normally consistent across a Series' seasons and movies (Tanjiro is in
every Demon Slayer season + the films). Associating once per Series avoids repeating the link on
every node.

When a character appears in only *part* of a Series, narrow it with `scope[]`. A scope entry is a
typed reference to any media node — a **Season**, a **Movie**, or a **Special** — so a character
who debuts in one film or a single OVA is captured precisely:

```yaml
scope: [ { movieId: ds-infinity-castle-1 } ]        # only in this film
scope: [ { seasonId: ds-entertainment-district } ]  # only this cour
```

Omit `scope` for the whole-Series default.

### 2.3 Voice actors: default on the Character, overridable per appearance

A character usually keeps the **same voice actor across series** — Saber is Ayako Kawasumi in
every Fate work — so the default cast lives on the `Character` as `voiceActors[]`, one entry per
`language` (Japanese, plus each dub). When an appearance **recasts** the role or **adds a dub**, it
overrides with its own `voiceActors[]`; otherwise it inherits the default.

`staffId` references a **`Staff`** entity (voice actors + crew) keyed by its AniList staff id — its
own R2 sub-model, many-to-many like characters; see the [Staff Data Model](../data-model-staff/).
We store the VA *association* (who voices whom, by language — a fact); the staff member's bio and
photo are expression, fetched live.

> **No `role` field.** Main/supporting is an *editorial* per-media classification (AniList
> contributors assign it), not a fact about the world, and it ships with the character payload we
> fetch live anyway — so it is intentionally **not** stored.

### 2.4 Per-appearance id overrides (the AniList exception)

We unify a character under one `Character` (our `id`) even when AniList doesn't. AniList sometimes
lists the "same" character under **different character ids** in different media — alternate-route or
alternate-form versions, or plain data splits. When that happens, the appearance carries its own
`externalIds` so the live fetch for that media hits the right AniList node, while everything still
rolls up to one `Character`:

```yaml
appearances:
  - { seriesId: fate-stay-night }                              # inherits the Character's canonical id
  - { seriesId: fate-grand-order,
      externalIds: { anilistId: 498 } }                        # this media lists a different id
```

Default: omit `externalIds` and inherit the Character's canonical id.

### 2.5 Storage: facts vs expression

| Data | Store? | Why |
|---|:--:|---|
| Character `id`, `externalIds`, `names` | ✅ | Facts / our keys |
| The appearance graph (`seriesId`, `scope`, id overrides) | ✅ | Factual associations we derive and own |
| Voice-actor links (`staffId` + `language`; default + overrides) | ✅ | Factual "who voices whom" |
| `role` (main/supporting) | ❌ fetch live | Editorial per-media classification, not a fact (§2.3) |
| Bios, descriptions, images (character & staff) | ❌ fetch live | AniList "expression" — ToS forbids warehousing (§5.1a) |

---

## Part 3 — Worked examples

### 3.1 Saber — one Character, many Series in one Franchise

Saber appears across several *Fate* Series (see the Fate franchise in
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

One `Character`, multiple `appearances` into different Series of the **same** Franchise (Fate),
with the voice actor stated once as the default.

### 3.2 Subaru — the same Character across two Franchises

Crossovers make a character span Franchises. *Isekai Quartet* is a crossover anime; Subaru is a
lead in both the **Re:Zero** Franchise and the **Isekai Quartet** Franchise.

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

The two appearance Series belong to different Franchises, so Subaru's *Franchises* (Re:Zero +
Isekai Quartet) are simply the distinct Franchises of his appearances — derived, never stored.

---

## Part 4 — Building the records

1. **Seed characters** from each media node's AniList character list — the media IDs are already
   in the [Anime Series Data Model](../data-model-anime-series/) (`externalIds.anilistId`).
2. **Dedup into one `Character`** per AniList character id, collecting an `appearance` per Series
   (roll node-level hits up to the Series; keep `scope[]` only for subset cases). Record a
   per-appearance `externalIds` override when a media uses a different character id (§2.4).
3. **Resolve voice actors** from the AniList edges: the VA shared across appearances becomes the
   `Character.voiceActors` default; per-series differences (recasts, dubs) become appearance
   overrides.
4. **Store** the character nodes + appearance graph + names + VA links (facts) next to the
   franchise records; **fetch** role, bios, and images live at display time (never warehoused).
5. **Refresh** with the franchise pipeline; curation overrides win (merge alternate-version
   entries, fix a miscategorised appearance).

---

## Part 5 — Open questions

- **Scope granularity** — `scope[]` reaches Season / Movie / Special; is anything finer
  (per-Episode) ever needed, or is node-level enough?
- **Merge vs split identity** — the per-appearance `externalIds` override (§2.4) lets one
  `Character` span differing AniList ids, but *deciding* whether an alternate-form / what-if
  version is the same character is a curation call — what's the default policy?
- **Staff model** — `staffId` resolves to a node in the [Staff Data Model](../data-model-staff/)
  (voice actors + crew, many-to-many). A `Studio` model (organizations, not people) is still future.
