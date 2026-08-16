// Tests for check-provenance.mjs.
//
// The checker's whole value is that a green run means "nothing is missing", so
// the failure that matters is a false negative — passing while a field is
// undocumented. Two such bugs shipped and were caught in review; both have a
// regression test here.
//
// Run: node --test .claude/skills/update-data-provenance/scripts/

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname, resolve } from 'node:path'

const SCRIPT = resolve(import.meta.dirname, 'check-provenance.mjs')

// A minimal repo shaped like the real one: a record with a nested episode list
// and a shared externalIds block reached from two different parents.
const SCHEMA = {
  $schema: 'https://json-schema.org/draft/2020-12/schema',
  type: 'object',
  properties: { series: { $ref: '#/$defs/series' } },
  $defs: {
    id: { type: 'string' },
    series: {
      type: 'object',
      properties: {
        id: { $ref: '#/$defs/id' },
        seasons: { type: 'array', items: { $ref: '#/$defs/season' } },
        specials: { type: 'array', items: { $ref: '#/$defs/special' } },
        characters: { type: 'array', items: { $ref: '#/$defs/character' } },
      },
    },
    season: {
      type: 'object',
      properties: {
        id: { $ref: '#/$defs/id' },
        externalIds: { $ref: '#/$defs/externalIds' },
        episodes: { type: 'array', items: { $ref: '#/$defs/episode' } },
      },
    },
    special: {
      type: 'object',
      properties: {
        id: { $ref: '#/$defs/id' },
        episodes: { type: 'array', items: { $ref: '#/$defs/episode' } },
      },
    },
    character: {
      type: 'object',
      properties: { id: { $ref: '#/$defs/id' }, externalIds: { $ref: '#/$defs/externalIds' } },
    },
    episode: { type: 'object', properties: { airedNumber: { type: 'integer' } } },
    externalIds: { type: 'object', properties: { anidbId: { type: 'integer' } } },
  },
}

const TABLE = [
  '## Where each field comes from',
  '',
  '| Field | Filled from | Licence |',
  '|---|---|---|',
  '| `series.id`, `seasons[].id`, `specials[].id`, `*.characters[].id` | authored | ODbL |',
  '| `seasons[].externalIds.anidbId` | upstream | ODbL |',
  '| `*.characters[].externalIds.anidbId` | authored | ODbL |',
  '| `seasons[].episodes[].airedNumber`, `specials[].episodes[].airedNumber` | upstream | ODbL |',
  '',
  '## Next section',
  '',
].join('\n')

const NOTICE = 'Credits: example-source — https://example.com\n'
const CONFIG = 'sources:\n    a:\n        url: https://example.com/data.json\n'

function fixture(overrides = {}) {
  const dir = mkdtempSync(join(tmpdir(), 'provenance-'))
  const files = {
    'config/schemas/anime.schema.json': JSON.stringify(SCHEMA, null, 2),
    'web/content/docs/sources-and-licensing.mdx': TABLE,
    NOTICE,
    'config.yaml': CONFIG,
    ...overrides,
  }
  for (const [path, content] of Object.entries(files)) {
    if (content === null) continue
    const full = join(dir, path)
    mkdirSync(dirname(full), { recursive: true })
    writeFileSync(full, content)
  }
  return dir
}

// Returns {code, out}. The script writes findings to stdout and aborts to stderr.
function run(dir) {
  try {
    const out = execFileSync('node', [SCRIPT], { cwd: dir, encoding: 'utf8', stdio: 'pipe' })
    return { code: 0, out }
  } catch (e) {
    return { code: e.status, out: `${e.stdout ?? ''}${e.stderr ?? ''}` }
  }
}

const withSchema = (mutate) => {
  const s = structuredClone(SCHEMA)
  mutate(s)
  return { 'config/schemas/anime.schema.json': JSON.stringify(s, null, 2) }
}

test('passes when every field is documented under its container', () => {
  const dir = fixture()
  const { code, out } = run(dir)
  assert.equal(code, 0, out)
  assert.match(out, /all documented under their own container/)
  rmSync(dir, { recursive: true, force: true })
})

test('fails on a field absent from the table', () => {
  const dir = fixture(withSchema((s) => (s.$defs.season.properties.part = { type: 'integer' })))
  const { code, out } = run(dir)
  assert.equal(code, 1)
  assert.match(out, /seasons\.part/)
  rmSync(dir, { recursive: true, force: true })
})

