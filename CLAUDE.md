# anime-metadata-db

## What the catalogue carries

**Only TV series and films.** When importing from the open sources, take
upstream entries of type `TV` and `MOVIE` and nothing else. `SPECIAL`, `ONA`
and `OVA` entries are out of scope and must not be imported.

Why: upstream indexes far more than anime releases. Of the 1,711 entries it
lists for 2025, 1,067 are `SPECIAL`/`ONA`/`OVA`, and 488 of those carry no
`relatedAnime` at all — nothing to attach them to. They are music videos, OP
animations, brand-collaboration shorts, motion comics and promo clips:

    1ep  Ado: ROCKSTAR
    1ep  2025 Arknights Music Synesthesia - Yiqu Fengbei
    1ep  "Odekake Kozame" Kurashiki-shi Collaboration Movie
    0ep  #AdVSJus Motion Comic

With no parent to hang them on, each would become its own Series with its own
id and override file. That is a mirror of an index, not a catalogue of anime.

The `Special` node type still exists in the model, and an OVA or special that
genuinely belongs to a series may be authored by hand onto it. The rule is
about *bulk import*, not about the model.

## Repository layout

`dataset/` is the product and carries the ODbL/DbCL licence; everything else is
MIT. Keep that line clean — anything that is a record belongs under `dataset/`.

    dataset/data/        generated records; never hand-edited
    dataset/overrides/   authored records; the source of truth
    dataset/schemas/     validates both
    src/animedb/         shared Go module: model types + the dataset loader
    src/api/             Connect API server (Go module)
    src/builder/         the CLI that writes dataset/data/ (Go module)
    src/web/             docs site (Next.js)

There is no Go module at the repository root. The three modules are siblings
under `src/`, and `src/api` and `src/builder` both `replace` the shared module
to `../animedb`. `model` is public rather than internal because it crosses a
module boundary.

## Where the API reads the dataset

Two sources, because the two environments differ:

- **In a checkout** — tests, `go run ./cmd/api`, the Playwright suite — it is
  read from disk. `src/api/cmd/api` walks up for `dataset/data/index.tsv`;
  `-dataset` or `ANIME_DATASET_DIR` overrides it.
- **Deployed** it is fetched over HTTP from the commit the deployment was built
  from, derived from `VERCEL_GIT_*`. `-dataset-url` or `ANIME_DATASET_URL`
  overrides it.

Disk wins over the deployed commit, so a working tree is always authoritative
locally.

The fetched index is what a cold start pays for, and it grows with the
catalogue: 2.21 MB at 8,494 series. **Its size is not what the time is, though.**
Measured over five samples each, the 2.21 MB index and the 1.74 MB one it
replaced both return in about 0.17s warm, and the first fetch of a
freshly-pushed commit costs 0.3-0.4s. An earlier note here read "1.74 MB and
about 0.8s ... against 0.25s when it was 286 KB" and drew a line through two
points that were really measuring how warm the CDN was. If a cold start needs
to get faster, measure before assuming the index is why; a record is a separate
fetch of a few hundred bytes, and request count may matter more than bytes. The
answer is still caching at the edge or a different store rather than a smaller
index, which is what the API serves listings from.

**Vercel's Go builder ships the compiled binary and nothing else.** Repository
files reach the build — that is what `.vercelignore` governs, and why
`replace ../animedb` resolves — but they never reach the running function. Do
not write code, comments or checks that assume a deployment can read a file
beside the executable; that belief took production down once already.

Because a deployed instance fetches, every test that matters must run the binary
in the shape the deployment runs it. That is
`TestTheBinaryServesWithNoRepositoryOnDisk` in `src/api/cmd/api`: it builds the
binary and runs it from an empty directory with no repository anywhere. Every
other suite runs inside a checkout and cannot see this class of failure.

## Importing a year

`builder import --year YYYY` writes override files for one year of upstream and
builds nothing, so the result is reviewable as a diff of the authored layer.
Then `make build-data` and `make index`.

It refuses more than it writes, on purpose:

- **A work related to a series the catalogue already has is reported, never
  written — unless no series it is related to is named after it.** Creating a
  second series for an installment of an existing one is the only import
  mistake nothing downstream can detect, so this is the rule to be careful
  with. But "related" is not "an installment of": "Healin' Good♡Precure" is a
  Precure season in its own right and links to other Precure, and holding it
  for a merge nobody could make lost it. 1,699 works were in that position.

  So a link means a merge where one of the linked series' own names is this
  work's name carried further, and otherwise the work is written as the series
  it is. `accountedFor` asks that, and three things keep it honest: the
  comparison keeps the spaces and punctuation `FoldTitle` drops, so a name only
  accounts for a work at a boundary ("Free" accounts for "Free Fall", "Hero"
  does not account for "Heroman"); the names are read from the authored layer
  as well as the built records, since a run importing several years before a
  build would otherwise see none for the years it just wrote; and where nothing
  is known about a linked series' names the work is **reported**, because that
  costs a line in a report somebody reads and the other direction costs a
  duplicate nobody can find.

  `builder merge` asks the same question with the same inputs. It did not, and
  the two commands disagreed about what a candidate was: works `import` writes
  as their own series were still proposed as merges and then refused, which
  inflated the backlog figure from 643 to 2,290.

  **The failure mode this leaves is a sequel retitled so that it shares no name
  with its parent**, which is written as its own series. No title rule
  separates that from a series in its own right that merely links to its
  siblings, and `anilist-claimed-elsewhere` cannot see it because the two are
  different works with different ids. It is asserted as deliberate in
  `TestARetitledSequelIsWrittenAsItsOwnSeriesWhichIsTheKnownFailureMode`, and
  what bounds it is that `import` builds nothing: the result is a reviewable
  diff of the authored layer, which is the whole reason the command is split
  from the build.
