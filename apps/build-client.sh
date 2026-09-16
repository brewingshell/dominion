#!/usr/bin/env bash
# Build the desktop (AppImage) and Android (debug APK) clients into ../client_app.
#
# Usage: ./build-client.sh [desktop|android|all]
#
# Android needs an SDK and a JDK with jlink (17 works; some 21 builds omit it):
#   export ANDROID_HOME="$HOME/Android"
#   export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
set -euo pipefail

target="${1:-all}"
here="$(cd "$(dirname "$0")" && pwd)"
out="$here/../client_app"
cd "$here"

mkdir -p "$out"
[ -d node_modules ] || npm install

build_desktop() {
  echo "==> Building AppImage"
  npm run electron:dist
  echo "    -> $out/dominion-*.AppImage"
}

build_android() {
  echo "==> Building Android debug APK"
  npx cap sync android
  ( cd android && ./gradlew assembleDebug )
  local apk="android/app/build/outputs/apk/debug/app-debug.apk"
  local dest="$out/dominion-debug.apk"
  cp "$apk" "$dest"
  echo "    -> $dest"
}

case "$target" in
  desktop) build_desktop ;;
  android) build_android ;;
  all) build_desktop; build_android ;;
  *) echo "usage: $0 [desktop|android|all]" >&2; exit 2 ;;
esac

echo
ls -lh "$out"/*.AppImage "$out"/*.apk 2>/dev/null || true
