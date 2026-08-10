#!/usr/bin/env node
// Mechanical drift checks for the data provenance documentation.
//
// This script does NOT decide which source fills which field — that is a
// judgement call that has to be made by reading internal/build. What it does is
// catch the two drifts that are purely mechanical, and therefore the two most
// likely to slip through review:
//
//   1. a field exists in the schemas but is absent from the provenance table
//   2. the build fetches a source that NOTICE does not credit
//
// Exit code 1 if either check fails, so this can be wired into CI later.

import { readFileSync, readdirSync, existsSync } from 'node:fs'
import { join } from 'node:path'

const ROOT = process.cwd()
const DOC = 'docs/content/docs/sources-and-licensing.md'
const NOTICE = 'NOTICE'
const SCHEMA_DIR = 'config/schemas'
const SOURCES_DIR = 'internal/sources'
const BUILD_DIR = 'internal/build'
const CONFIG = 'config.yaml'
const SECTION = '## Where each field comes from'

const read = (p) => readFileSync(join(ROOT, p), 'utf8')

// $defs that describe a scalar, not a record with fields of its own. They have
// no properties to document.
const SCALAR_DEFS = new Set(['id', 'date', 'releaseSeason', 'specialFormat'])

// $defs whose every field shares one provenance, so the doc covers them with a
// single row (`characters[].names`) instead of enumerating the inside. Their
// sub-fields are deliberately not checked.
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

// Collect {schema, def, prop} for every property the schemas define.
function schemaFields() {
  const out = []
  const opaque = []
  for (const file of readdirSync(join(ROOT, SCHEMA_DIR)).filter((f) => f.endsWith('.json'))) {
    const schema = JSON.parse(read(join(SCHEMA_DIR, file)))
    const containers = { '(root)': schema, ...(schema.$defs ?? {}) }
    for (const [def, node] of Object.entries(containers)) {
      if (SCALAR_DEFS.has(def) || !node?.properties) continue
      if (OPAQUE_DEFS.has(def)) {
        opaque.push({ file, def, count: Object.keys(node.properties).length })
        continue
      }
      for (const prop of Object.keys(node.properties)) out.push({ file, def, prop })
    }
  }
  return { fields: out, opaque }
}

// The doc's field paths are abbreviated relative to their container
// (`seasons[].releaseYear`, `externalIds.tvdbId`), not absolute from the root,
// so match on the bare property name rather than trying to model the path
// convention. Generous by design: a name mentioned anywhere in the section
// counts as documented, because the goal is catching what is missing outright.
function documentedNames(section) {
  const names = new Set()
  for (const [, span] of section.matchAll(/`([^`]+)`/g)) {
    for (const part of span.split(/[.\[\]*\/\s,]+/)) {
      if (/^[a-zA-Z][a-zA-Z0-9]*$/.test(part)) names.add(part)
    }
  }
  return names
}

// The reverse direction needs to be strict, or prose in the "filled from"
// column ("the `ja` label becomes...") reads as a documented field. Only the
// first column of a table row declares a field.
function tabulatedNames(section) {
  const names = new Set()
  for (const line of section.split('\n')) {
    if (!line.startsWith('|')) continue
    const cell = line.split('|')[1] ?? ''
    if (/^\s*-+\s*$/.test(cell)) continue // separator row
    for (const [, span] of cell.matchAll(/`([^`]+)`/g)) {
      for (const part of span.split(/[.\[\]*\s,]+/)) {
        if (/^[a-zA-Z][a-zA-Z0-9]*$/.test(part)) names.add(part)
      }
    }
  }
  return names
}

const doc = read(DOC)
const start = doc.indexOf(SECTION)
if (start === -1) {
  console.error(`FATAL: section "${SECTION}" not found in ${DOC}`)
  console.error('The section was renamed or removed — update this script to match.')
  process.exit(2)
}
const rest = doc.slice(start + SECTION.length)
const end = rest.indexOf('\n## ')
const section = end === -1 ? rest : rest.slice(0, end)

const documented = documentedNames(section)
const tabulated = tabulatedNames(section)
const { fields, opaque } = schemaFields()

const missing = fields.filter((f) => !documented.has(f.prop))
if (missing.length) {
  fail(`\nFIELDS MISSING FROM THE PROVENANCE TABLE (${missing.length})`)
  for (const f of missing) {
    const where = f.def === '(root)' ? 'top level' : `$defs/${f.def}`
    fail(`  ${f.prop.padEnd(18)} — ${f.file} ${where}`)
  }
  fail(`\n  Each needs a row in "${SECTION}" saying what fills it and under`)
  fail('  which licence. Read internal/build to find out; do not guess.')
}

const known = new Set(fields.map((f) => f.prop))
const stale = [...tabulated].filter((n) => !known.has(n))
if (stale.length) {
  fail(`\nDOCUMENTED BUT NOT IN ANY SCHEMA (${stale.length})`)
  for (const n of stale) fail(`  ${n}`)
  fail('\n  Either the field was removed and the row is stale, or the first')
  fail('  column picked up prose that is not a field name.')
}

if (!missing.length && !stale.length) {
  console.log(`OK  ${fields.length} schema fields, all documented`)
}

// Surface what was collapsed, so the single-provenance assumption stays visible
// rather than silently swallowing newly added fields.
if (opaque.length) {
  console.log('\nDocumented as one row each (sub-fields not checked individually):')
  for (const o of opaque) {
    console.log(`  ${o.def.padEnd(16)} ${String(o.count).padStart(2)} sub-fields — ${o.file}`)
  }
  console.log('  Verify these still share one provenance before trusting the OK above.')
}

// ---------------------------------------------------------------------------
// 2. Every source the build fetches must be credited in NOTICE.
// ---------------------------------------------------------------------------

// config.yaml is the authoritative list of what the build actually downloads.
// A URL added there without a matching NOTICE entry is an attribution gap.
const notice = read(NOTICE)
const urls = [...read(CONFIG).matchAll(/^\s*url:\s*(\S+)/gm)].map((m) => m[1])

// Reduce a URL to the token most likely to name it in NOTICE: for a GitHub or
// raw.githubusercontent URL that is the repository name, otherwise the host.
function creditToken(url) {
  const u = new URL(url)
  if (u.hostname === 'github.com' || u.hostname === 'raw.githubusercontent.com') {
    const [, , repo] = u.pathname.split('/')
    return repo
  }
  return u.hostname.replace(/^www\./, '')
}

const uncredited = [...new Set(urls.map(creditToken))].filter(
  (t) => !notice.toLowerCase().includes(t.toLowerCase()),
)
if (uncredited.length) {
  fail(`\nSOURCES FETCHED BUT NOT CREDITED IN NOTICE (${uncredited.length})`)
  for (const t of uncredited) fail(`  ${t}`)
  fail('\n  NOTICE travels with every redistribution. An uncredited source is')
  fail('  a licensing defect, not a documentation nit.')
} else {
  console.log(`OK  ${new Set(urls.map(creditToken)).size} fetched sources, all credited in NOTICE`)
}

// A source adapter with no config.yaml entry would be missed by the check
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

console.log(
  failed
    ? '\nFAILED — the provenance docs are out of date. See above.'
    : '\nMechanical checks passed. The source attribution itself still needs a read.',
)
process.exit(failed ? 1 : 0)
