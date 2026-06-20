---
title: "Staff Data Model"
date: 2026-06-19
weight: 4
---

# Staff Data Model & Worked Examples

**Date:** 2026-06-19
**Author:** Michael Freling (with Claude Code)
**Status:** Design input — companion to [Anime Series/Franchise Metadata Research](../anime-metadata-research/)
**Related:** [Anime Series Data Model](../data-model-anime-series/) (the R1 spine) ·
[Characters Data Model](../data-model-characters/) (where voice roles live).

Staff are the **real people** who make anime — voice actors and production crew. Like `Character`,
a `Staff` is **R2** enrichment, **global**, and linked **many-to-many** to the franchise spine. It
is the entity that `CharacterAppearance.voiceActors.staffId` (in the
[Characters Data Model](../data-model-characters/)) resolves to.

> **Scope & storage.** R2. Per facts-vs-expression (research note §5.1a), we store **facts** — staff
> IDs, names, and the credit graph (who did what job on which work) — and **fetch expression live**
> (bio, photo, links). Voice roles are *not* stored here; they live on the character side (§2.2).
> IDs below are illustrative.

---

## Part 1 — The model

### 1.1 Entities

```text
Staff                   GLOBAL real person — voice actor and/or production crew
  id
  names                 { native, romaji, english?, aka[]? }
  externalIds           { anilistId, … }
  credits[]             StaffCredit   — PRODUCTION roles only (voice roles live on Character, §2.2)

StaffCredit             a Staff ↔ Series/node production credit (the many-to-many edge)
  seriesId              the work — the rollup association (always)
  scope[]?              optional — specific media nodes when the credit isn't whole-Series,
                        each one of { seasonId } | { movieId } | { specialId } (common here — §2.3)
  role                  controlled production role — e.g. "Director", "Series Composition",
                        "Script", "Character Design", "Music", "Original Creator" (§2.4)
  externalIds?          optional per-credit override of the Staff's AniList id (split entries)
```

A `Staff` is not nested under a Series; it reaches the franchise model through its `credits`, each
naming a `seriesId` from the [Anime Series Data Model](../data-model-anime-series/). A staff
member's Franchises are **derived** — the distinct Franchises of their credited works — not stored.

### 1.2 Field reference

| Field | Entity | Why it exists |
|---|---|---|
| `names {native,romaji,english,aka}` | Staff | *澤野弘之* (native) vs *Hiroyuki Sawano* (romaji) |
| `externalIds.anilistId` | Staff | The staff id — the join key (and what `voiceActors.staffId` points to) |
| `credits[]` | Staff | The many-to-many production-credit links into the Series spine |
| `seriesId` | StaffCredit | The credited work — rollup association (§2.3) |
| `scope[]` | StaffCredit | Optional specific nodes — `{seasonId}`/`{movieId}`/`{specialId}` (§2.3) |
| `role` | StaffCredit | The factual job title — controlled vocabulary (§2.4) |
| `externalIds` | StaffCredit | Optional per-credit AniList id override for split entries |

Store IDs, names, and the credit graph (facts); fetch bios/photos live (research note §5.1a).

---

## Part 2 — Rules & concepts

### 2.1 Staff are global and many-to-many

Like characters: a `Staff` is its own node, and the **same person works on many works across many
Franchises** (§3.2). Identity is keyed by `externalIds.anilistId`; AniList staff edges across media
collapse to one `Staff` with many `credits`.

### 2.2 Voice roles vs production credits — they live in different places

A staff member does two kinds of work, kept apart so neither is duplicated:

- **Voice roles** (a VA voices a specific Character) are character-centric, so they live on the
  [Characters model](../data-model-characters/) as `CharacterAppearance.voiceActors → staffId`. To
  get "characters voiced by this Staff," query those edges by `staffId`. They are **not** repeated
  in `Staff.credits`.
