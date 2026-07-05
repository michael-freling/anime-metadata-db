#!/usr/bin/env bash
# Ensure a headless Chromium (via Playwright) is available for screenshots,
# staging any missing system libraries into a local prefix — no root required.
# Idempotent; prints the path to an env file the caller should `source`.
#
# Tuned for Ubuntu 24.04 (noble): the soname->package map below uses noble names
# (some are the t64 variants). On other distros, unmapped libs are reported.
set -euo pipefail

WORK="${1:?usage: setup-browser.sh <work-dir>}"
mkdir -p "$WORK"
ENV_FILE="$WORK/browser-env.sh"
export PLAYWRIGHT_BROWSERS_PATH="$WORK/ms-playwright"
LIBPREFIX="$WORK/libs"
LIBDIR="$LIBPREFIX/usr/lib/x86_64-linux-gnu"

shell_bin() { ls "$PLAYWRIGHT_BROWSERS_PATH"/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell 2>/dev/null | head -1; }
missing_for() { LD_LIBRARY_PATH="$LIBDIR" ldd "$1" 2>/dev/null | awk '/not found/{print $1}'; }
all_missing() { local b; for b in "$@"; do [ -n "$b" ] && missing_for "$b"; done | sort -u; }

# Fast path: already set up and all libs resolve.
if [ -f "$ENV_FILE" ]; then
  SB="$(shell_bin || true)"
  if [ -n "$SB" ] && [ -z "$(all_missing "$SB")" ]; then echo "$ENV_FILE"; exit 0; fi
fi

# 1) Install Playwright + Chromium if missing.
# Anchor npm to this dir with an explicit package.json — otherwise npm walks up
# the tree and may resolve to a parent project's node_modules and install nothing
# here. --no-workspaces guards against an ancestor package.json declaring one.
if [ ! -d "$WORK/node_modules/playwright" ]; then
  [ -f "$WORK/package.json" ] || printf '{ "name": "docs-screenshots", "version": "1.0.0", "private": true }\n' > "$WORK/package.json"
  ( cd "$WORK" && npm install playwright@latest --no-workspaces --no-fund --no-audit >/dev/null 2>&1 )
fi
( cd "$WORK" && npx --yes playwright install chromium >/dev/null 2>&1 ) || true

SB="$(shell_bin || true)"
CHROME_BIN="$(ls "$PLAYWRIGHT_BROWSERS_PATH"/chromium-*/chrome-linux/chrome 2>/dev/null | head -1 || true)"
[ -n "$SB" ] || { echo "error: chromium did not install under $PLAYWRIGHT_BROWSERS_PATH" >&2; exit 1; }

# soname -> apt package (Ubuntu noble) for the Chromium runtime deps.
declare -A SONAME_PKG=(
  [libnspr4.so]=libnspr4 [libnss3.so]=libnss3 [libnssutil3.so]=libnss3 [libsmime3.so]=libnss3
  [libgbm.so.1]=libgbm1 [libasound.so.2]=libasound2t64
  [libatk-1.0.so.0]=libatk1.0-0t64 [libatk-bridge-2.0.so.0]=libatk-bridge2.0-0t64 [libatspi.so.0]=libatspi2.0-0t64
  [libXcomposite.so.1]=libxcomposite1 [libXdamage.so.1]=libxdamage1 [libXfixes.so.3]=libxfixes3
  [libXrandr.so.2]=libxrandr2 [libXrender.so.1]=libxrender1 [libXi.so.6]=libxi6
  [libXext.so.6]=libxext6 [libXtst.so.6]=libxtst6 [libXcursor.so.1]=libxcursor1
  [libxkbcommon.so.0]=libxkbcommon0 [libwayland-server.so.0]=libwayland-server0
  [libwayland-client.so.0]=libwayland-client0 [libwayland-egl.so.1]=libwayland-egl1
  [libdrm.so.2]=libdrm2 [libcups.so.2]=libcups2t64 [libdbus-1.so.3]=libdbus-1-3
  [libpango-1.0.so.0]=libpango-1.0-0 [libpangocairo-1.0.so.0]=libpango-1.0-0
  [libcairo.so.2]=libcairo2 [libcairo-gobject.so.2]=libcairo-gobject2
  [libgtk-3.so.0]=libgtk-3-0t64 [libgdk-3.so.0]=libgtk-3-0t64
  [libgdk_pixbuf-2.0.so.0]=libgdk-pixbuf-2.0-0 [libglib-2.0.so.0]=libglib2.0-0t64
)

# 2) Stage missing libraries, iterating until resolved (deps can surface deps).
AP="$WORK/aptroot"; apt_ready=0
for _round in 1 2 3 4 5; do
  # ldd exits non-zero when libs are missing; tolerate it (that's the signal).
  miss="$(all_missing "$SB" "$CHROME_BIN" || true)"
  [ -z "$miss" ] && break
  pkgs=""
  for so in $miss; do
    p="${SONAME_PKG[$so]:-}"
    if [ -n "$p" ]; then pkgs="$pkgs $p"; else echo "warn: no package mapping for $so" >&2; fi
  done
  pkgs="$(printf '%s\n' $pkgs | sort -u | tr '\n' ' ')"
  [ -z "${pkgs// /}" ] && break
  if [ "$apt_ready" = 0 ]; then
    mkdir -p "$AP/lists/partial" "$AP/cache/archives/partial" "$AP/debs"; : > "$AP/status"
    apt-get update -o Dir::State::Lists="$AP/lists" -o Dir::Cache="$AP/cache" \
      -o Dir::State::status="$AP/status" -o Debug::NoLocking=true >/dev/null 2>&1 || true
    apt_ready=1
  fi
  ( cd "$AP/debs" && apt-get download $pkgs \
      -o Dir::State::Lists="$AP/lists" -o Dir::Cache="$AP/cache" \
      -o Dir::State::status="$AP/status" -o Debug::NoLocking=true >/dev/null 2>&1 || true )
  for d in "$AP"/debs/*.deb; do [ -f "$d" ] && dpkg-deb -x "$d" "$LIBPREFIX"; done
done

leftover="$(all_missing "$SB" "$CHROME_BIN" || true)"
[ -n "$leftover" ] && echo "warn: unresolved libraries (screenshots may fail): $leftover" >&2 || true

# 3) Emit the env file for the caller.
cat > "$ENV_FILE" <<EOF
export PLAYWRIGHT_BROWSERS_PATH="$PLAYWRIGHT_BROWSERS_PATH"
export LD_LIBRARY_PATH="$LIBDIR:\${LD_LIBRARY_PATH:-}"
export SHOTS_NODE_DIR="$WORK"
EOF
echo "$ENV_FILE"
