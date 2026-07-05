#!/usr/bin/env bash
# Build the docs site, serve it locally, and screenshot the key pages so the
# rendered layout can be reviewed. Prints the screenshot paths on completion.
#
#   bash .claude/skills/verify-docs-layout/scripts/capture.sh [extra /paths ...]
#
# Env: DRAFTS=1 include the draft-only Development section; PORT=8199 serve port.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(git -C "$HERE" rev-parse --show-toplevel)"
WORK="$REPO/docs/.screenshots"
OUT="$WORK/out"
PORT="${PORT:-8199}"
DRAFTS="${DRAFTS:-0}"
mkdir -p "$OUT"; rm -f "$OUT"/*.png 2>/dev/null || true

HUGO="$(ls "$REPO"/docs/.hugo/hugo-* 2>/dev/null | head -1 || true)"
[ -n "$HUGO" ] || { echo "error: pinned Hugo not found — run 'make hugo' first" >&2; exit 1; }

echo ">> ensuring headless browser (first run downloads Chromium)…"
ENVF="$(bash "$HERE/setup-browser.sh" "$WORK")"
# shellcheck disable=SC1090
source "$ENVF"

echo ">> building docs (production, baseURL=/)…"
rm -rf "$REPO/docs/public"
FLAGS="--minify"; [ "$DRAFTS" = 1 ] && FLAGS="$FLAGS --buildDrafts"
( cd "$REPO/docs" && HUGO_BASEURL="/" "$HUGO" $FLAGS >/dev/null )

echo ">> serving docs/public on :$PORT…"
python3 -m http.server "$PORT" --directory "$REPO/docs/public" >/dev/null 2>&1 &
SRV=$!
cleanup() { kill "$SRV" 2>/dev/null || true; }
trap cleanup EXIT
for _ in $(seq 1 40); do curl -sf -o /dev/null "http://localhost:$PORT/" && break; sleep 0.5; done

# Pages to shoot: caller-supplied paths, else a sensible default set.
if [ "$#" -gt 0 ]; then
  PATHS=("$@")
else
  PATHS=(/ /docs/ /docs/using-the-api/ /docs/using-the-dataset/ /docs/building-the-dataset/)
  [ "$DRAFTS" = 1 ] && PATHS+=(/development/)
fi

echo ">> capturing ${#PATHS[@]} page(s)…"
cp "$HERE/shoot.mjs" "$WORK/shoot.mjs"
( cd "$WORK" && node shoot.mjs "http://localhost:$PORT" "$OUT" "${PATHS[@]}" )

echo ""
echo "Screenshots written to: $OUT"
ls -1 "$OUT"/*.png
echo ""
echo "Next: Read each PNG above and check the layout (see SKILL.md checklist)."
