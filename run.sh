#!/bin/sh
# Runs the dominion server inside a tmux session so its log is a live tab in the
# portal. Launched by dominion.service as: tmux new-session -d -s dominion ...
#
# The tmux server may predate the systemd unit, in which case the unit's
# EnvironmentFile is not inherited here, so source it explicitly. Passing the
# PIN via the environment (never -pin) keeps it out of ps and pane_start_command.
set -u

here="$(cd "$(dirname "$0")" && pwd)"
bin="${DOMINION_BIN:-$here/dominion}"
tmux_bin="${DOMINION_TMUX:-/usr/bin/tmux}"
addr="${DOMINION_ADDR:-:5550}"

PATH=/usr/local/bin:/usr/bin:/bin
export PATH

if [ -f "${DOMINION_ENV:-$HOME/.config/dominion/env}" ]; then
  # shellcheck disable=SC1091
  . "${DOMINION_ENV:-$HOME/.config/dominion/env}"
fi

if [ ! -x "$bin" ]; then
  echo "[dominion] $bin not found or not executable; build it with: go build -o dominion ."
fi

# Restart the server if it exits (e.g. Ctrl-C typed into the portal tab), so the
# tab stays alive and the log is preserved.
while :; do
  "$bin" -addr "$addr" -tmux "$tmux_bin"
  code=$?
  echo "[dominion] exited with status $code; restarting in 2s"
  sleep 2
done
