#!/usr/bin/env bash
# Build the desktop (AppImage) and Android (debug APK) clients into ../client_app
# with the default (original-mark) icon.
#
# Usage: ./build-client.sh [desktop|android|all]
#
# Android needs an SDK and a JDK with jlink (17 works; some 21 builds omit it):
#   export ANDROID_HOME="$HOME/Android"
#   export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
# The desktop AppImage needs GTK3 + WebKitGTK dev packages:
#   sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
set -euo pipefail

target="${1:-all}"
version="${DOMINION_VERSION:-0.0.2}"
here="$(cd "$(dirname "$0")" && pwd)"
out="$here/../client_app"
cd "$here"

mkdir -p "$out"

case "$target" in
  desktop)
    ./build-desktop.sh icons/original.png "dominion_$version"
    ;;
  android)
    SKIP_DESKTOP=1 DOMINION_VERSION="$version" ./build-variant.sh "dominion_$version" icons/original.png icons/original-foreground.png
    ;;
  all)
    DOMINION_VERSION="$version" ./build-variant.sh "dominion_$version" icons/original.png icons/original-foreground.png
    ;;
  *)
    echo "usage: $0 [desktop|android|all]" >&2
    exit 2
    ;;
esac

echo
ls -lh "$out"/*.AppImage "$out"/*.apk 2>/dev/null || true
