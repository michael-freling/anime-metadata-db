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
cp "$REPO/.vercelignore" "$WORK/.gitignore"

excluded() { git -C "$WORK" check-ignore -q "$1"; }

fail=0

# Every tracked file under these paths must reach the deployment.
#   web/**            the app the web project builds
#   data/**           embedded into the Go function with go:embed
#   web/content/**    the MDX the docs site renders
for required in 'web/package.json' 'web/content/docs' 'web/src' 'data/series'; do
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
for intended in 'proto' 'docs'; do
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