- **Production credits** (director, composition, music, …) are not character-centric, so they live
  here as `Staff.credits`.

A pure voice actor therefore has an empty `credits[]` and is referenced only from the character
side; a composer has `credits` here and (usually) no voice edges.

### 2.3 Credits are often scoped to a specific Season or Movie

Crew **changes between seasons** far more than characters do (a new director for S2, a film with a
different composer), so `StaffCredit.scope[]` is used *more often* than character scoping — a credit
commonly targets a specific Season/Movie rather than the whole Series. Roll up to `seriesId` only
when the role really spans everything (e.g. "Original Creator").

### 2.4 `role` is a fact (unlike a character's role)

A character's main/supporting `role` was dropped as editorial ([Characters §2.3](../data-model-characters/)).
A staff member's role is the opposite — a **factual job title** (this person *directed* this work),
so it is stored. The value is a **controlled vocabulary** we normalize from AniList's free-text role
strings (folding "Chief Director" / "Director" / "Episode Director" decisions happen here).

### 2.5 Storage: facts vs expression

| Data | Store? | Why |
|---|:--:|---|
| Staff `id`, `externalIds`, `names` | ✅ | Facts / our keys |
| The credit graph (`seriesId`, `scope`, `role`) | ✅ | Factual job associations we derive and own |
| Voice roles | ⛔ elsewhere | Stored on the Character side, not duplicated here (§2.2) |
| Bio, photo, social links | ❌ fetch live | AniList "expression" — ToS forbids warehousing (§5.1a) |

---

## Part 3 — Worked examples

### 3.1 Ayako Kawasumi — a voice actor (credited from the character side)

```yaml
Staff:
  id: ayako-kawasumi
  names: { native: "川澄綾子", romaji: "Ayako Kawasumi" }
  externalIds: { anilistId: 95012 }     # illustrative
  credits: []                           # pure VA — no production crew credits
```

This is the `staffId` that Saber's appearance points at
([Characters §3.1](../data-model-characters/#31-saber--one-character-many-series-in-one-franchise):
`voiceActors: [ { staffId: ayako-kawasumi, language: ja } ]`). Her voice roles — Saber and many
others — are read from those character edges, **not** stored in `credits`.

### 3.2 Hiroyuki Sawano — a composer across Franchises

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

One `Staff`, `credits` spanning three different Franchises — so his Franchises are derived from the
credits, never stored. The Gundam credit is `scope`d to the *Unicorn* node within the UC Series
(§2.3), since he scored that entry specifically.

---

## Part 4 — Building the records

1. **Seed staff** from each media node's AniList *staff* edges (production roles) — the media IDs
   are already in the [Anime Series Data Model](../data-model-anime-series/) (`externalIds.anilistId`).
2. **Dedup into one `Staff`** per AniList staff id; collect a `credit` per work, `scope`d to the
   node when the role is season-specific (the common case — §2.3).
3. **Normalize roles** — fold AniList's free-text role strings into the controlled vocabulary (§2.4).
4. **Store** staff nodes + credits + names (facts) next to the franchise records; **fetch** bios and
   photos live. **Voice roles are not seeded here** — they come from the character pipeline.
5. **Refresh** with the franchise pipeline; curation overrides win.

---

## Part 5 — Open questions

- **Role vocabulary** — how tightly to normalize AniList's long free-text role list (Director vs
  Chief Director vs Episode Director vs Storyboard…)? A flat controlled set, or a hierarchy?
- **Credit granularity** — per-Episode credits exist on AniList (episode director, storyboard); is
  Season/Movie-level enough, or do we ever scope a credit to an Episode?
- **VA reverse-index** — "characters voiced by this Staff" is a query over the character edges
  (§2.2); do we materialize it for performance, or resolve on demand?
- **Studios** — studios are *organizations*, not people, so they are out of scope here; a separate
  `Studio` model (also many-to-many onto the spine) is a future sibling doc.
