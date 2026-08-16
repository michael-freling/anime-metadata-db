#!/usr/bin/env bash
# Build the docs site, serve it locally, and screenshot the key pages so the
# rendered layout can be reviewed. Prints the screenshot paths on completion.
#
#   bash .claude/skills/verify-docs-layout/scripts/capture.sh [extra /paths ...]
#
# Env: INTERNAL=1 capture the dev-only Development section; PORT=8199 serve port.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(git -C "$HERE" rev-parse --show-toplevel)"
APP="$REPO/web"
WORK="$APP/.screenshots"
OUT="$WORK/out"
PORT="${PORT:-8199}"
INTERNAL="${INTERNAL:-0}"
mkdir -p "$OUT"; rm -f "$OUT"/*.png 2>/dev/null || true

[ -d "$APP/node_modules" ] || { echo ">> installing web dependencies…"; ( cd "$APP" && npm install >/dev/null ); }

echo ">> ensuring headless browser (first run downloads Chromium)…"
ENVF="$(bash "$HERE/setup-browser.sh" "$WORK")"
# shellcheck disable=SC1090
source "$ENVF"

# The Development section is excluded from every non-dev build (see
# web/src/lib/source.ts), so capturing it means running the dev server rather
# than the production one. Everything else is shot against a real production
# build, which is what users actually get.
if [ "$INTERNAL" = 1 ]; then
  echo ">> starting dev server on :$PORT (includes internal docs)…"
  ( cd "$APP" && exec npx next dev -p "$PORT" ) >/dev/null 2>&1 &
else
  echo ">> building web (production)…"
  ( cd "$APP" && npm run build >/dev/null )
  echo ">> serving production build on :$PORT…"
  ( cd "$APP" && exec npx next start -p "$PORT" ) >/dev/null 2>&1 &
fi
SRV=$!
cleanup() { kill "$SRV" 2>/dev/null || true; }
trap cleanup EXIT
for _ in $(seq 1 90); do curl -sf -o /dev/null "http://localhost:$PORT/" && break; sleep 1; done

# Pages to shoot: caller-supplied paths, else a sensible default set. Fumadocs
# URLs have no trailing slash — a trailing slash redirects and the shot lands on
# the wrong page.
if [ "$#" -gt 0 ]; then
  PATHS=("$@")
else
  PATHS=(/ /docs /docs/using-the-api /docs/using-the-dataset /docs/building-the-dataset)
  [ "$INTERNAL" = 1 ] && PATHS+=(/docs/development)
fi

echo ">> capturing ${#PATHS[@]} page(s)…"
cp "$HERE/shoot.mjs" "$WORK/shoot.mjs"
( cd "$WORK" && node shoot.mjs "http://localhost:$PORT" "$OUT" "${PATHS[@]}" )

echo ""
echo "Screenshots written to: $OUT"
ls -1 "$OUT"/*.png
echo ""
echo "Next: Read each PNG above and check the layout (see SKILL.md checklist)."
