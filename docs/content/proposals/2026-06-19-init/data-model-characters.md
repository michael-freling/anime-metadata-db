---
title: "Characters Data Model"
date: 2026-06-19
weight: 3
---

# Characters Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)
**Related:** [Anime Series Data Model](../data-model-anime-series/) — the R1 spine this joins onto.

Characters are **R2** enrichment (research note §2.1). Unlike the franchise hierarchy — a strict
`Franchise → Series → Season → Episode` tree — a character cuts **across** it: the same character
appears in many Series and even many Franchises. So a `Character` is a **global** entity that
links **many-to-many** to `Series` in the [Anime Series Data Model](../data-model-anime-series/),
joined by the `externalIds` both models already carry.

> **Scope & storage.** This is R2. Per the research note's facts-vs-expression rule (§5.1a), we
> store **facts** — character IDs, names, and the appearance graph (which character is in which
> Series, in what role) — and **fetch expression live** (bios, descriptions, images), which
> AniList's ToS forbids warehousing. IDs and roles below are illustrative.

---

## Part 1 — The model

### 1.1 Entities

```text
Character               GLOBAL — owned by no Franchise or Series
  id
  names                 { english, romaji, native, aka[]? }
  externalIds           { anilistId, … }
  appearances[]         CharacterAppearance

CharacterAppearance     a Character ↔ Series link (the many-to-many edge)
  seriesId              the Series the character appears in — the PRIMARY association
  role                  MAIN | SUPPORTING | MINOR     (per appearance — see §2.3)
  voiceActors[]         { staffId, language }         (Staff is a sibling R2 model; can vary per series)
  seasonIds[]?          optional — narrow to specific Seasons/Movies if not the whole Series
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
| `appearances[]` | Character | The many-to-many links into the Series spine |
| `seriesId` | CharacterAppearance | The associated Series (primary association — §2.2) |
| `role` | CharacterAppearance | MAIN / SUPPORTING / MINOR — *per Series*, not global (§2.3) |
| `voiceActors[]` | CharacterAppearance | VA by language; varies across series (recasts, dubs) |
| `seasonIds[]` | CharacterAppearance | Optional finer scope when a character isn't in the whole Series |

Store the IDs, names, and appearance graph (facts); fetch bios/art live (research note §5.1a).

---

## Part 2 — Rules & concepts

### 2.1 Characters are global and many-to-many

- A `Character` is not owned by a Franchise or Series — it is its own node.
- The **same character appears in multiple Series** (Saber in *Fate/stay night* and *Fate/Zero*)
  and even **multiple Franchises** (a crossover character — §3.2). One `Character` node, many
  `appearances`.
- Identity is keyed by `externalIds.anilistId`: when several Series' character lists resolve to
  the same AniList character, they collapse to one `Character` with several appearances.

### 2.2 Associate at the Series level

The primary association is `CharacterAppearance.seriesId` — a **Series**, not a Season or
Episode — because a character is normally consistent across a Series' seasons and movies (Tanjiro
is in every Demon Slayer season + the films). Associating once per Series avoids repeating the
link on every Season.

Use the optional `seasonIds[]` only when a character genuinely appears in a *subset* of a Series
(a late-arriving character, a cameo confined to one cour). Default to whole-Series.

### 2.3 Role and voice actors are per-appearance, not global

A character's importance is **relative to the work**: a side character in the main series can be
the lead of a spinoff. So `role` lives on the appearance, not the character. Likewise
`voiceActors` — the VA differs by `language` (sub vs each dub) and can change between series
(recasts), so it belongs on the appearance. (`Staff` — voice actors and crew — is its own R2
sub-model, referenced here by `staffId` and out of scope for this doc.)

### 2.4 Storage: facts vs expression

| Data | Store? | Why |
|---|:--:|---|
| Character `id`, `externalIds`, `names` | ✅ | Facts / our keys |
| The appearance graph (`seriesId`, `role`, VA links) | ✅ | Factual associations we derive and own |
| Bios, descriptions, personality, images | ❌ fetch live | AniList "expression" — ToS forbids warehousing (research note §5.1a) |

---

## Part 3 — Worked examples

### 3.1 Saber — one Character, many Series in one Franchise

Saber appears across several *Fate* Series (see the Fate franchise in
[Anime Series Data Model §3.2](../data-model-anime-series/#32-fate--numbered-vs-date-ordered-series-in-one-franchise)).

```yaml
Character:
  id: artoria-pendragon
  names: { english: "Saber (Artoria Pendragon)", romaji: "Seibā", native: "セイバー" }
  externalIds: { anilistId: 497 }            # illustrative
  appearances:
    - { seriesId: fate-stay-night, role: MAIN, voiceActors: [ { staffId: ayako-kawasumi, language: ja } ] }
    - { seriesId: fate-zero,       role: MAIN, voiceActors: [ { staffId: ayako-kawasumi, language: ja } ] }
    # … also Fate/Grand Order, etc. — the SAME Character node, more appearances
```

One `Character`, multiple `appearances` into different Series of the **same** Franchise (Fate).

### 3.2 Subaru — the same Character across two Franchises

Crossovers make a character span Franchises. *Isekai Quartet* is a crossover anime; Subaru is a
lead in both the **Re:Zero** Franchise and the **Isekai Quartet** Franchise.

```yaml
Character:
  id: subaru-natsuki
  names: { english: "Subaru Natsuki", romaji: "Natsuki Subaru", native: "ナツキ・スバル" }
  externalIds: { anilistId: 119377 }          # illustrative
  appearances:
    - { seriesId: re-zero,        role: MAIN }   # Franchise: Re:Zero
    - { seriesId: isekai-quartet, role: MAIN }   # Franchise: Isekai Quartet  ← different franchise
```

The two appearance Series belong to different Franchises, so Subaru's *Franchises* (Re:Zero +
Isekai Quartet) are simply the distinct Franchises of his appearances — derived, never stored.

---

## Part 4 — Building the records

1. **Seed characters** from each media node's AniList character list — the media IDs are already
   in the [Anime Series Data Model](../data-model-anime-series/) (`externalIds.anilistId`).
2. **Dedup into one `Character`** per AniList character id, collecting an `appearance` per Series
   the character is found in (roll Season-level hits up to the Series unless a subset warrants
   `seasonIds[]`).
3. **Capture role + voice actors** per appearance from the AniList edge (`role`, `voiceActors` by
   language).
4. **Store** the character nodes + appearance graph + names (facts) next to the franchise records;
   **fetch** bios and images live at display time (never warehoused).
5. **Refresh** with the franchise pipeline; curation overrides win (e.g. merge alternate-version
   entries, fix a miscategorised role).

---

## Part 5 — Open questions

- **Appearance granularity** — default to Series-level; is `seasonIds[]` enough for subset cases,
  or do some characters need Movie/Episode-level scoping?
- **Character identity edge cases** — alternate-universe or "what-if" versions (a different-route
  Saber, a genderswapped variant) sometimes share and sometimes split AniList ids. Trust the id,
  or add a curation override to merge/split?
- **`role` stability** — MAIN/SUPPORTING is partly editorial (AniList-derived). Store as-is, or
  treat as a soft hint?
- **Staff / voice actors** — referenced here by `staffId`; the `Staff` sub-model (VAs + crew, also
  many-to-many) is a sibling doc, not designed yet.
