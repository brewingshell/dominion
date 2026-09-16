# Third-party notices

dominion bundles the following third-party components. Their licenses are
reproduced or referenced below; the full texts ship with each project.

## JavaScript (vendored under `web/vendor/`)

- **xterm.js** — MIT License — https://github.com/xtermjs/xterm.js
- **@xterm/addon-fit** — MIT License — https://github.com/xtermjs/xterm.js

## Go modules

- **github.com/creack/pty** — BSD 3-Clause License —
  https://github.com/creack/pty
- **github.com/gorilla/websocket** — BSD 3-Clause License —
  https://github.com/gorilla/websocket

## Client apps (`apps/`, development only)

The native shells are built with:

- **Capacitor** (`@capacitor/*`) — MIT License — https://capacitorjs.com
- **Electron** — MIT License — https://electronjs.org
- **electron-builder** — MIT License — https://electron.build

These are build-time dependencies and are not embedded in the Go server binary.
