# dominion

A tiny, self-hosted web portal for **tmux**. Every tmux session on the machine
becomes a tab, and each tab is a fully interactive terminal in the browser. One
Go binary, no runtime dependencies, no database.

It is built for a **trusted LAN**: reach your shells from a phone or laptop
without SSH clients or port forwarding.

```
┌──────────────────────────────────────────────────────────┐
│  dominion │ adjutant │ aiur │ comfy │ rabota │  ⏻  [+]   │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  $ tmux ls                                               │
│  adjutant: 1 windows (created ...)                       │
│  _                                                       │
└──────────────────────────────────────────────────────────┘
```

## Security first

dominion is a door to your shells. Read this before exposing it to anything.

- **Default PIN is `3232`.** Change it. It is a shared secret entered once per
  client; there is no per-user account model.
- **LAN only.** Do not put it on the public internet. If you must reach it from
  outside, use a VPN (e.g. Tailscale/WireGuard) rather than a port forward.
- **HTTPS with a self-signed local CA.** Traffic is encrypted, but unless you
  install the CA into the device trust store (or pin it in the native app in
  `apps/`), the certificate is **not verified**, so it does not protect against
  a man-in-the-middle on the LAN path. Installing the CA upgrades this to full
  verification.
- **Plain HTTP is accepted by default**, on the same port, for convenience: an
  `http://` request is served without encryption, so the PIN and session cookie
  travel in the clear. On a trusted LAN that is usually fine; if you would rather
  refuse it, run with `-allow-http=false` (or `-tls=false` for HTTP only).
- **The portal can create and kill tmux sessions.** Killing a session ends every
  process inside it. The `dominion` session that hosts the server is protected
  and cannot be killed.
- Login attempts are rate limited per client IP, sessions tokens are in-memory
  (a restart logs everyone out), and there is a lock button that hides the UI
  and requires the PIN again while keeping your terminals alive.

If that trade-off is acceptable for your network, the rest is pleasant to use.

## Requirements

- Linux with `tmux` installed
- Go 1.24+ (only to build; the binary is self-contained)

## Quick start

```sh
git clone https://github.com/BrewingShell/dominion
cd dominion
go build -trimpath -ldflags "-s -w" -o dominion .
./dominion
```

