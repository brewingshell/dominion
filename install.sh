#!/bin/sh
# Install dominion as a systemd user service. Run from the checkout:
#
#   ./install.sh
#
# It writes ~/.config/systemd/user/dominion.service with the checkout's real
# absolute path, creates the env file if missing, and starts the unit. The
# server then runs inside a tmux session named "dominion".
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
unit_dir="$HOME/.config/systemd/user"
unit="$unit_dir/dominion.service"
env_file="$HOME/.config/dominion/env"

if [ ! -x "$here/dominion" ]; then
  echo "Building dominion..."
  (cd "$here" && go build -trimpath -ldflags "-s -w" -o dominion .)
fi

mkdir -p "$unit_dir" "$HOME/.config/dominion"

if [ ! -f "$env_file" ]; then
  printf 'DOMINION_PIN=3232\n' > "$env_file"
  chmod 600 "$env_file"
  echo "Wrote $env_file (change the PIN!)."
fi

sed "s|__INSTALL_DIR__|$here|g" "$here/dominion.service" > "$unit"
echo "Installed $unit"

systemctl --user daemon-reload
systemctl --user enable --now dominion
systemctl --user status dominion --no-pager || true

echo
echo "To keep it running without an interactive login:"
echo "  loginctl enable-linger \"\$USER\""
