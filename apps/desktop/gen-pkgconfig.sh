#!/bin/sh
# Generate the pkg-config shim webview_go needs.
#
# webview_go's cgo directive asks pkg-config for `webkit2gtk-4.0`. Debian 13 and
# Ubuntu 24.04 ship only WebKitGTK 4.1, whose library the bundled header
# dlopen()s at runtime; so a .pc named 4.0 that forwards to the installed 4.1
# package is all that is required.
#
# Writes apps/desktop/.pkgconfig/webkit2gtk-4.0.pc (gitignored), then build with:
#   PKG_CONFIG_PATH=apps/desktop/.pkgconfig go build ./...
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
out="$here/.pkgconfig"
mkdir -p "$out"

if ! pkg-config --exists webkit2gtk-4.1; then
  echo "webkit2gtk-4.1 not found; install libwebkit2gtk-4.1-dev" >&2
  exit 1
fi

dir="$(pkg-config --variable=pcfiledir webkit2gtk-4.1)"
src="$dir/webkit2gtk-4.1.pc"
[ -f "$src" ] || { echo "cannot find $src" >&2; exit 1; }

sed 's/^Name:.*/Name: webkit2gtk-4.0/' "$src" > "$out/webkit2gtk-4.0.pc"
echo "wrote $out/webkit2gtk-4.0.pc"
