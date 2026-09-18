#!/bin/sh
# Install dominion from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/brewingshell/dominion/master/get.sh | sh
#
# It downloads the static server binary, verifies it against SHA256SUMS, installs
# it (with run.sh) into DOMINION_INSTALL_DIR, seeds ~/.config/dominion/.env, and
# enables the systemd user service when possible.
#
# Options (environment variables):
#   DOMINION_VERSION=v0.1      install a specific release (default: latest)
#   DOMINION_INSTALL_DIR=DIR   where to put the binary and run.sh (~/.local/bin)
#   DOMINION_CONFIG_DIR=DIR    where the .env lives (~/.config/dominion)
#   DOMINION_TUI=1             also install the terminal client (dominion-client-tui)
#   DOMINION_NO_SERVICE=1      do not touch systemd; just install the files
#
# Linux/tmux only. For the from-source path and other options, see the README.
set -eu

repo="brewingshell/dominion"
raw="https://raw.githubusercontent.com/$repo"

say()  { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v sha256sum >/dev/null 2>&1 || die "sha256sum is required"

[ "$(uname -s)" = "Linux" ] || die "dominion targets Linux (found $(uname -s))"
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  *) die "no prebuilt binary for $(uname -m); build from source (see README)" ;;
esac

if command -v tmux >/dev/null 2>&1; then
  :
else
  warn "tmux was not found on PATH; dominion needs it at runtime (e.g. apt install tmux)"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

tag="${DOMINION_VERSION:-}"
if [ -z "$tag" ]; then
  tag="$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" \
    | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
fi
[ -n "$tag" ] || die "could not find a release; set DOMINION_VERSION=vX.Y"
case "$tag" in
  v*) ;;
  *) tag="v$tag" ;;
esac
ver="${tag#v}"
asset="dominion_${ver}_linux_amd64"
base="https://github.com/$repo/releases/download/$tag"

say "==> downloading $asset ($tag)"
curl -fsSL -o "$tmp/$asset" "$base/$asset" || die "download failed: $base/$asset"
curl -fsSL -o "$tmp/SHA256SUMS" "$base/SHA256SUMS" || die "could not fetch SHA256SUMS"

say "==> verifying checksum"
# Extract just this asset's line so a missing entry fails loudly instead of
# being treated as "ignored".
if ! ( cd "$tmp" && grep "  $asset\$" SHA256SUMS > "$tmp/$asset.sums" && sha256sum -c "$tmp/$asset.sums" ); then
  die "checksum verification failed; refusing to install"
fi

# run.sh is colocated with the binary so its "$here/dominion" resolves.
curl -fsSL -o "$tmp/run.sh" "$raw/$tag/run.sh" || die "could not fetch run.sh"

install_dir="${DOMINION_INSTALL_DIR:-$HOME/.local/bin}"
mkdir -p "$install_dir"
install -m 0755 "$tmp/$asset" "$install_dir/dominion"
install -m 0755 "$tmp/run.sh" "$install_dir/run.sh"
say "==> installed $install_dir/dominion"

if [ "${DOMINION_TUI:-0}" = "1" ]; then
  tui_asset="dominion-client-tui_${ver}_linux_amd64"
  say "==> downloading $tui_asset ($tag)"
  curl -fsSL -o "$tmp/$tui_asset" "$base/$tui_asset" || die "download failed: $base/$tui_asset"
  if ! ( cd "$tmp" && grep "  $tui_asset\$" SHA256SUMS > "$tmp/$tui_asset.sums" && sha256sum -c "$tmp/$tui_asset.sums" ); then
    die "checksum verification failed for $tui_asset"
  fi
  install -m 0755 "$tmp/$tui_asset" "$install_dir/dominion-client-tui"
  say "==> installed $install_dir/dominion-client-tui"
fi

conf_dir="${DOMINION_CONFIG_DIR:-$HOME/.config/dominion}"
mkdir -p "$conf_dir"
if [ -f "$conf_dir/.env" ]; then
  say "==> keeping existing $conf_dir/.env"
else
  if curl -fsSL -o "$tmp/.env.example" "$raw/$tag/.env.example"; then
    install -m 600 "$tmp/.env.example" "$conf_dir/.env"
    say "==> wrote $conf_dir/.env (PIN defaults to 1111 — change it)"
  else
    warn "could not fetch .env.example; continuing without a config file"
  fi
fi

if [ "${DOMINION_NO_SERVICE:-0}" != "1" ] && command -v systemctl >/dev/null 2>&1; then
  unit_dir="$HOME/.config/systemd/user"
  mkdir -p "$unit_dir"
  if curl -fsSL -o "$tmp/dominion.service" "$raw/$tag/dominion.service"; then
    escaped="$(printf '%s' "$install_dir" | sed 's/[&|]/\\&/g')"
    sed "s|__INSTALL_DIR__|$escaped|g" "$tmp/dominion.service" > "$unit_dir/dominion.service"
    if systemctl --user daemon-reload 2>/dev/null; then
      systemctl --user enable --now dominion >/dev/null 2>&1 \
        || warn "could not start the service; run: systemctl --user start dominion"
      say "==> enabled systemd user service dominion"
    else
      warn "user systemd is not reachable; run it directly: $install_dir/dominion"
    fi
  else
    warn "could not fetch dominion.service; skipping service install"
  fi
fi

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) say ""; say "Add dominion to your PATH:"; say "  export PATH=\"$install_dir:\$PATH\"" ;;
esac

say ""
say "dominion is installed. Open https://<this-host>:5550 and enter the PIN"
say "(1111 by default; change it in $conf_dir/.env)."
if [ "${DOMINION_TUI:-0}" = "1" ]; then
  say "Run the terminal client with: dominion-client-tui --server <host>"
fi
say "Keep it running without an interactive login: loginctl enable-linger \"\$USER\""
