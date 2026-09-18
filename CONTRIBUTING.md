# Contributing

Small, focused project — patches are welcome.

## Dev loop

```sh
go build -trimpath -ldflags "-s -w" -o dominion .
./dominion -addr 127.0.0.1:5550     # local-only while testing
```

The web assets are embedded with `go:embed`, so rebuild after editing anything
under `web/`.

## Checks

Run these before opening a PR; CI runs the same:

```sh
gofmt -l .            # should print nothing
go vet ./...
go test ./...
node --check web/app.js
npm install && npm test
```

## Vendored xterm.js

`web/vendor/xterm.js` is the pinned `@xterm/xterm` `lib/xterm.js` **with local
input patches** for Android soft keyboards / IMEs (`_keyDownSeen` dropping
composed input, duplicate characters from the deferred textarea diff, and
committed composition text being re-sent after a delete). The whole bundle is
generated, not hand-edited:

```sh
node test/xterm-patch.mjs          # fetch the pinned release, patch, rewrite the bundle
node test/xterm-patch.mjs --check  # verify web/vendor/xterm.js without writing
```

To upgrade xterm: bump `VENDOR_VERSION`/`PRISTINE_SHA256` and adjust `PATCHES`
in `test/xterm-patch.mjs`, rerun the script, then set `PATCHED_SHA256` to the
hash it prints. `test/xterm-vendor.test.mjs` (part of `npm test`) fails if the
bundle drifts from that patched build.

## Guidelines

- Keep the server dependency-light. It embeds its own frontend and talks to
  `tmux` and a PTY; avoid pulling in frameworks.
- Session names are always passed to tmux as argv with an exact target
  (`-t =name`) — never build shell strings.
- Add a test with behaviour changes: table-driven Go tests for the backend,
  `test/*.test.mjs` (jsdom) for the frontend.
- One logical change per commit, with a message that explains the why.

## Releases

Releases are cut from a version tag; `.github/workflows/release.yml` runs the
same `release.sh` a maintainer would run locally:

```sh
./release.sh 0.0.2 --no-publish   # build + stage dist/, no publish
./release.sh 0.0.2                # build and publish via gh/GITHUB_TOKEN
git tag v0.0.2 && git push origin v0.0.2   # CI builds and publishes
```

- The tag (`vX.Y.Z`) is the single source of truth; it is embedded as
  `-X main.version` and drives the APK `versionName`/`versionCode`.
- The official APK bundles the canonical CA from the `DOMINION_CA_PEM_B64`
  repository secret (base64 of `~/.config/dominion/ca.pem`).
- Update `CHANGELOG.md` in the same commit that bumps behaviour. The shell
  capability version in `apps/capacitor.config.ts` is separate from the release
  version and only changes when the shell's feature set does.

## Reporting

- Bugs and feature ideas: GitHub issues.
- Security problems: see [SECURITY.md](SECURITY.md), not the public tracker.
