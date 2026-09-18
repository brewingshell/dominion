#!/usr/bin/env bash
# Build one icon variant of the Android + Linux desktop clients into ../client_app.
#
# Usage: ./build-variant.sh <output-base> <icon.png> <foreground.png>
#
#   output-base    e.g. "dominion_0.0.2" -> ../client_app/dominion_0.0.2.{apk,AppImage}
#   icon.png       1024x1024 square app icon
#   foreground.png 1024x1024 transparent foreground (Android adaptive icon safe
#                  zone); pass the same file as icon.png if unsure.
#
# Android needs an SDK and a JDK with jlink (17 works; some 21 builds omit it):
#   export ANDROID_HOME="$HOME/Android"
#   export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
# The desktop AppImage needs GTK3 + WebKitGTK dev packages:
#   sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
set -euo pipefail

base="${1:?usage: build-variant.sh <output-base> <icon.png> <foreground.png>}"
icon="${2:?missing icon.png}"
fg="${3:?missing foreground.png}"

# The icon paths are relative to wherever the script was invoked (release.sh
# passes repo-root paths, build-client.sh passes apps-relative ones), so pin
# them before cd'ing into apps/.
here="$(cd "$(dirname "$0")" && pwd)"
invoked_from="$(pwd)"
case "$icon" in /*) ;; *) icon="$invoked_from/$icon" ;; esac
case "$fg" in /*) ;; *) fg="$invoked_from/$fg" ;; esac
out="$here/../client_app"
android_res="$here/android/app/src/main/res"
cd "$here"

[ -f "$icon" ] || { echo "icon not found: $icon" >&2; exit 1; }
[ -f "$fg" ] || { echo "foreground not found: $fg" >&2; exit 1; }
[ -d node_modules ] || npm install
# The generated Android project is gitignored, so a fresh clone or CI runner
# does not have it. Create it on demand instead of failing.
[ -d android ] || npx cap add android
mkdir -p "$out"

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

echo "==> $base: Android project overlay"
# Keep the generated project in sync with the tracked overlay: the WebView
# client patch, the network security config, and the bundled CA.
mkdir -p android/app/src/main/res/xml android/app/src/main/res/raw \
         android/app/src/main/java/net/dominion/client
cp android-overlay/MainActivity.java android/app/src/main/java/net/dominion/client/MainActivity.java
cp android-overlay/network_security_config.xml android/app/src/main/res/xml/network_security_config.xml
if [ -n "${DOMINION_CA:-}" ]; then
  ./android-overlay/sync-ca.sh "$DOMINION_CA" >/dev/null
else
  ./android-overlay/sync-ca.sh >/dev/null
fi
cp android-overlay/res-raw/dominion_ca.pem android/app/src/main/res/raw/dominion_ca.pem

echo "==> $base: Android APK"
npx cap sync android >/dev/null

# The release version drives the APK's versionName/versionCode (the generated
# project defaults to 1.0/1). versionCode is major*10000 + minor*100 + patch.
if [ -n "${DOMINION_VERSION:-}" ]; then
  gradle=android/app/build.gradle
  IFS=. read -r v_maj v_min v_pat <<EOF
${DOMINION_VERSION}
EOF
  v_maj="${v_maj//[^0-9]/}"; v_min="${v_min//[^0-9]/}"; v_pat="${v_pat//[^0-9]/}"
  v_maj="${v_maj:-0}"; v_min="${v_min:-0}"; v_pat="${v_pat:-0}"
  v_code=$((10#$v_maj * 10000 + 10#$v_min * 100 + 10#$v_pat))
  if grep -q 'versionCode ' "$gradle" && grep -q 'versionName ' "$gradle"; then
    sed -i "s/versionCode [0-9][0-9]*/versionCode $v_code/" "$gradle"
    sed -i "s/versionName \"[^\"]*\"/versionName \"$DOMINION_VERSION\"/" "$gradle"
    echo "    versionName $DOMINION_VERSION (versionCode $v_code)"
  fi
fi

( cd android && ./gradlew assembleDebug -q )
rm -f "$out/$base.apk"
cp android/app/build/outputs/apk/debug/app-debug.apk "$out/$base.apk"

echo "==> $base: desktop AppImage"
if [ "${SKIP_DESKTOP:-0}" = "1" ]; then
  echo "    (skipped)"
else
  ./build-desktop.sh "$icon" "$base"
fi

echo
if [ "${SKIP_DESKTOP:-0}" = "1" ]; then
  ls -lh "$out/$base.apk"
else
  ls -lh "$out/$base.AppImage" "$out/$base.apk"
fi
