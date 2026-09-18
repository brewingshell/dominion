# Third-party notices

dominion bundles the following third-party components. Their licenses are
reproduced or referenced below; the full texts ship with each project.

## JavaScript (vendored under `web/vendor/`)

- **xterm.js** — MIT License — https://github.com/xtermjs/xterm.js
  Pinned to `@xterm/xterm` 6.0.0 with local input patches for Android soft
  keyboards / IMEs; see `CONTRIBUTING.md` ("Vendored xterm.js") and
  `test/xterm-patch.mjs`. The patches are minor and remain under the MIT
  license.
- **@xterm/addon-fit** — MIT License — https://github.com/xtermjs/xterm.js

## Go (server)

- **github.com/creack/pty** — BSD 3-Clause License —
  https://github.com/creack/pty
- **github.com/gorilla/websocket** — BSD 3-Clause License —
  https://github.com/gorilla/websocket

## Go (terminal client)

- **github.com/rivo/tview** — MIT License — https://github.com/rivo/tview
- **github.com/gdamore/tcell/v2** — Apache-2.0 License —
  https://github.com/gdamore/tcell
- **golang.org/x/term** — BSD 3-Clause License —
  https://cs.opensource.google/go/x/term

## Client apps (`apps/`)

Android shell:

- **Capacitor** (`@capacitor/*`) — MIT License — https://capacitorjs.com

Desktop shell (Go + system WebView):

- **github.com/webview/webview_go** — MIT License —
  https://github.com/webview/webview_go
- **WebKitGTK** — LGPL-2.1-or-later / BSD — https://webkitgtk.org
- **GTK 3** — LGPL-2.1-or-later — https://gtk.org

WebKitGTK and GTK are **runtime dependencies of the host system**, linked
dynamically and not bundled into the AppImage. Capacitor and webview_go are
build-time dependencies and not embedded in the Go server binary.
