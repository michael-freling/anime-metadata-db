#!/usr/bin/env node
// Checks that no API response can return an unbounded amount of data.
//
// Structural only, by design: it can prove that a repeated field has a cursor
// or a total, and it cannot know whether a collection grows with the catalogue.
// That judgement lives in SKILL.md, and in BOUNDED_BY_ENTITY below, where each
// exemption carries its reason. The runtime half is
// TestNoResponseEmbedsAnUnboundedCollection in internal/api.
//
// Usage: node .claude/skills/enforce-pagination/scripts/check-pagination.mjs [proto-path]

import { readFileSync } from 'node:fs';
import { argv, exit } from 'node:process';

const PROTO = argv[2] ?? 'proto/anime/v1/anime.proto';

// Collections bounded by the entity that owns them, not by how large the
// catalogue grows. Each entry states why, because "it is small today" is not a
// reason and the next person needs to be able to check the claim.
export const BOUNDED_BY_ENTITY = new Map([
  ['Character.voice_actors', 'one per dub language, not per work in the catalogue'],
  ['CharacterAppearance.voice_actors', 'the cast override for one appearance; same bound'],
  ['CharacterAppearance.scope', 'the installments of a single series that an appearance narrows to'],
  ['StaffCredit.series_ids', 'the series one casting covers — a handful'],
  ['StaffCredit.series_titles', 'parallel to series_ids, resolved for display; same bound'],
  ['WatchOrder.entries', 'a hand-curated order over one franchise, authored by a person'],
]);

// Parses `message Name { ... }` blocks, tolerating nested braces.
export function parseMessages(source) {
  const messages = new Map();
  // Leading whitespace allowed: the committed proto starts each message at
  // column 0, but requiring that would silently parse only the first message of
  // any indented input and report no problems for the rest.
  const re = /^[ \t]*message\s+(\w+)\s*\{/gm;
  let m;
  while ((m = re.exec(source)) !== null) {
    let depth = 1;
    let i = re.lastIndex;
    for (; i < source.length && depth > 0; i++) {
      if (source[i] === '{') depth++;
      else if (source[i] === '}') depth--;
    }
    messages.set(m[1], source.slice(re.lastIndex, i - 1));
  }
  return messages;
}

// Returns the fields of a message body: name, whether repeated, and its type.
export function parseFields(body) {
  const fields = [];
  // Strip comments so a field name mentioned in prose is not mistaken for one.
  const code = body.replace(/\/\/[^\n]*/g, '');
  const re = /^\s*(repeated\s+|optional\s+)?([\w.]+)\s+(\w+)\s*=\s*\d+\s*;/gm;
  let m;
  while ((m = re.exec(code)) !== null) {
    fields.push({ repeated: m[1]?.trim() === 'repeated', type: m[2], name: m[3] });
  }
  return fields;
}

// A response paginates if it says where to continue and how much there is.
function isPaginated(fields) {
  const has = (n) => fields.some((f) => f.name === n);
  return has('next_page_token') && has('total_size');
}

export function check(source) {
  const problems = [];
  const messages = parseMessages(source);

  for (const [name, body] of messages) {
    const fields = parseFields(body);
    const has = (f) => fields.some((x) => x.name === f);

    // A response named List* has to paginate — that is what the name promises.
    if (/^List\w+Response$/.test(name)) {
      for (const required of ['next_page_token', 'total_size']) {
        if (!has(required)) problems.push(`${name} is a list response but has no ${required}`);
      }
    }

    // Whether a response paginates is decided by its fields, not its name:
    // Search predates the List* convention and paginates perfectly well. A
    // paginated response's matching request must let the caller drive it.
    if (isPaginated(fields) && name.endsWith('Response')) {
      const request = messages.get(`${name.slice(0, -'Response'.length)}Request`);
      if (request) {
        const requestFields = parseFields(request);
        for (const required of ['page_token', 'limit']) {
          if (!requestFields.some((f) => f.name === required)) {
            problems.push(
              `${name} is paginated but its request has no ${required}, ` +
                `so a caller cannot ask for the next page or bound the first`,
            );
          }
        }
      }
    }

    for (const field of fields) {
      const path = `${name}.${field.name}`;

      if (field.repeated) {
        if (BOUNDED_BY_ENTITY.has(path)) continue;
        // The payload of a paginated response is covered by that response's
        // own next_page_token/total_size, checked above.
        if (isPaginated(fields)) continue;
        if (!has(`${field.name}_total`)) {
          problems.push(
            `${path} is repeated but unbounded: give it a ${field.name}_total and cap it, ` +
              `page it from a List RPC, or add it to BOUNDED_BY_ENTITY with a reason`,
          );
        }
        continue;
      }

      // A total with nothing to count is a leftover from a removed field, and
      // would quietly report 0 forever.
      if (field.name.endsWith('_total') && field.type === 'int32') {
        const collection = field.name.slice(0, -'_total'.length);
        if (!fields.some((x) => x.repeated && x.name === collection)) {
          problems.push(`${path} counts a repeated field named ${collection}, which does not exist`);
        }
      }
    }
  }

  // An exemption for a field that no longer exists is stale judgement; it will
  // silently exempt the wrong thing if the name is ever reused.
  //
  // Only checked for messages the source actually defines: this function is
  // also called on fragments in its own tests, where every other message is
  // legitimately absent.
  for (const path of BOUNDED_BY_ENTITY.keys()) {
    const [message, field] = path.split('.');
    const body = messages.get(message);
    if (body && !parseFields(body).some((f) => f.repeated && f.name === field)) {
      problems.push(`BOUNDED_BY_ENTITY lists ${path}, which is not a repeated field in the proto`);
    }
  }

  return problems;
}

// Run only when invoked directly, so the tests can import the functions.
if (import.meta.url === `file://${process.argv[1]}`) {
  const problems = check(readFileSync(PROTO, 'utf8'));
  if (problems.length > 0) {
    console.error(`${PROTO}: every response must be bounded\n`);
    for (const p of problems) console.error(`  - ${p}`);
    console.error(`\nSee .claude/skills/enforce-pagination/SKILL.md`);
    exit(1);
  }
  console.log(`${PROTO}: every collection is paginated or capped with its true size`);
}
