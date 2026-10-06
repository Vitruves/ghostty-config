#!/bin/sh
# Capture the full-screen frames the tests draw and render them to PNG.
# Usage: tools/shot.sh <outdir> [go test -run pattern]
set -e
out="${1:?outdir}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
GHOSTTY_CONFIG_DUMP="$out" go test ./internal/tui -run "${2:-TestPaletteFrames}" >/dev/null
# PYTHON names an interpreter that has Pillow, when the default one has not.
for f in "$out"/*.ansi; do
  "${PYTHON:-python3}" "$(dirname "$0")/ansi2png.py" "$f" "${f%.ansi}.png" >/dev/null
done
ls "$out"/*.png
