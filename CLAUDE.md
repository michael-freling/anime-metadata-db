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

## Working on the dataset

- `data/` is generated. Never hand-edit it — edit `builder/config/overrides/`
  and run `builder build`, then `make index`.
- Author only what the sources cannot express. Titles, dates, episode counts
  and `anilistId` are resolved by the build; a Series' own native title, its
  installment structure, and its `numbered:` decision are not.
- Before any rebuild, confirm the pinned sources still match their checksums.
  `builder init` re-pins a rolling source when upstream has moved, which mixes
  an unrelated data refresh into whatever change you are making.

## Checks that read the dataset

More things read the shape of a record than the Go compiler covers. After a
model or schema change, run all of these, not just the tests:

- `builder/config/schemas/*.json` — validated in CI by `Data lint`
- `.claude/skills/update-data-provenance/scripts/check-provenance.mjs` — **not
  in CI**; nothing else reconciles the schema against the licensing doc
- `.claude/skills/enforce-pagination/scripts/check-pagination.mjs`
- `TestCoveragePageMatchesTheDataset` — regenerates the coverage tables and
  prints the corrected markdown on failure
- the `-tags e2e` suites, which assert on generated YAML as text
