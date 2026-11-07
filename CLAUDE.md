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
