---
name: enforce-pagination
description: Check that no API response can return a collection that grows with the dataset — every search paginates, and every embedded collection is bounded by the single entity that owns it. Use whenever api/proto/anime/v1/anime.proto or api/proto/browse/v1/browse.proto gains a repeated field or an RPC, whenever a Get* response embeds a new collection, whenever a UI page renders a list from the API, and whenever asked whether an endpoint scales or why a response is truncated.
---

# Keep every response bounded

This dataset is built to hold the full upstream catalogue — roughly 41,000
works, against the ~220 committed today. Every design decision in the API
follows from one rule:

> **No response may contain a collection whose size grows with the dataset.**

The storage rework (`api/internal/index`) exists to keep the *server* from holding
everything. That is only half the problem. A server that boots in 100 ms and
then serialises 40,000 rows into one response has simply moved the cost from
startup to request time, where the user is waiting for it.

The failure is quiet, which is why it needs a check. A list that is complete at
150 rows and truncated at 150,000 looks identical in code review, in tests
against the committed data, and on the page — right up until it doesn't.

## The two legal shapes

Every repeated field must be one of these. There is no third option.

**1. A paginated search.** Lives in a response alongside `next_page_token` and
`total_size`, with `page_token` and `limit` on the request. The caller controls
the size; the server never decides to send everything. This is the shape for
anything that returns *many entities* — `SearchSeries`, `SearchReleases`,
`ListCatalog`, `ListCharacters`, `ListStaff`.

**2. A collection bounded by its owning entity.** Lives in a `Get*` response and
is returned **whole**. `GetSeries` embeds every season, every episode, every
film, every special and the entire cast, because what bounds those is what one
series can be — the longest-running show alive is a four-figure episode count of
two integers each — and that does not move when the catalogue grows from 220
works to 41,000. The largest series in the dataset answers in 66 KB.

Each such field must be listed in `BOUNDED_BY_ENTITY` in the checker, **with the
reason**. "It is small today" is not a reason; "one series' cast" is. The list is
the judgement the mechanical check cannot make, so an entry with a vague reason
is the thing to push back on in review.

### Why not cap the embeds instead

That was the previous design, and the cap was the bug. A `Get*` used to truncate
at 25 with a sibling `<field>_total` giving the real count, which meant every
client had to read the total rather than the array length, and had to make a
second call to get the rest. Both were quietly skipped: a series page showed 100
of 148 cast members with nothing to indicate the other 48 existed, and capping
episodes at 25 turned "26 episodes" into "25 episodes" on the same page.

Returning one entity whole removes the class of bug rather than testing for it.
The rule that survives is the one that was actually load-bearing: **a response
that returns many entities must page**.

## 1. Run the mechanical check

From the repository root:

```bash
node .claude/skills/enforce-pagination/scripts/check-pagination.mjs
```

It checks both schemas — the public `anime.v1` and the internal `browse.v1`.
Being private buys no relief: the browse API is served from the same process
over the same dataset, so an unbounded response there is the same outage.

It fails (exit 1) on the structural violations:

- a `repeated` field with neither a `_total` sibling, nor a home in a paginated
  response, nor an entry in `BOUNDED_BY_ENTITY`
- a `List*Request` missing `page_token` or `limit`
- a `List*Response` missing `next_page_token` or `total_size`
- a `_total` field with no matching repeated field

The Go suite carries the runtime half, which the proto cannot express:
`TestNoResponseEmbedsAnUnboundedCollection` walks every response for every id in
the committed dataset by reflection and fails on any collection over the cap. It
covers fields added after this skill was written, because it reflects rather
than enumerating.

## 2. Judge whether the collection actually grows

This is the part no script can decide, and the part worth thinking about.

Ask: **can this collection grow as the catalogue grows?**

- A season's episodes — yes. Long-runners accumulate them indefinitely; the
  committed data already has a 170-episode season.
- A series' cast — yes. Already 148 for one series.
- A staff member's credits — yes. A working voice actor accumulates roles for a
  career.
- A franchise's series — yes, and it compounds: each series carries seasons,
  each season carries episodes.
- A character's voice actors — **no**. One per dub language, bounded by how many
  languages a show is dubbed into, not by how many shows exist.
- An appearance's scope refs — **no**. Bounded by the installments of one series.

If it grows, it needs both shapes: a List RPC, and a capped embed wherever a
`Get*` response mentions it. If it genuinely does not grow, add it to
`BOUNDED_BY_ENTITY` in the checker **with the reason**, so the next person
inherits the judgement instead of re-deriving it.

Prefer being wrong in the direction of paginating. A cursor on a collection that
turns out to be small costs one unnecessary field. An unbounded collection that
turns out to be large costs an outage.

## 3. Check the UI, not just the API

A bounded API does not give you a truthful page. Wherever the frontend renders a
collection it must either page it or say what it is not showing:

- Paging a single collection → `Pager` (`web/src/components/browse.tsx`), which
  prints `Showing N of M` and links onward.
- Several collections on one page → one cursor cannot describe them, so preview
  each with an honest count and link to the view that does page it. Both
  `/browse` (its type chips) and `/browse/[id]` (a franchise's several casts)
  do this.

`GetSeries` returns a series whole, so a series page renders `series.characters`
and `season.episodes` directly and an array's length **is** the count. What still
needs a pager is anything listing many entities — the `/browse` result sets.

## 4. When adding a new endpoint

- Request: `page_token` and `limit`.
- Response: `next_page_token` (empty on the last page) and `total_size` (the
  count of every match, not the page).
- Reject a malformed token rather than silently starting from the beginning — a
  corrupt cursor must not look like a first page.
- Page tokens are opaque (`api/internal/index`, base64) so the cursor scheme can
  change later without a wire break. Never document the format.
- Add the endpoint to `TestSearchEndpointsPageTheirWholeCollection`, which walks
  it to exhaustion and asserts every item appears exactly once and the reported
  total matches what paging produced, and to
  `TestSearchResponsesNeverReturnTheWholeCollection`, which asserts a `limit` is
  actually honoured.
