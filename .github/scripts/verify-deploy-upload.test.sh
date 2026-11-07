#!/usr/bin/env bash
# Regression test for verify-deploy-upload.sh.
#
# The guard protects the deployment upload, but nothing protected the guard: an
# edit to its matcher, its materialise-then-copy ordering, or its required list
# could quietly turn it into a check that passes everything, and .vercelignore
# would be unguarded again with CI still green. Each case below is a mistake
# that actually shipped, or one a plausible edit would introduce.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
GUARD="$HERE/verify-deploy-upload.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
fail=0

# run <name> <expected: ok|reject> <pattern-file-contents...>
run() {
  local name="$1" expect="$2"
  shift 2
  local file="$TMP/patterns"
  printf '%s\n' "$@" > "$file"

  local got
  if VERCELIGNORE="$file" bash "$GUARD" >/dev/null 2>&1; then got=ok; else got=reject; fi

  if [ "$got" = "$expect" ]; then
    echo "  ok   $name"
    pass=$((pass + 1))
  else
    echo "  FAIL $name — expected $expect, got $got"
    fail=$((fail + 1))
  fi
}

echo "verify-deploy-upload.sh:"

# The real file must pass, or the guard is broken rather than the config.
if bash "$GUARD" >/dev/null 2>&1; then
  echo "  ok   the repository's own .vercelignore passes"
  pass=$((pass + 1))
else
  echo "  FAIL the repository's own .vercelignore is rejected"
  fail=$((fail + 1))
fi

# Anchored patterns: what the file should look like.
run "anchored patterns pass" ok '/src/api/proto/' '/.sources/' '/src/builder/' '/coverage.out'

# The outage: unanchored docs/ also matched web/content/docs/ and removed every
# documentation page from the deployment. The Hugo tree it was aimed at is gone
# — the pattern is not in the file any more — but the trap it fell into is a
# property of gitignore, so it is still what this asserts against.
run "unanchored docs/ is rejected" reject 'docs/' '/src/api/proto/' '/src/builder/'

# The earlier failure: excluding the app the web project builds.
run "excluding web/ is rejected" reject '/src/api/proto/' '/src/builder/' 'web/'

# The same class, one level in.
run "excluding src/web/content/ is rejected" reject '/src/api/proto/' '/src/builder/' '/src/web/content/'

# dataset/data is read from disk by the Go function; losing the staff half
# would break the API without touching the site.
run "excluding dataset/data/staff/ is rejected" reject '/src/api/proto/' '/src/builder/' '/dataset/data/staff/'

# An emptied file keeps everything, which must NOT count as passing: the
# intended exclusions are part of the contract.
run "an empty file is rejected" reject ''

# The Go API deploys from this repository too, and its files are a separate
# failure: the site would keep working while the API function stopped building.
run "excluding src/api/ is rejected" reject '/src/api/proto/' '/src/builder/' '/src/api/'

# The API module resolves the shared module through `replace ../animedb`, so
# src/animedb must reach the upload even though Vercel builds from src/api.
# Dropping it fails the build with an unresolvable module rather than anything
# that names the real cause. Its go.mod is named separately from the directory
# because that is the file the replace resolves against.
run "excluding src/animedb/ is rejected" reject '/src/api/proto/' '/src/builder/' '/src/animedb/'
run "excluding src/animedb/go.mod is rejected" reject '/src/api/proto/' '/src/builder/' '/src/animedb/go.mod'
run "excluding src/api/go.mod is rejected" reject '/src/api/proto/' '/src/builder/' '/src/api/go.mod'

# The listing index is one file inside a directory the check already requires,
# so excluding just it would leave every other dataset/data file matching. It
# is named separately for that reason, and this is what proves the naming
# works: without the index the API does not boot at all.
run "excluding dataset/data/index.tsv is rejected" reject '/src/api/proto/' '/src/builder/' '/dataset/data/index.tsv'

# config/ unanchored repeats the docs/ collision, since src/api/internal/config/
# exists. The unanchored-pattern trap has a new shape rather than being gone:
# there are now three internal/ directories, and an unanchored pattern meant for
# the builder's would take the API's with it.
run "unanchored internal/ is rejected" reject '/src/api/proto/' '/src/builder/' 'internal/'

# Dropping a pattern entirely must not pass: the exclusions are part of the
# contract, not an optimisation.
run "dropping /src/builder/ is rejected" reject '/src/api/proto/'

# A wildcard that sweeps up the whole repository.
run "excluding everything is rejected" reject '*'

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