- **A series the build could not fill is reported.** The build refuses a kind
  unless the number of upstream candidates equals the number of declared
  installments, so a series must declare exactly what its own title resolves
  to. Two films under one title cannot resolve at all (a film carries no number
  to order by), and upstream often carries two un-merged records for the same
  film.
- **An id already in use is reported.** Every id in the dataset shares one
  namespace, so a slug has to be free against seasons and films too, not just
  against series filenames.
- **A title that romanises to nothing is reported.** `Slug` keeps `[a-z0-9]`,
  and 熊野古道民話集 has none, so the id, the filename and every work id under
  it come out empty. Choosing a romanisation for one is a person's call.
- **Two indistinguishable upstream records of one work are reported.** See
  below.

Upstream lists some works twice: once as a merged record carrying an AniList id,
once under the English title carrying none, agreeing on type, quarter and
episode count. "Amaama to Inazuma" beside "Sweetness & Lightning", "Ansatsu
Kyoushitsu" beside "Assassination Classroom". The build already knows this — it
resolves a kind against the merged half first and consults the rest only where
that half cannot account for the installments — so **the importer declares the
count the build will resolve, not the size of the raw pool.** Declaring from the
raw pool forced the fallback and made the second record a season of its own: 64
series carried a phantom season and 1,414 episodes were counted twice. Where
neither copy carries an id there is nothing to prefer, and the series is
refused.

`builder merge` attaches the works that report names, and only where the
evidence settles it: upstream must link the work to the series (sharing a title
is not a link — that is also what a homonym does), the work's own title must be
the series' title carried further, and a season is appended only when it aired
after every installment already there, so the claim stays "a later installment"
rather than a position inside the run. Attaching to a kind also pins the
resolved ids of the slots already in it, because the build matches a kind by
candidate count and a new slot changes it. Of 2,468 reported, 115 met all of
that; the rest are still a person's work. Even then a spin-off can read as a
sequel — six of 52 did, and were removed by hand after reading the list — so
the attached seasons are worth reading rather than trusting.

`relatedAnime` is not a grouping primitive. Followed transitively it puts 5,597
entries in one component — Evangelion, Oshi no Ko and brand-collab shorts
together — because promo specials bridge unrelated franchises. Series boundaries
are authored here; grouping is by the title the build itself resolves against.

**One upstream work belongs to one series.** `anilist-claimed-elsewhere` gates
on that, and nothing asked it until 166 AniList ids turned out to be claimed by
two series each, across 315 of 9,711 series. Both shapes were the same mistake:
a sequel given a series of its own while already being a season of its parent,
and a work upstream lists under two titles differing only in width or
punctuation (しまじろうのわお！ against しまじろうのわお!). So the import folds
titles before comparing them (`importer.FoldTitle`), and reads the ids the
authored layer already accounts for as well as the built ones — a run that
imports several years before any build cannot otherwise see what the earlier
years claimed.

## Checks must fail differently from "nothing happened"

Every figure derived from the dataset grows, and anything that wrote one down
as a literal broke on the 2010-2019 import — five times, each one reported as
the wrong problem:

- a paging test capped its loop at 40 pages, so a 41-page catalogue read as
  "paging lost 23 entries" rather than "the loop stopped early";
- the coverage page's tables and two e2e match counts froze figures that had
  since moved, and failed as though the filters were broken;
- the dataset-diff job built a comment larger than GitHub's 64 KiB limit and
  failed after the diff had succeeded;
- a CI poller hit GitHub's unauthenticated 60-requests/hour limit and reported
  the error body as "0 checks seen", indistinguishable from CI not having
  started. That one cost the most, because it sent the search in the wrong
  direction entirely.

So: assert the relation, not the number. A filter narrows (`0 < filtered <
all`); a pager stops because it ran out of pages, which is a separate assertion
from the count; a budget derives from the data it measures. And when a check
cannot do its job, say that — a zero that means "I could not look" has to read
differently from a zero that means "I looked and found none".

The same applies to reading CI. Use the authenticated GitHub tooling rather
than bare `curl`: 60 requests an hour is about thirty minutes of polling.

