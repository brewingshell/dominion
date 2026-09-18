# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.0.3]

### Fixed

- Killing a session no longer returns `500` when the tmux server is shutting
  down after its last session. A failed `kill-session` (including the silent,
  empty-output case) is now confirmed against `has-session`, so the API still
  reports `404 Not Found`.

## [0.0.2]

### Added

- `dominion-client-tui`: a terminal client for a portal. It logs in with the
  PIN, lists sessions, creates and kills them, and attaches to one over the
  portal's WebSocket PTY bridge (Ctrl-] detaches). Shares the saved-server
  address book at `~/.config/dominion/client.json`.
- `-version` on the server and the TUI, populated from the release tag.
- A tag-driven release pipeline (`.github/workflows/release.yml`) that builds and
  publishes the server, TUI, Android APK, Linux AppImage, and the bundled
  `ca.pem` in one GitHub release.
- `release.sh` gains `--no-publish` and bundles `DOMINION_CA` for the APK.
- `get.sh` gains `DOMINION_TUI=1` to install the terminal client.
- Shell clients advertise a capability version (`dominion-shell/1.1`); the
  portal gates saved-server management on it so an older client hides the
  control instead of showing a dead button.
- Desktop AppImage window identity and icon support (X11 `_NET_WM_ICON`, Wayland
  `app_id`).

### Fixed

- Vendored xterm.js carries local Android soft-keyboard/IME input fixes, and a
  test guards the bundle against drift.

[Unreleased]: https://github.com/brewingshell/dominion/compare/v0.0.3...HEAD
[0.0.3]: https://github.com/brewingshell/dominion/compare/v0.0.2...v0.0.3
[0.0.2]: https://github.com/brewingshell/dominion/releases/tag/v0.0.2
