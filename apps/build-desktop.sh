#!/usr/bin/env bash
# Build the thin Linux desktop client (Go + system WebView) into ../client_app.
#
#   ./build-desktop.sh <icon.png> [output-base]
#
# The binary links against the system GTK3 + WebKitGTK, so it is a few megabytes
# rather than Electron's ~170 MB. Those libraries are NOT bundled in the
# AppImage; they are a runtime dependency of any desktop Linux.
#
# Prerequisites (Debian/Ubuntu):
#   sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
set -euo pipefail

icon="${1:-icons/original.png}"
base="${2:-dominion_0.1}"

here="$(cd "$(dirname "$0")" && pwd)"
out="$here/../client_app"
desktop="$here/desktop"
cd "$here"

[ -f "$icon" ] || { echo "icon not found: $icon" >&2; exit 1; }

pkg-config --exists gtk+-3.0 || { echo "missing libgtk-3-dev" >&2; exit 1; }
pkg-config --exists webkit2gtk-4.1 || { echo "missing libwebkit2gtk-4.1-dev" >&2; exit 1; }

# webview_go asks pkg-config for webkit2gtk-4.0, which Debian 13 dropped. The
# bundled header dlopen()s libwebkit2gtk-4.1 at runtime, so a shim .pc that is a
# copy of the installed 4.1 one is all that is needed.
shim_dir="$desktop/.pkgconfig"
mkdir -p "$shim_dir"
# Only the module name changes (pkg-config looks it up by filename); the Libs
# inside still reference the installed 4.1 library, which is correct.
sed 's/^Name:.*/Name: webkit2gtk-4.0/' \
  /usr/lib/x86_64-linux-gnu/pkgconfig/webkit2gtk-4.1.pc > "$shim_dir/webkit2gtk-4.0.pc"

# The Go embed directives cannot reach ../www, so stage a copy.
rm -rf "$desktop/www"
mkdir -p "$desktop/www"
cp "$here/www/index.html" "$here/www/bootstrap.js" "$desktop/www/"

echo "==> building desktop binary"
( cd "$desktop" && \
  PKG_CONFIG_PATH="$shim_dir${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}" \
  CGO_ENABLED=1 go build -trimpath -ldflags "-s -w" -o dominion-desktop . )

# ---- AppDir ----
appdir="$(mktemp -d)/dominion.AppDir"
mkdir -p "$appdir/usr/bin" "$appdir/usr/share/icons/hicolor/256x256/apps"
install -m 0755 "$desktop/dominion-desktop" "$appdir/usr/bin/dominion-desktop"

cat > "$appdir/dominion.desktop" <<'EOF'
[Desktop Entry]
Type=Application
Name=dominion
Comment=Connect to a dominion tmux portal
Exec=dominion-desktop
Icon=dominion
Categories=TerminalEmulator;Network;
EOF

# Icon: use the 1024 square source resized to 256.
convert "$icon" -resize 256x256 PNG32:"$appdir/dominion.png"
cp "$appdir/dominion.png" "$appdir/usr/share/icons/hicolor/256x256/apps/dominion.png"

cat > "$appdir/AppRun" <<'EOF'
#!/bin/sh
exec "$(dirname "$0")/usr/bin/dominion-desktop" "$@"
EOF
chmod +x "$appdir/AppRun"

# ---- appimagetool (downloaded once, no FUSE needed) ----
tool="$here/.appimagetool"
if [ ! -x "$tool" ]; then
  echo "==> fetching appimagetool"
  curl -sSLo "$tool" \
    "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage"
  chmod +x "$tool"
fi

mkdir -p "$out"
echo "==> packaging $base.AppImage"
rm -f "$out/$base.AppImage"
ARCH=x86_64 "$tool" --appimage-extract-and-run "$appdir" "$out/$base.AppImage" >/dev/null

# Clean staging.
rm -rf "$desktop/www" "$desktop/dominion-desktop"
echo
ls -lh "$out/$base.AppImage"
