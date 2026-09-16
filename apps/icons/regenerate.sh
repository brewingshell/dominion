#!/usr/bin/env bash
# Regenerate the square app icons from the source marks.
#
#   icons/original.png             the project mark (from assets/logo.png)
#   icons/original-foreground.png  adaptive foreground (safe zone)
#   icons/terran.png               alternate mark (from assets/override_logo.png)
#   icons/terran-foreground.png    adaptive foreground (safe zone)
#
# The override logo is an opaque JPEG-derived PNG with a black background, so
# the black is keyed out with a fuzz to give a clean transparent mark. The
# original mark is already transparent.
#
# Run from apps/:  ./icons/regenerate.sh
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../.." && pwd)"
cd "$here"

# ---- original mark (assets/logo.png, already has alpha) ----
convert -size 1024x1024 xc:none \( "$root/assets/logo.png" -resize 880x880 \) \
  -gravity center -composite PNG32:original.png
convert -size 1024x1024 xc:none \( "$root/assets/logo.png" -resize 660x660 \) \
  -gravity center -composite PNG32:original-foreground.png

# ---- alternate mark (assets/override_logo.png, opaque black background) ----
convert "$root/assets/override_logo.png" -fuzz 8% -transparent black /tmp/dominion-mark.png
convert -size 1024x1024 xc:none \( /tmp/dominion-mark.png -resize 880x880 \) \
  -gravity center -composite PNG32:terran.png
convert -size 1024x1024 xc:none \( /tmp/dominion-mark.png -resize 660x660 \) \
  -gravity center -composite PNG32:terran-foreground.png
rm -f /tmp/dominion-mark.png

echo "regenerated:"
for f in original.png original-foreground.png terran.png terran-foreground.png; do
  printf "  %-28s " "$f"
  identify -format "%wx%h %[channels]\n" "$f"
done
