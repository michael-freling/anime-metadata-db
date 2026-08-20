#!/usr/bin/env node
// Tests for check-pagination.mjs.
//
// The check guards the API; this guards the check. Without it, an edit that
// made the checker pass everything would leave the proto unguarded with CI
// still green — which is the failure mode the check exists to prevent, one
// level up.
//
// Usage: node .claude/skills/enforce-pagination/scripts/check-pagination.test.mjs

import { readFileSync } from 'node:fs';
import { exit } from 'node:process';
import { check } from './check-pagination.mjs';

let failed = 0;

function run(name, source, expectation) {
  const problems = check(source);
  const ok = expectation(problems);
  console.log(`  ${ok ? 'ok  ' : 'FAIL'}  ${name}`);
  if (!ok) {
    failed++;
    console.log(`        got: ${JSON.stringify(problems, null, 2)}`);
  }
}

const accepts = (problems) => problems.length === 0;
const rejects = (needle) => (problems) => problems.some((p) => p.includes(needle));

// --- the two legal shapes are accepted ------------------------------------

run(
  'a paginated list response is accepted',
  `message ListThingsRequest {
     string page_token = 1;
     int32 limit = 2;
   }
   message ListThingsResponse {
     repeated Thing things = 1;
     string next_page_token = 2;
     int32 total_size = 3;
   }`,
  accepts,
);

run(
  'a capped embed with its true size is accepted',
  `message Thing {
     repeated Part parts = 1;
     int32 parts_total = 2;
   }`,
  accepts,
);

// A response can paginate without being named List* — Search does.
run(
  'pagination is recognised by fields, not by the message name',
  `message SearchRequest {
     string page_token = 1;
     int32 limit = 2;
   }
   message SearchResponse {
     repeated Result results = 1;
     string next_page_token = 2;
     int32 total_size = 3;
   }`,
  accepts,
);

// --- the violations are caught --------------------------------------------

run(
  'an unbounded repeated field is rejected',
  `message Thing {
     repeated Part parts = 1;
   }`,
  rejects('Thing.parts is repeated but unbounded'),
);

run(
  'a list response without a cursor is rejected',
  `message ListThingsResponse {
     repeated Thing things = 1;
     int32 total_size = 2;
   }`,
  rejects('no next_page_token'),
);

run(
  'a list response without a total is rejected',
  `message ListThingsResponse {
     repeated Thing things = 1;
     string next_page_token = 2;
   }`,
  rejects('no total_size'),
);

run(
  'a paginated response whose request cannot be driven is rejected',
  `message ListThingsRequest {
     string name = 1;
   }
   message ListThingsResponse {
     repeated Thing things = 1;
     string next_page_token = 2;
     int32 total_size = 3;
   }`,
  rejects('its request has no page_token'),
);

run(
  'a total counting nothing is rejected',
  `message Thing {
     int32 parts_total = 1;
   }`,
  rejects('which does not exist'),
);

// A field renamed out from under an exemption must not stay silently exempt:
// the name could later be reused for something that does grow.
run(
  'a stale entry in BOUNDED_BY_ENTITY is rejected',
  `message WatchOrder {
     string name = 1;
   }`,
  rejects('BOUNDED_BY_ENTITY lists WatchOrder.entries'),
);

// --- parsing details that would silently weaken the check -----------------

run(
  'a field named in a comment is not mistaken for a real one',
  `message Thing {
     // parts_total used to live here.
     repeated Part parts = 1;
   }`,
  rejects('Thing.parts is repeated but unbounded'),
);

run(
  'a nested block does not truncate the message it sits in',
  `message Thing {
     Other other = 1 [(x) = { a: 1 }];
     repeated Part parts = 2;
   }`,
  rejects('Thing.parts is repeated but unbounded'),
);

// --- the real proto ---------------------------------------------------------

run('the committed proto passes', readFileSync('api/proto/anime/v1/anime.proto', 'utf8'), accepts);

console.log(failed === 0 ? '\nall checks passed' : `\n${failed} failed`);
exit(failed === 0 ? 0 : 1);
