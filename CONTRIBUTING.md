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

## Guidelines

- Keep the server dependency-light. It embeds its own frontend and talks to
  `tmux` and a PTY; avoid pulling in frameworks.
- Session names are always passed to tmux as argv with an exact target
  (`-t =name`) — never build shell strings.
- Add a test with behaviour changes: table-driven Go tests for the backend,
  `test/*.test.mjs` (jsdom) for the frontend.
- One logical change per commit, with a message that explains the why.

## Reporting

- Bugs and feature ideas: GitHub issues.
- Security problems: see [SECURITY.md](SECURITY.md), not the public tracker.
