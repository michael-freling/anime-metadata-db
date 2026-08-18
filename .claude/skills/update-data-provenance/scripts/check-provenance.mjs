#!/usr/bin/env node
// Mechanical drift checks for the data provenance documentation.
//
// This script does NOT decide which source fills which field — that is a
// judgement call that has to be made by reading builder/internal/build. What it does is
// catch the two drifts that are purely mechanical, and therefore the two most
// likely to slip through review:
//
//   1. a field exists in the schemas but is absent from the provenance table
//   2. the build fetches a source that NOTICE does not credit
//
// Exit codes: 0 clean, 1 drift found, 2 the script could not run its checks
// (malformed input, or the table's heading was renamed). Never exits 0 without
// having actually compared something.

import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join } from 'node:path'

const ROOT = process.cwd()
const DOC = 'web/content/docs/sources-and-licensing.mdx'
const NOTICE = 'NOTICE'
const SCHEMA_DIR = 'builder/config/schemas'
const SOURCES_DIR = 'builder/internal/sources'
const BUILD_DIR = 'builder/internal/build'
const CONFIG = 'builder/config.yaml'
const SECTION = '## Where each field comes from'

// Anything that stops the comparison from happening is fatal. Passing because
// an input could not be read would be worse than failing.
function abort(msg, detail) {
  console.error(`FATAL: ${msg}`)
  if (detail) console.error(`  ${detail}`)
  process.exit(2)
}

function read(p) {
  try {
    return readFileSync(join(ROOT, p), 'utf8')
  } catch (e) {
    abort(`cannot read ${p}`, e.message)
  }
}

function readJSON(p) {
  const raw = read(p)
  try {
    return JSON.parse(raw)
  } catch (e) {
    abort(`${p} is not valid JSON`, e.message)
  }
}

// $defs that describe a scalar, not a record with fields of its own.
const SCALAR_DEFS = new Set(['id', 'date', 'releaseSeason', 'specialFormat'])

// $defs whose every field shares one provenance, so the doc covers them with a
// single row (`characters[].names`) instead of enumerating the inside. A
// property pointing at one of these is therefore a documentable leaf, and the
// fields within it are not checked individually.
//
// Adding a def here is a claim that everything inside it comes from one source
// under one licence. If you ever add a field to one of these that is filled
// from somewhere else, the def must come OUT of this list and be enumerated —
// no check will catch that for you.
const OPAQUE_DEFS = new Set([
  'title', // titles/names: all sub-fields filled together by fillTitles/fillNames
  'names',
  'voiceActor', // the cast structure is authored wholesale
  'appearance',
  'scopeRef',
  'watchOrder', // editorial, authored wholesale
  'alternateCutOf',
])

let failed = false
const fail = (msg) => {
  failed = true
  console.log(msg)
}

// ---------------------------------------------------------------------------
// 1. Every field the schemas allow must appear in the provenance table.
// ---------------------------------------------------------------------------

// The $def a property points at, if any, and whether it does so through an
// array. `{$ref: '#/$defs/season'}` and `{type:'array', items:{$ref:...}}` are
// the only two shapes the schemas use.
function refTarget(node) {
  const ref = node?.$ref ?? (node?.type === 'array' ? node.items?.$ref : undefined)
  if (!ref) return null
  return { def: ref.replace('#/$defs/', ''), array: Boolean(node.type === 'array') }
}

// An inline object literal (no $ref) that has fields of its own.
function inlineProperties(node) {
  return node?.properties ?? (node?.type === 'array' ? node.items?.properties : undefined)
}

