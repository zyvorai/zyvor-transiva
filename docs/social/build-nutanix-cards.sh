#!/usr/bin/env bash
# Render the Nutanix-exit cards and deck PDF with Google Chrome (+ macOS sips for the jpg).
#   ./docs/social/build-nutanix-cards.sh
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
[[ -x "$CHROME" ]] || { echo "Google Chrome not found (set CHROME=...)" >&2; exit 1; }
shot() { "$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --window-size="$2,$3" --screenshot="$4" "file://$HERE/nutanix-exit-card.html#$1" >/dev/null 2>&1; echo "wrote $4"; }
shot light 1200 630 "$HERE/nutanix-exit-card.png"
shot dark  1200 630 "$HERE/nutanix-exit-card-dark.png"
raw="$(mktemp "${TMPDIR:-/tmp}/nx.XXXXXX.png")"; shot li 1600 900 "$raw"
sips -s format jpeg -s formatOptions 92 "$raw" --out "$HERE/nutanix-exit-linkedin.jpg" >/dev/null; rm -f "$raw"
"$CHROME" --headless=new --disable-gpu --no-pdf-header-footer \
  --print-to-pdf="$HERE/../marketing/nutanix-exit-deck.pdf" "file://$HERE/../marketing/nutanix-exit-deck.html" >/dev/null 2>&1
echo "wrote deck pdf"
