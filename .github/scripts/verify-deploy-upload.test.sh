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
run "anchored patterns pass" ok '/docs/' '/proto/' '/.sources/' '/config/' '/coverage.out'

# The outage: unanchored docs/ also matched web/content/docs/ and removed every
# documentation page from the deployment.
run "unanchored docs/ is rejected" reject 'docs/' '/proto/'

# The earlier failure: excluding the app the web project builds.
run "excluding web/ is rejected" reject '/docs/' '/proto/' 'web/'

# The same class, one level in.
run "excluding web/content/ is rejected" reject '/docs/' '/proto/' '/web/content/'

# data/** is embedded into the Go function; losing the staff half would break
# the API without touching the site.
run "excluding data/staff/ is rejected" reject '/docs/' '/proto/' '/data/staff/'

# An emptied file keeps everything, which must NOT count as passing: the
# intended exclusions are part of the contract.
run "an empty file is rejected" reject ''

# The Go API deploys from this repository too, and its files are a separate
# failure: the site would keep working while the API function stopped building.
run "excluding internal/ is rejected" reject '/docs/' '/proto/' '/internal/'
run "excluding cmd/ is rejected" reject '/docs/' '/proto/' '/cmd/'
run "excluding go.mod is rejected" reject '/docs/' '/proto/' '/go.mod'

# The listing index is one file inside a directory the check already requires,
# so excluding just it would leave every other data/ file matching. It is named
# separately for that reason, and this is what proves the naming works: without
# the index the API does not boot at all.
run "excluding data/index.tsv is rejected" reject '/docs/' '/proto/' '/config/' '/data/index.tsv'

# config/ unanchored repeats the docs/ collision, since internal/config/ exists.
run "unanchored config/ is rejected" reject '/docs/' '/proto/' 'config/'

# Dropping a pattern entirely must not pass: the exclusions are part of the
# contract, not an optimisation.
run "dropping /config/ is rejected" reject '/docs/' '/proto/'

# A wildcard that sweeps up the whole repository.
run "excluding everything is rejected" reject '*'

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
