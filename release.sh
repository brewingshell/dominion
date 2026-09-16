#!/usr/bin/env bash
# Build and publish a dominion release: the self-contained server binary plus the
# optional Android/desktop clients, all attached to one GitHub release.
#
#   ./release.sh 0.1                 # server binary + clients (needs their toolchains)
#   ./release.sh 0.1 --server-only   # just the server binary and checksums
#
# The server is built with CGO_ENABLED=0, so the binary is static and runs on any
# x86-64 Linux (glibc or musl) with no runtime dependencies beyond tmux.
#
# Client builds shell out to apps/build-variant.sh and need the Android SDK/JDK
# and GTK/WebKitGTK dev packages; see apps/README.md. Set SKIP_CLIENTS=1 as an
# alternative to --server-only, or use DOMINION_ICON to pick another icon set.
#
# Auth: the gh CLI logged in, or GITHUB_TOKEN with "Contents: write".
set -euo pipefail

version="${1:?usage: ./release.sh <version> [--server-only]   e.g. ./release.sh 0.1}"
mode="${2:-}"
here="$(cd "$(dirname "$0")" && pwd)"
repo="brewingshell/dominion"
tag="v$version"
dist="$here/dist"
cd "$here"

server_only=0
[ "$mode" = "--server-only" ] && server_only=1
[ "${SKIP_CLIENTS:-0}" = "1" ] && server_only=1

rm -rf "$dist"
mkdir -p "$dist"

assets=()

echo "==> server: dominion_${version}_linux_amd64 (static)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w" -o "$dist/dominion_${version}_linux_amd64" .
assets+=("dominion_${version}_linux_amd64")

if [ "$server_only" = "1" ]; then
  echo "==> clients: skipped"
else
  echo "==> clients: dominion_$version (Android APK + Linux AppImage)"
  ./apps/build-variant.sh "dominion_$version" \
    apps/icons/original.png apps/icons/original-foreground.png
  cp "client_app/dominion_$version.apk" "client_app/dominion_$version.AppImage" "$dist/"
  assets+=("dominion_$version.apk" "dominion_$version.AppImage")
fi

echo "==> checksums"
# Build SHA256SUMS from the staged assets only, so the manifest never lists
# itself.
( cd "$dist" && sha256sum "${assets[@]}" > SHA256SUMS )
cat "$dist/SHA256SUMS"

notes="dominion $tag — server binary and client builds.

Server: static linux/amd64 binary (no runtime dependencies beyond tmux).
Clients: Android APK and Linux AppImage (see apps/README.md).
Verify downloads against SHA256SUMS, or install the server with get.sh:

  curl -fsSL https://raw.githubusercontent.com/$repo/master/get.sh | sh"

paths=()
for a in "${assets[@]}"; do paths+=("$dist/$a"); done
paths+=("$dist/SHA256SUMS")

if command -v gh >/dev/null 2>&1; then
  echo "==> publishing $tag via gh"
  gh release create "$tag" "${paths[@]}" \
    --repo "$repo" --title "$tag" --notes "$notes"
  echo "published $tag"
  exit 0
fi

: "${GITHUB_TOKEN:?set GITHUB_TOKEN (Contents: write) or install gh}"
api="https://api.github.com/repos/$repo"

echo "==> creating release $tag"
curl -sS -X POST "$api/releases" \
  -H "Authorization: Bearer $GITHUB_TOKEN" \
  -H "Accept: application/vnd.github+json" \
  -d "{\"tag_name\":\"$tag\",\"name\":\"$tag\",\"body\":$(printf '%s' "$notes" | python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()))')}" \
  > /tmp/dominion-release.json

rid="$(python3 -c 'import json;print(json.load(open("/tmp/dominion-release.json"))["id"])')"
for f in "${paths[@]}"; do
  name="$(basename "$f")"
  echo "==> uploading $name"
  curl -sS -X POST \
    "https://uploads.github.com/repos/$repo/releases/$rid/assets?name=$name" \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$f" > /dev/null
done
rm -f /tmp/dominion-release.json
echo "published $tag"
