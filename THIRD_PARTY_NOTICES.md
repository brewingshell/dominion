# Third-party notices

dominion bundles the following third-party components. Their licenses are
reproduced or referenced below; the full texts ship with each project.

## JavaScript (vendored under `web/vendor/`)

- **xterm.js** — MIT License — https://github.com/xtermjs/xterm.js
- **@xterm/addon-fit** — MIT License — https://github.com/xtermjs/xterm.js

## Go (server)

- **github.com/creack/pty** — BSD 3-Clause License —
  https://github.com/creack/pty
- **github.com/gorilla/websocket** — BSD 3-Clause License —
  https://github.com/gorilla/websocket

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
