#!/usr/bin/env bash
# Build one icon variant of the desktop + Android clients into ../client_app.
#
# Usage: ./build-variant.sh <output-base> <icon.png> <foreground.png>
#
#   output-base   e.g. "dominion_0.1" -> ../client_app/dominion_0.1.AppImage + .apk
#   icon.png      1024x1024 square app icon
#   foreground.png 1024x1024 transparent foreground (Android adaptive icon safe
#                  zone); pass the same file as icon.png if unsure.
#
# Android needs an SDK and a JDK with jlink (17 works; some 21 builds omit it):
#   export ANDROID_HOME="$HOME/Android"
#   export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
set -euo pipefail

base="${1:?usage: build-variant.sh <output-base> <icon.png> <foreground.png>}"
icon="${2:?missing icon.png}"
fg="${3:?missing foreground.png}"

here="$(cd "$(dirname "$0")" && pwd)"
out="$here/../client_app"
android_res="$here/android/app/src/main/res"
cd "$here"

[ -f "$icon" ] || { echo "icon not found: $icon" >&2; exit 1; }
[ -f "$fg" ] || { echo "foreground not found: $fg" >&2; exit 1; }
[ -d node_modules ] || npm install
[ -d android ] || { echo "apps/android missing; run: npx cap add android" >&2; exit 1; }
mkdir -p "$out"

echo "==> $base: AppImage"
# electron-builder writes <productName>-<version>.AppImage; clear any previous
# one first so a stale file is never mistaken for this build's output.
rm -f "$out"/dominion-*.AppImage
rm -f "$out/$base.AppImage"
npx electron-builder --linux AppImage --config.linux.icon="$icon" >/dev/null
if [ -f "$out"/dominion-*.AppImage ]; then
  mv "$out"/dominion-*.AppImage "$out/$base.AppImage"
fi
chmod +x "$out/$base.AppImage"

echo "==> $base: Android launcher icons"
# Transparent by default: the mark is drawn with alpha and no plate behind it.
# Set BG_COLOR=#rrggbb to put a solid adaptive-icon background back.
bg="${BG_COLOR:-#00000000}"
cat > "$android_res/values/ic_launcher_background.xml" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <color name="ic_launcher_background">$bg</color>
</resources>
EOF
# PNG32 forces an alpha channel so transparency survives the resize.
for d in mdpi:48 hdpi:72 xhdpi:96 xxhdpi:144 xxxhdpi:192; do
  dens="${d%%:*}"; size="${d##*:}"
  fg_size=$(( size * 108 / 48 ))
  convert "$icon" -resize "${size}x${size}" PNG32:"$android_res/mipmap-$dens/ic_launcher.png"
  convert "$icon" -resize "${size}x${size}" PNG32:"$android_res/mipmap-$dens/ic_launcher_round.png"
  convert "$fg" -resize "${fg_size}x${fg_size}" PNG32:"$android_res/mipmap-$dens/ic_launcher_foreground.png"
done

echo "==> $base: Android APK"
npx cap sync android >/dev/null
( cd android && ./gradlew assembleDebug -q )
rm -f "$out/$base.apk"
cp android/app/build/outputs/apk/debug/app-debug.apk "$out/$base.apk"

echo
ls -lh "$out/$base.AppImage" "$out/$base.apk"
