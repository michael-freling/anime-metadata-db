#!/usr/bin/env bash
# Verify that .vercelignore does not exclude files the deployments need.
#
# This exists because CI cannot otherwise see this class of failure. The web
# app's build job runs `next build` over a full checkout and passes, while the
# real deployment builds over an upload that .vercelignore has already filtered
# — so a pattern that deletes the wrong thing produces a green CI run and a
# broken site. It has happened twice:
#
#   web/    excluded the app itself      -> "No Next.js version detected"
#   docs/   unanchored, so it also matched web/content/docs/
#           -> every /docs page 404'd in production while the rest worked
#
# The check replays the patterns with git's own matcher rather than
# reimplementing gitignore semantics, since misreading those semantics is
# exactly what caused the second failure.
set -euo pipefail

REPO="$(git rev-parse --show-toplevel)"
# The pattern file to check. Overridable so the guard's own test can feed it
# deliberately broken variants; defaults to the real one.
PATTERNS="${VERCELIGNORE:-$REPO/.vercelignore}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git -C "$WORK" init -q .

# Materialise the tracked file list so git can match paths against it.
while IFS= read -r -d '' f; do
  mkdir -p "$WORK/$(dirname "$f")"
  : > "$WORK/$f"
done < <(git -C "$REPO" ls-files -z)

# Copy the patterns in AFTER materialising: the repository tracks its own
# .gitignore, so doing this first meant the loop truncated it to nothing and the
# check silently passed everything.
cp "$PATTERNS" "$WORK/.gitignore"

excluded() { git -C "$WORK" check-ignore -q "$1"; }

fail=0

# Every tracked file under these paths must reach the deployment. Both Vercel
# projects build from this repository, so both are covered:
#
#   web/**       the app the web project builds, including the MDX it renders
#   web/openapi  the OpenAPI spec the API reference pages are rendered from.
#                It is imported by src/lib/openapi.ts, so the web build fails
#                without it — and note api/proto/ IS excluded below: the spec is
#                generated from the proto, which is why the generated artefact
#                has to ship even though its source does not.
#   api/, internal/, go.mod, go.sum, dataset.go
#                what Vercel's Go builder compiles. The API builds from api/
#                with Root Directory set there, but it depends on the root
#                module through a replace directive, so the repository root's
#                go.mod and internal/ have to reach the upload too — building
#                from api/ alone would fail to resolve the dataset module.
#                The whole of internal/ rather than one package: an unanchored
#                `config/` pattern would sweep up builder/internal/config/ the
#                same way `docs/` swept up web/content/docs/.
#   data/**      embedded into that function with go:embed
#   data/index.tsv
#                named on its own, not just covered by data/: it is the listing
#                index every browse and search request is answered from, and
#                without it the server does not start at all. A pattern that
#                excluded only this one file would leave the rest of data/
#                matching and the check passing.
for required in \
  'web/package.json' 'web/content/docs' 'web/src' 'web/openapi' \
  'api/cmd' 'api/internal' 'api/go.mod' 'api/go.sum' \
  'internal' 'go.mod' 'go.sum' 'dataset.go' \
  'data' 'data/index.tsv'; do
  matched=0
  while IFS= read -r -d '' f; do
    matched=$((matched + 1))
    if excluded "$f"; then
      echo "::error file=.vercelignore::$f is excluded from the deployment upload but is required"
      fail=1
    fi
  done < <(git -C "$REPO" ls-files -z -- "$required")

  if [ "$matched" -eq 0 ]; then
    echo "::error::no tracked files matched '$required' — this check is asserting nothing, update it"
    fail=1
  fi
done

# And the exclusions that are meant to happen still do, so the file cannot be
# "fixed" by emptying it. Paths that no longer exist are skipped rather than
# failing, since the Hugo tree is on its way out.
# Checked against every directory pattern the file carries, not a sample: an
# unanchored `config/` would collide with internal/config/ exactly as the
# unanchored `docs/` collided with web/content/docs/.
for intended in 'api/proto' 'docs' 'builder'; do
  first="$(git -C "$REPO" ls-files -- "$intended" | head -1 || true)"
  [ -n "$first" ] || continue
  if ! excluded "$first"; then
    echo "::error file=.vercelignore::$first should be excluded from the deployment upload but is not"
    fail=1
  fi
done

if [ "$fail" -eq 0 ]; then
  echo "ok: .vercelignore keeps everything the deployments need"
fi
exit "$fail"
