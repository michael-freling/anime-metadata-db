---
title: Coverage
weight: 4
---


How much of anime history this dataset actually manages, by release year and
release season. Every number below is counted from the committed `data/`, so it
describes the dataset as it stands — not what the upstream sources contain.

## What is counted

A **work** is one node that maps to a real anime release: a Season, a Movie or a
Special. A Series or Franchise is *our* grouping and is never counted as a work,
so a five-season show contributes five works, not six.

Seasons carry a `releaseYear` **and** a `releaseSeason`; movies and specials
carry only a year, so they share one column.

## Works by year and season

| Year | Winter | Spring | Summer | Fall | Films & specials | Total | Episodes |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 2006 | 1 | — | — | — | — | 1 | 24 |
| 2011 | — | — | — | 1 | — | 1 | 13 |
| 2012 | — | 1 | — | — | — | 1 | 12 |
| 2013 | — | — | — | 1 | — | 1 | 75 |
| 2014 | — | — | — | 1 | — | 1 | 12 |
| 2015 | — | 1 | — | — | — | 1 | 51 |
| 2016 | — | 1 | — | — | — | 1 | 25 |
| 2017 | — | — | 1 | 1 | 1 | 3 | 182 |
| 2018 | — | 1 | — | 2 | — | 3 | 48 |
| 2019 | — | 2 | 1 | 2 | — | 5 | 139 |
| 2020 | — | 1 | 3 | 2 | 1 | 7 | 97 |
| 2021 | 3 | 1 | 1 | 1 | — | 6 | 76 |
| 2022 | 1 | 2 | 2 | 2 | — | 7 | 92 |
| 2023 | 3 | 4 | 4 | 3 | — | 14 | 194 |
| 2024 | 4 | 4 | 2 | 4 | — | 14 | 209 |
| 2025 | 3 | 3 | 3 | — | 1 | 10 | 110 |
| 2026 | 80 | 69 | — | — | — | 149 | 1870 |
| **Total** | **95** | **90** | **17** | **20** | **3** | **225** | **3229** |

## Totals

| | Count |
|---|---:|
| Franchises | 1 |
| Series | 152 |
| Seasons | 222 |
| Movies | 3 |
| Works (seasons + movies + specials) | 225 |
| Episodes | 3229 |

## Cast

The R2 layer — characters and the staff who voice them — is far thinner than the
structure, because it is bounded by what Wikidata (the only CC0 name source the
build may redistribute) actually records.

| | Count |
|---|---:|
| Series with at least one character | 47 of 152 |
| Characters | 647 |
| …of those, with a voice actor | 607 |
| Staff (voice actors) | 416 |
| Voice-actor links | 651, all `ja` |

Coverage is uneven in a way that tracks a title's popularity on Wikidata rather
than anything about the show: Slime carries 148 characters and Frieren 43, while
most series carry none at all. A series with no cast is normal, not a gap in the
build.

Only **Japanese** voice actors are recorded. Wikidata's `voice actor` (P725)
statements cover dub casts too, but a language is only trustworthy when the
statement says so — so a link is taken as Japanese when the statement is either
qualified `language of work or name` = Japanese, or carries no language
qualifier and the actor is a Japanese citizen. Anything qualified to another
language is left out rather than guessed at. Other languages can be added the
same way, into their own `staff/` file.

## How to read the shape of it

**Winter and Spring 2026 are complete for TV.** Every TV anime the pinned
sources place in those two seasons is in the dataset — 149 works. Nothing after
Spring 2026 is included: an announced but unaired season is left out until it
airs, so the dataset never carries a season with no episodes.

**Everything before 2026 is there because it is a prior season of something in
2026** (plus the hand-authored Demon Slayer and Fate entries). That is why the
earlier years are thin and uneven: 2023 has 14 works not because 2023 was
sampled, but because fourteen shows running in 2026 started or continued then.
A show like Frieren pulls its 2023 season in with it.

**Films and specials are barely covered.** Only three exist, all authored by
hand. The bulk import is TV-only: of the 580 Winter+Spring 2026 entries in the
offline database, only 246 carry an AniList id at all — and the OVA/ONA/SPECIAL
long tail, which the builder resolves facts by AniList id alone, is mostly
recaps and shorts without one.

## Regenerating this table

The numbers come from `data/` and nothing else, so they can be recomputed at any
time:

```python
import glob, yaml
from collections import Counter, defaultdict

works, episodes = defaultdict(Counter), defaultdict(int)
for path in sorted(glob.glob("data/series/*.yaml")):
    record = yaml.safe_load(open(path, encoding="utf-8"))
    series = record["franchise"]["series"] if "franchise" in record else [record["series"]]
    for one in series:
        for kind, bucket in (("seasons", None), ("movies", "FILM"), ("specials", "FILM")):
            for node in one.get(kind) or []:
                works[node.get("releaseYear")][bucket or node.get("releaseSeason") or "?"] += 1
                episodes[node.get("releaseYear")] += len(node.get("episodes") or [])

for year in sorted(y for y in works if y):
    print(year, dict(works[year]), episodes[year])
```

`GetHealth` on the [API]({{< relref "/docs/using-the-api" >}}) reports the
top-line totals (franchises, series, seasons, episodes) for a running server.