A check can also be reading a fraction of what it claims to. `anilist-unlinked`
walks `relatedAnime` to confirm an installment is linked to its siblings, and
followed anilist.co URLs alone — **22,546 of upstream's 443,373** of them. The
rest name a neighbour by its AniDB, MyAnimeList or anime-planet URL, so two
works that were each other's only neighbour both failed a *gating* check against
correct data. When a check walks someone else's graph, measure how much of it
the walk can actually see.

The edges are fixed; the **endpoints** are still AniList ids. The walk steps
through an entry carrying no AniList id, but it is seeded from one and matches
one, so an installment the build reached by its `anidbId` alone — 385 of the
9,280 carrying any id — is not something `anilist-unlinked` can check. That is
said rather than hidden: the provenance line's fractions are all out of the
6,688 AniList ids, and the installments with none are reported beside them as
their own figure, so the denominator never quietly absorbs a population the
check did not measure. Those installments are still gated by
`anilist-claimed-elsewhere`, which keys on both providers.

The same goes for a word list. Four series ended up named after a recut of
themselves, because `NativeTitle` takes the longest kana synonym and
劇場総集編ぼっち・ざ・ろっく！ Re:Re: is longer than ぼっち・ざ・ろっく！. The
obvious fix — prefer a synonym carrying no 総集編/特別編集版/新編集版/完全版/
ディレクターズカット — was measured before it was written, and it changes 11
titles: about four improve, four move sideways, and **three get worse.**
"HIGHLANDER ハイランダー 〜ディレクターズカット版〜" becomes ハイランダー,
losing the subtitle, and "PSYCHO-PASS サイコパス 新編集版" becomes
"PSYCHO-PASS サイコパス", which is already another series' name. 前編 and 後編
are worse still: there the marker *is* the work's name, since the halves are
different works.

So the four were authored by hand (date-a-live, bocchi-the-rock,
girls-band-cry, re-zero) and the rule was not written. A marker means "a recut
of something" for one series and "this work's name" for the next, and nothing
in the sources says which.

A seed carrying no id is not a signal either. Of the nine series the 2020-2026
re-import proposed, the three carrying an id on some provider were all genuine
and the six carrying none were all wrong — a recap of One Punch Man's second
season, the English-titled listing of the Sailor Moon Eternal films, a
romanisation twin (`Sinbi Apartment: Ghost Ball Zero` beside the id-bearing
`Sinbi Apateu: Ghost Ball ZERO`, which `FoldTitle` cannot pair because
"Apartment" and "Apateu" are not the same string), two named parts of a trilogy
upstream also lists as a merged three-episode record and as three un-merged
ones, and a second season of a parent already here. A work carrying no id
cannot be checked by `anilist-claimed-elsewhere` either, so writing it as a
series is exactly the duplicate nothing downstream detects.

Nine for nine, and still not a rule: **3,775 of 10,032 series carry no id
anywhere in their record**, 37.6% of the catalogue. Upstream simply does not
index much of what it lists — older titles, Chinese and Korean shows — so
refusing an id-less seed would refuse a third of the catalogue to catch six
duplicates. Those six were deleted by hand, and a later import will propose
them again, because nothing in the sources separates them from the 3,775.

And resist a threshold the data does not have. Twelve series numbered separate
adaptations end to end — Doraemon across 1973, 1979 and 2005 — but no gap
cutoff separates them from real continuations: Re:Zero pauses eight years and
Blade of the Immortal's eleven-year gap is a remake. Those twelve were authored
by hand and the borderline ones listed for a read, because a cutoff here would
be one more number that fails differently from what it measures.

## Working on the dataset

- `dataset/data/` is generated. Never hand-edit it — edit `dataset/overrides/`
  and run `make build-data`, then `make index`.
- Author only what the sources cannot express. Titles, dates, episode counts
  and `anilistId` are resolved by the build; a Series' own native title, its
  installment structure, and its `numbered:` decision are not.
- Before any rebuild, confirm the pinned sources still match their checksums.
  `builder init` re-pins a rolling source when upstream has moved, which mixes
  an unrelated data refresh into whatever change you are making.

## Checks that read the dataset

More things read the shape of a record than the Go compiler covers. After a
model or schema change, run all of these, not just the tests:

- `dataset/schemas/*.json` — validated in CI by `Data lint`
- `.claude/skills/update-data-provenance/scripts/check-provenance.mjs` — **not
  in CI**; nothing else reconciles the schema against the licensing doc
- `.claude/skills/enforce-pagination/scripts/check-pagination.mjs`
- `TestCoveragePageMatchesTheDataset` — regenerates the coverage tables and
  prints the corrected markdown on failure
- the `-tags e2e` suites, which assert on generated YAML as text

Run them with `-count=1`. These read `dataset/` at run time, which Go's test
cache does not track, so a pass gets replayed against data that has since
changed — the coverage page looked current while stale exactly this way. CI is
unaffected, since it builds from a cold cache.