// Enumerate the fields that need a row, qualified by the container they appear
// in. Matching on the bare property name would let a name documented under one
// container vouch for an undocumented field of the same name under another —
// the exact false negative this script exists to prevent — so every field
// carries the label(s) its container is referenced by.
function schemaFields() {
  const fields = []
  const opaque = []
  const skipped = []

  for (const file of readdirSync(join(ROOT, SCHEMA_DIR)).filter((f) => f.endsWith('.json'))) {
    const schema = readJSON(join(SCHEMA_DIR, file))
    const defs = schema.$defs ?? {}

    // Who references each $def, so a field can be qualified by the container it
    // actually appears under. A $def reached from several parents under the
    // same property name (externalIds from season/movie/character/staff,
    // episode from season and special) would otherwise collapse into one label
    // and hide that its provenance differs by parent.
    const parents = new Map()
    for (const [container, node] of [['(root)', schema], ...Object.entries(defs)]) {
      for (const [prop, sub] of Object.entries(node?.properties ?? {})) {
        const t = refTarget(sub)
        if (!t) continue
        if (!parents.has(t.def)) parents.set(t.def, [])
        parents.get(t.def).push({ parent: container, prop })
      }
    }

    // The property name a $def is referenced by — one segment, so a qualified
    // label never grows past `parent.prop`.
    const shortLabel = (def) => parents.get(def)?.[0]?.prop ?? def

    // Defs that appear under many kinds of node where what fills them differs
    // by node — always name the parent, even in a schema file where there
    // happens to be only one, so the paths mean the same thing across files.
    const ALWAYS_QUALIFY = new Set(['externalIds'])

    const labelsFor = (def) => {
      // An opaque parent is documented as a single row, so it contributes no
      // path of its own to the things inside it.
      const refs = (parents.get(def) ?? []).filter(
        (p) => p.parent === '(root)' || !OPAQUE_DEFS.has(p.parent),
      )
      if (!ALWAYS_QUALIFY.has(def)) {
        // A top-level record: the root is not a distinguishing context.
        const fromRoot = refs.find((p) => p.parent === '(root)')
        if (fromRoot) return [fromRoot.prop]
        if (new Set(refs.map((p) => p.parent)).size <= 1) {
          return [...new Set(refs.map((p) => p.prop))]
        }
      }
      return [...new Set(refs.map((p) => `${shortLabel(p.parent)}.${p.prop}`))]
    }

    const walk = (node, containerLabels, defName) => {
      for (const [prop, sub] of Object.entries(node?.properties ?? {})) {
        const t = refTarget(sub)

        // Points at a record whose own fields are documented individually — it
        // is structure, not data, and needs no row of its own.
        if (t && !OPAQUE_DEFS.has(t.def) && !SCALAR_DEFS.has(t.def)) {
          skipped.push({ file, prop, def: t.def })
          continue
        }

        // An inline object with fields of its own: recurse rather than treat it
        // as a leaf, or its contents would be invisible to the check.
        const inline = !t && inlineProperties(sub)
        if (inline) {
          walk({ properties: inline }, containerLabels.map((l) => `${l}${l ? '.' : ''}${prop}`), defName)
          continue
        }

        fields.push({ file, def: defName, prop, labels: containerLabels })
      }
    }

    // The root object has no container prefix; the doc writes its fields bare.
    walk(schema, [''], '(root)')

    for (const [def, node] of Object.entries(defs)) {
      if (SCALAR_DEFS.has(def) || !node?.properties) continue
      if (OPAQUE_DEFS.has(def)) {
        opaque.push({ file, def, count: Object.keys(node.properties).length })
        continue
      }
      const known = labelsFor(def)
      if (!known.length) {
        // Unreferenced def: no container name to qualify by. Say so rather than
        // silently matching on the bare name.
        console.log(`NOTE  $defs/${def} in ${file} is never referenced — matching its fields by bare name`)
      }
      walk(node, known.length ? known : ['*'], def)
    }
  }
  return { fields, opaque, skipped }
}