Then open `https://<your-host>:5550` in a browser and enter the PIN (`3232` by
default; set `DOMINION_PIN` to change it). Your browser will warn about the
self-signed certificate — see [TLS](#tls).

Run locally while testing:

```sh
./dominion -addr 127.0.0.1:5550
```

## Run as a service (recommended)

`install.sh` builds if needed, writes a systemd **user** unit with the checkout's
real path, creates the PIN env file, and starts the unit:

```sh
./install.sh
systemctl --user status dominion      # check
systemctl --user restart dominion     # after a rebuild
systemctl --user stop dominion        # stop
```

To start automatically without an interactive login:

```sh
loginctl enable-linger "$USER"
```

The server runs **inside a tmux session named `dominion`**, so its own log is the
first tab in the portal. `run.sh` sources `~/.config/dominion/env` (the tmux
server may predate the unit and not inherit it) and loops the server, so a Ctrl-C
from the portal tab restarts it instead of ending the session. Because tmux
detaches there is no tracked main PID (`Type=oneshot` + `RemainAfterExit`), and
`KillMode=process` plus `ExecStop` ensure stopping the unit tears down only the
`dominion` session, never your other tmux sessions.

## TLS

The server generates a local CA plus a leaf certificate on first start. The CA is
persisted, so its fingerprint is stable across restarts; the leaf is regenerated
each start with the host's current IPs.

By default the **same port answers both `https://` and `http://`**: the listener
peeks the first byte of each connection (a TLS handshake starts with `0x16`) and
routes it to TLS or plain HTTP. That way a bare `host:5550` or an `http://`
bookmark still reaches the login page instead of failing. Pass `-allow-http=false`
to refuse plaintext.

```sh
./dominion -fingerprint          # print the CA path and SHA-256 fingerprint
./dominion -tls=false            # serve HTTP only
./dominion -allow-http=false     # HTTPS only (reject plaintext)
./dominion -tls-cert cert.pem -tls-key key.pem -tls-ca ca.pem
./dominion -tls-san portal.lan   # extra SAN (repeatable)
./dominion -secure-cookies       # force Secure cookies (behind a TLS proxy)
```

Files live in `~/.config/dominion` (`ca.pem`, `ca-key.pem` 0600, `cert.pem`,
`key.pem`). Install `ca.pem` into your device trust store to remove the browser
warning; the native apps under `apps/` can pin it without any warning.

The auth cookie is marked `Secure` only when the request that set it arrived over
TLS (or when `-secure-cookies` is set), so logging in over plain HTTP works while
HTTPS sessions stay protected.

## Flags

| Flag           | Default                | Description                                        |
|----------------|------------------------|----------------------------------------------------|
| `-addr`        | `:5550`                | Listen address. Bind one interface with `192.168.x.x:5550`. |
| `-pin`         | `$DOMINION_PIN` or `3232` | PIN required to access the portal.               |
| `-tmux`        | `tmux`                 | Path to the tmux binary.                           |
| `-ttl`         | `12h`                  | How long a login lasts.                            |
| `-tls`         | `true`                 | Serve TLS with a local CA-signed certificate.      |
| `-allow-http`  | `true`                 | Also accept plain HTTP on the same port.           |
| `-secure-cookies` | `false`             | Force the `Secure` cookie attribute (TLS proxy).   |
| `-tls-dir`     | `~/.config/dominion`   | Where the CA and certificate files live.           |
| `-tls-cert`, `-tls-key`, `-tls-ca` | generated | Use existing certificate material.              |
| `-tls-san`     | none                   | Extra DNS name or IP for the certificate (repeatable). |
| `-fingerprint` |                        | Print the CA path and fingerprint, then exit.      |
| `-branding`    | `assets`               | Directory of logo overrides (see [Branding](#branding)). |

Prefer `DOMINION_PIN` over `-pin`: a flag is visible in `ps` to other local
users, an environment variable is not.

## The dominion session

- It sorts to the **front of the tab list** and is rendered in dark red.
- It has **no kill button**; the API refuses `POST /api/sessions/kill` for it
  with `403`, since killing it would stop the portal.

## Branding

The login mark is an original asset under `assets/` (`logo.svg` and a raster
`logo.png`). To use your own, drop a file into `assets/` — no rebuild needed,
just restart:

- `assets/override_logo.png` (or `.svg`/`.webp`) overrides the login mark and,
  by default, the favicon and app icon too.
- `assets/override_logo.*` is gitignored, so your branding never lands in the
  repository.
- Per-slot overrides are also honoured: `override_favicon.*`,
  `override_apple_touch.*`.

The precedence per slot is `override_<slot>.*` → `override_logo.*` → `logo.*` →
the embedded default. Point elsewhere with `-branding /path/to/dir`.

## Client apps

Thin native shells (Android APK, Linux AppImage) that prompt for the portal
address and load the same server-served UI. See [`apps/README.md`](apps/README.md).
They exist so the self-signed certificate can be accepted or pinned without a
browser warning; they do not reimplement the terminal.

Build them into [`client_app/`](client_app/README.md) (gitignored outputs):

```sh
cd apps
./build-client.sh            # AppImage + APK
./build-client.sh desktop    # just the AppImage
./build-client.sh android    # just the APK
```

## Development

```sh
go vet ./...
go test ./...
./dominion -addr 127.0.0.1:5550    # local-only while testing
```

The frontend has a jsdom test harness (dev-only; not embedded in the binary):

```sh
npm install
npm test
```

Layout:

```
main.go                    flags, embedded web assets, TLS listener
internal/tmux/             session listing, exact targeting, name validation
internal/auth/             PIN check, session tokens, rate limiting
internal/ptybridge/        PTY <-> WebSocket bridge with resize, keepalive,
                           and periodic token revalidation
internal/tlsconf/          local CA + leaf certificate generation
internal/server/           HTTP routes, auth middleware, static files, branding
web/                       index.html, app.js, style.css, vendor/ (xterm.js)
assets/                    logo source + override drop-in
test/                      jsdom harness for web/app.js (dev-only)
apps/                      Android APK + Linux AppImage shells
client_app/                built client binaries (gitignored)
```

## API

All routes are same-origin and, except `/healthz` and `/api/login`, require the
auth cookie.

| Method | Path                     | Purpose                                  |
|--------|--------------------------|------------------------------------------|
| `GET`  | `/healthz`               | Liveness probe, unauthenticated.         |
| `POST` | `/api/login`             | `{"pin":"..."}` → sets the auth cookie.  |
| `POST` | `/api/lock`              | Require the PIN again; keeps terminals.  |
| `POST` | `/api/logout`            | Revoke the token; closes terminals.      |
| `GET`  | `/api/sessions`          | List tmux sessions.                      |
| `POST` | `/api/sessions/create`   | `{"name":"..."}` create a session.       |
| `POST` | `/api/sessions/kill`     | `{"name":"..."}` kill a session.         |
| `GET`  | `/api/attach?session=…`  | WebSocket; PTY bridge for one session.   |

Sessions are targeted exactly (`tmux -t =name`), so a name that is a prefix or
glob of another can never hit the wrong session.

## Notes

- Sessions can be created and killed from the portal. The `+` button opens a name
  dialog and attaches to the new session on success. Each tab has an `×` that
  asks for confirmation and warns when the session is attached by a real external
  client (portal attachments are not counted).
- The last open tab is restored on reload (via `?session=<name>`) and when the
  native apps re-enter (via `localStorage`). A fresh client opens the first
  non-pinned session.
- Lock keeps open terminals and scrollback; log out and token expiry close
  attached sockets with WebSocket code `4001`. On shutdown the server closes
  terminals with `1001`.
- Static assets are served with a strict Content-Security-Policy
  (`connect-src 'self'`). If a browser ever refuses the same-origin WebSocket,
  widen it to `connect-src 'self' ws: wss:` in `internal/server/server.go`.

## tmux coexistence

`~/.tmux.conf` settings so the web client and physical terminals do not fight
over window size:

```tmux
set -g window-size latest
set -g aggressive-resize on
set -g mouse on
```

Reload after editing: `tmux source-file ~/.tmux.conf`

## License

MIT — see [LICENSE](LICENSE). Third-party components are listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
