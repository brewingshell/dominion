#!/usr/bin/env bash
# Copy the server's CA certificate into the Android raw resources so the app
# can pin it via network_security_config.xml.
#
# Usage: ./sync-ca.sh [path-to-ca.pem]
# Default source: ~/.config/dominion/ca.pem
set -euo pipefail

src="${1:-$HOME/.config/dominion/ca.pem}"
here="$(cd "$(dirname "$0")" && pwd)"
dest="$here/res-raw/dominion_ca.pem"

if [ ! -f "$src" ]; then
  echo "CA not found at $src" >&2
  echo "Retrieve it with:  dominion -fingerprint   (prints the CA path)" >&2
  exit 1
fi

mkdir -p "$(dirname "$dest")"
cp "$src" "$dest"
echo "Copied $src -> $dest"
echo "Now copy res-raw/dominion_ca.pem and network_security_config.xml into"
echo "android/app/src/main/res/ after running 'npx cap add android'."