// Parse the field paths the doc declares into `container.prop` pairs, dropping
// the `[]` array marker so `seasons[].id` and `seasons.id` compare equal.
// `*.releaseDate` is a deliberate wildcard: the field means the same thing under
// every container, so the doc documents it once.
function parsePaths(spans) {
  const exact = new Set()
  const suffix = new Set()
  for (const span of spans) {
    const path = span.replace(/\[\]/g, '').trim()
    if (!/^[a-zA-Z*][a-zA-Z0-9.*]*$/.test(path)) continue
    if (path.startsWith('*.')) suffix.add(path.slice(2))
    else if (!path.includes('*')) exact.add(path)
  }
  return { exact, suffix }
}

// A field is documented if its qualified path is written out, or if a `*.`
// wildcard asserts the field means the same thing under every container it
// appears in.
const matches = (path, doc) =>
  doc.exact.has(path) || [...doc.suffix].some((s) => path === s || path.endsWith(`.${s}`))

const backticked = (text) => [...text.matchAll(/`([^`]+)`/g)].map((m) => m[1])

// Generous for the "is anything missing" direction: a path written anywhere in
// the section counts as documented.
const documentedPaths = (section) => parsePaths(backticked(section))

// Strict for the reverse direction, or prose in the "filled from" column ("the
// `ja` label becomes...") would read as a declared field. Only the first column
// of a table row declares one.
function tabulatedPaths(section) {
  const spans = []
  for (const line of section.split('\n')) {
    if (!line.startsWith('|')) continue
    const cell = line.split('|')[1] ?? ''
    if (/^\s*-+\s*$/.test(cell)) continue // separator row
    spans.push(...backticked(cell))
  }
  return parsePaths(spans)
}

const doc = read(DOC)
const start = doc.indexOf(SECTION)
if (start === -1) {
  abort(
    `section "${SECTION}" not found in ${DOC}`,
    'The section was renamed or removed — update this script to match.',
  )
}
const rest = doc.slice(start + SECTION.length)
const end = rest.indexOf('\n## ')
const section = end === -1 ? rest : rest.slice(0, end)

const documented = documentedPaths(section)
const tabulated = tabulatedPaths(section)
const { fields, opaque, skipped } = schemaFields()

// Every container a field appears under is its own path: a row covering
// `seasons[].externalIds.anidbId` says nothing about `characters[].externalIds`.
const pathsOf = (f) => f.labels.map((l) => (l ? `${l}.${f.prop}` : f.prop))

const expected = new Set(fields.flatMap(pathsOf))

const missing = fields.flatMap((f) => pathsOf(f).filter((p) => !matches(p, documented)).map((p) => ({ ...f, path: p })))
if (missing.length) {
  fail(`\nFIELDS MISSING FROM THE PROVENANCE TABLE (${missing.length})`)
  for (const f of missing) {
    // Plain dotted form: the matcher ignores `[]`, so the doc may add the array
    // markers back where they read better.
    fail(`  ${f.path.padEnd(40)} — ${f.file} ${f.def === '(root)' ? 'top level' : `$defs/${f.def}`}`)
  }
  fail(`\n  Each needs a row in "${SECTION}" saying what fills it and under`)
  fail('  which licence. Read builder/internal/build to find out; do not guess.')
  fail('  Use `*.field` only if it is filled the same way under every container.')
}

const stale = [
  ...[...tabulated.exact].filter((p) => !expected.has(p)),
  ...[...tabulated.suffix].filter((s) => ![...expected].some((p) => p === s || p.endsWith(`.${s}`))),
]
if (stale.length) {
  fail(`\nDOCUMENTED BUT NOT IN ANY SCHEMA (${stale.length})`)
  for (const p of stale) fail(`  ${p}`)
  fail('\n  Either the field was removed and the row is stale, or the first')
  fail('  column picked up something that is not a field path.')
}

if (!missing.length && !stale.length) {
  console.log(`OK  ${fields.length} schema fields, all documented under their own container`)
}

// Surface what was collapsed or skipped, so neither assumption silently
// swallows a newly added field.
if (opaque.length) {
  console.log('\nDocumented as one row each (sub-fields not checked individually):')
  for (const o of opaque) {
    console.log(`  ${o.def.padEnd(16)} ${String(o.count).padStart(2)} sub-fields — ${o.file}`)
  }
  console.log('  Verify these still share one provenance before trusting the OK above.')
}
if (skipped.length) {
  const names = [...new Set(skipped.map((s) => s.prop))].sort()
  console.log(`\nStructural properties, covered by the rows for what they contain: ${names.join(', ')}`)
}

// ---------------------------------------------------------------------------
// 2. Every source the build fetches must be credited in NOTICE.
// ---------------------------------------------------------------------------

// builder/config.yaml is the authoritative list of what the build actually downloads.
// A URL added there without a matching NOTICE entry is an attribution gap.
const notice = read(NOTICE)
const urls = [...read(CONFIG).matchAll(/^\s*url:\s*(\S+)/gm)].map((m) => m[1])
if (!urls.length) abort(`no source URLs found in ${CONFIG}`, 'The file format changed — update this script.')

// Reduce a URL to the token most likely to name it in NOTICE: for a GitHub or
// raw.githubusercontent URL that is the repository name, otherwise the host.
function creditToken(url) {
  let u
  try {
    u = new URL(url)
  } catch {
    return null
  }
  if (u.hostname === 'github.com' || u.hostname === 'raw.githubusercontent.com') {
    const [, , repo] = u.pathname.split('/')
    return repo || u.hostname
  }
  return u.hostname.replace(/^www\./, '')
}

const tokens = urls.map((url) => ({ url, token: creditToken(url) }))
const unparseable = tokens.filter((t) => !t.token)
for (const t of unparseable) fail(`\nUNPARSEABLE SOURCE URL in ${CONFIG}: ${t.url}`)

const credited = [...new Set(tokens.filter((t) => t.token).map((t) => t.token))]
const uncredited = credited.filter((t) => !notice.toLowerCase().includes(t.toLowerCase()))
if (uncredited.length) {
  fail(`\nSOURCES FETCHED BUT NOT CREDITED IN NOTICE (${uncredited.length})`)
  for (const t of uncredited) fail(`  ${t}`)
  fail('\n  NOTICE travels with every redistribution. An uncredited source is')
  fail('  a licensing defect, not a documentation nit.')
} else if (!unparseable.length) {
  console.log(`OK  ${credited.length} fetched sources, all credited in NOTICE`)
}

// A source adapter with no builder/config.yaml entry would be missed by the check
// above, so surface the package list too.
if (existsSync(join(ROOT, SOURCES_DIR))) {
  const pkgs = readdirSync(join(ROOT, SOURCES_DIR), { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name)
  console.log(`\nSource adapters in ${SOURCES_DIR}/: ${pkgs.join(', ')}`)
  console.log('  Confirm each maps to a NOTICE entry and to rows in the table.')
}

// ---------------------------------------------------------------------------
// 3. Where to verify each row. Informational — no pass/fail.
// ---------------------------------------------------------------------------

if (existsSync(join(ROOT, BUILD_DIR))) {
  const fillSites = []
  for (const file of readdirSync(join(ROOT, BUILD_DIR)).filter(
    (f) => f.endsWith('.go') && !f.endsWith('_test.go'),
  )) {
    const src = read(join(BUILD_DIR, file))
    for (const [, fn] of src.matchAll(/^func (?:\([^)]*\) )?((?:fill|assign|infer|Build)\w*)/gm)) {
      fillSites.push(`${BUILD_DIR}/${file}:${fn}`)
    }
  }
  if (fillSites.length) {
    console.log('\nBuilder fill sites — read these to verify the "filled from" column:')
    for (const s of fillSites) console.log(`  ${s}`)
  }
}

console.log(
  failed
    ? '\nFAILED — the provenance docs are out of date. See above.'
    : '\nMechanical checks passed. The source attribution itself still needs a read.',
)
process.exit(failed ? 1 : 0)
