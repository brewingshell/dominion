#!/usr/bin/env bash
# Publish the distributable (original-mark) client builds as a GitHub release.
#
#   ./release.sh 0.1
#
# Builds the dominion_<version> variant and attaches its APK and AppImage to
# the release tagged v<version>. Only the original-mark build is published; the
# alternate-icon builds stay local.
#
# Note: the AppImage is ~104 MiB, above GitHub's 100 MiB limit for files
# committed to a repository, so it is uploaded as a release asset (2 GiB cap)
# rather than pushed to git.
#
# Auth: needs a token with "Contents: write", in GITHUB_TOKEN, or the gh CLI
# logged in.
set -euo pipefail

version="${1:?usage: ./release.sh <version>   e.g. ./release.sh 0.1}"
here="$(cd "$(dirname "$0")" && pwd)"
repo="brewingshell/dominion"
tag="v$version"
base="dominion_$version"

cd "$here"

echo "==> building $base"
./build-variant.sh "$base" icons/original.png icons/original-foreground.png

apk="../client_app/$base.apk"
appimage="../client_app/$base.AppImage"
for f in "$apk" "$appimage"; do
  [ -f "$f" ] || { echo "missing $f" >&2; exit 1; }
done

notes="dominion client $tag — Android APK and Linux AppImage.

Both prompt for the portal address on first run. See client_app/README.md."

if command -v gh >/dev/null 2>&1; then
  gh release create "$tag" "$apk" "$appimage" \
    --repo "$repo" --title "$tag" --notes "$notes"
  echo "published $tag via gh"
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

# Upload each asset.
for f in "$apk" "$appimage"; do
  name="$(basename "$f")"
  echo "==> uploading $name"
  curl -sS -X POST \
    "https://uploads.github.com/repos/$repo/releases/$(python3 -c 'import json;print(json.load(open("/tmp/dominion-release.json"))["id"])')/assets?name=$name" \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$f" > /dev/null
done
rm -f /tmp/dominion-release.json
echo "published $tag"