// Regression: matching on the bare property name let `id`, documented under
// series/seasons/characters, vouch for an undocumented `id` elsewhere.
test('fails on a name collision across defs', () => {
  const dir = fixture(withSchema((s) => (s.$defs.episode.properties.id = { $ref: '#/$defs/id' })))
  const { code, out } = run(dir)
  assert.equal(code, 1, out)
  assert.match(out, /seasons\.episodes\.id/)
  assert.match(out, /specials\.episodes\.id/)
  rmSync(dir, { recursive: true, force: true })
})

// Regression: a $def reached from several parents under the same property name
// collapsed into one label, so documenting `seasons[].externalIds.anidbId`
// silently covered `characters[].externalIds.anidbId` too.
test('fails when only one parent of a shared def is documented', () => {
  const table = TABLE.replace('| `*.characters[].externalIds.anidbId` | authored | ODbL |\n', '')
  assert.notEqual(table, TABLE, 'test setup: the row was not removed')
  const dir = fixture({ 'web/content/docs/sources-and-licensing.mdx': table })
  const { code, out } = run(dir)
  assert.equal(code, 1, out)
  assert.match(out, /characters\.externalIds\.anidbId/)
  rmSync(dir, { recursive: true, force: true })
})

test('a *. wildcard covers every parent of a shared def', () => {
  const table = TABLE.replace(
    '| `seasons[].externalIds.anidbId` | upstream | ODbL |\n| `*.characters[].externalIds.anidbId` | authored | ODbL |',
    '| `*.externalIds.anidbId` | upstream | ODbL |',
  )
  assert.notEqual(table, TABLE, 'test setup: the rows were not replaced')
  const dir = fixture({ 'web/content/docs/sources-and-licensing.mdx': table })
  const { code, out } = run(dir)
  assert.equal(code, 0, out)
  rmSync(dir, { recursive: true, force: true })
})

test('sees inside an inline nested object', () => {
  const dir = fixture(
    withSchema(
      (s) =>
        (s.$defs.season.properties.broadcast = {
          type: 'object',
          properties: { network: { type: 'string' } },
        }),
    ),
  )
  const { code, out } = run(dir)
  assert.equal(code, 1, out)
  assert.match(out, /seasons\.broadcast\.network/)
  rmSync(dir, { recursive: true, force: true })
})

test('fails on a fetched source that NOTICE does not credit', () => {
  const dir = fixture({ NOTICE: 'Credits: nothing relevant\n' })
  const { code, out } = run(dir)
  assert.equal(code, 1)
  assert.match(out, /NOT CREDITED IN NOTICE/)
  rmSync(dir, { recursive: true, force: true })
})

test('flags a row for a field no schema defines', () => {
  const table = TABLE.replace('| `series.id`,', '| `seasons[].goneAway`, `series.id`,')
  assert.notEqual(table, TABLE, 'test setup: the row was not modified')
  const dir = fixture({ 'web/content/docs/sources-and-licensing.mdx': table })
  const { code, out } = run(dir)
  assert.equal(code, 1, out)
  assert.match(out, /seasons\.goneAway/)
  rmSync(dir, { recursive: true, force: true })
})

// Exit 2 is "could not compare", which must never be confused with a pass.
test('aborts with 2 on malformed schema JSON', () => {
  const dir = fixture({ 'config/schemas/anime.schema.json': '{ "broken": ' })
  const { code, out } = run(dir)
  assert.equal(code, 2)
  assert.match(out, /not valid JSON/)
  rmSync(dir, { recursive: true, force: true })
})

test('aborts with 2 when the table heading is renamed', () => {
  const table = TABLE.replace('## Where each field comes from', '## Field provenance')
  assert.notEqual(table, TABLE, 'test setup: the heading was not renamed')
  const dir = fixture({ 'web/content/docs/sources-and-licensing.mdx': table })
  const { code, out } = run(dir)
  assert.equal(code, 2)
  assert.match(out, /not found/)
  rmSync(dir, { recursive: true, force: true })
})

test('aborts with 2 when config.yaml declares no sources', () => {
  const dir = fixture({ 'config.yaml': 'settings:\n    dataDir: data\n' })
  const { code, out } = run(dir)
  assert.equal(code, 2)
  assert.match(out, /no source URLs/)
  rmSync(dir, { recursive: true, force: true })
})
