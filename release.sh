#!/usr/bin/env bash
# Build and publish a dominion release: the self-contained server binary, the
# terminal client, and the optional Android/desktop clients, all attached to one
# GitHub release.
#
#   ./release.sh 0.0.2                 # server + TUI + clients (needs their toolchains)
#   ./release.sh 0.0.2 --server-only   # server + TUI only
#   ./release.sh 0.0.2 --no-publish    # build and stage dist/, do not publish
#
# The server and TUI are built with CGO_ENABLED=0, so they are static and run on
# any x86-64 Linux (glibc or musl) with no runtime dependencies beyond tmux.
#
# The Android APK bundles the CA in DOMINION_CA (a PEM path): the official build
# must trust the maintainer's server. CI supplies it from a secret; a local run
# defaults to ~/.config/dominion/ca.pem when that exists.
#
# Client builds shell out to apps/build-variant.sh and need the Android SDK/JDK
# and GTK/WebKitGTK dev packages; see apps/README.md. Set SKIP_CLIENTS=1 as an
# alternative to --server-only, or use DOMINION_ICON to pick another icon set.
#
# Auth: the gh CLI logged in, or GITHUB_TOKEN with "Contents: write". Ignored
# under --no-publish.
set -euo pipefail

version=""
server_only=0
publish=1
for arg in "$@"; do
  case "$arg" in
    --server-only) server_only=1 ;;
    --no-publish)  publish=0 ;;
    -*)            echo "unknown option: $arg" >&2; exit 2 ;;
    *)             version="$arg" ;;
  esac
done
[ -n "$version" ] || {
  echo "usage: ./release.sh <version> [--server-only] [--no-publish]   e.g. ./release.sh 0.0.2" >&2
  exit 2
}
[ "${SKIP_CLIENTS:-0}" = "1" ] && server_only=1

here="$(cd "$(dirname "$0")" && pwd)"
repo="brewingshell/dominion"
tag="v$version"
dist="$here/dist"
cd "$here"

rm -rf "$dist"
mkdir -p "$dist"

assets=()

echo "==> server: dominion_${version}_linux_amd64 (static)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -X main.version=$version" \
  -o "$dist/dominion_${version}_linux_amd64" .
assets+=("dominion_${version}_linux_amd64")

echo "==> tui: dominion-client-tui_${version}_linux_amd64 (static)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags "-s -w -X main.version=$version" \
  -o "$dist/dominion-client-tui_${version}_linux_amd64" ./cmd/dominion-client-tui
assets+=("dominion-client-tui_${version}_linux_amd64")

# Bundle the canonical CA. A local run falls back to the generated CA so the
# APK keeps its pinned trust without an explicit variable.
ca="${DOMINION_CA:-}"
if [ -z "$ca" ] && [ -f "$HOME/.config/dominion/ca.pem" ]; then
  ca="$HOME/.config/dominion/ca.pem"
fi
if [ -n "$ca" ]; then
  [ -f "$ca" ] || { echo "DOMINION_CA not found: $ca" >&2; exit 1; }
  cp "$ca" "$dist/ca.pem"
  assets+=("ca.pem")
  export DOMINION_CA="$ca"
elif [ "$server_only" != "1" ]; then
  # The APK pins a CA; without one build-variant.sh fails later and less
  # clearly. Stop here instead.
  echo "error: no CA to bundle; set DOMINION_CA to a PEM path (the APK needs it)" >&2
  exit 1
fi

if [ "$server_only" = "1" ]; then
  echo "==> clients: skipped"
else
  echo "==> clients: dominion_$version (Android APK + Linux AppImage)"
  DOMINION_VERSION="$version" \
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

if [ "$publish" = "0" ]; then
  echo "==> publish skipped (--no-publish); artifacts in $dist"
  exit 0
fi

notes="dominion $tag — server binary, terminal client, and client builds.

$(awk -v heading="## [$version]" '
  $0 == heading { grab=1; next }
  grab && /^## / { exit }
  grab { print }
' CHANGELOG.md)

Server + TUI: static linux/amd64 binaries (no runtime dependencies beyond tmux).
Clients: Android APK and Linux AppImage (see apps/README.md).
ca.pem is the CA the bundled Android client trusts; install it for HTTPS.
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
