# dominion client apps

Thin native shells around the dominion web portal. They do **not** reimplement
the terminal: each app prompts for the portal address once, remembers it, and
loads the server-served UI. All portal behaviour — xterm, the mobile key
toolbar, lock/logout, session create/kill — comes from the server unchanged.

| Target | Stack | Size |
|--------|-------|------|
| Android | Capacitor + system WebView | ~3.7 MB APK |
| Linux desktop | Go + system WebView (WebKitGTK) | ~3.2 MB AppImage |

Neither bundles a browser engine. The Android app uses the platform WebView and
the desktop app links the system WebKitGTK, so both stay a few megabytes. (This
is why the desktop shell is Go rather than Electron: Electron bundles all of
Chromium, ~170 MB.)

## Layout

```
apps/
  package.json          Capacitor toolchain (Android only)
  capacitor.config.ts
  www/                  shared address prompt + bootstrap
  android-overlay/      custom Android files copied into the generated project
  desktop/              Go + webview_go desktop client (its own Go module)
  icons/                square app icons + regenerate.sh
  build-variant.sh      one icon variant -> APK + AppImage
  build-desktop.sh      desktop AppImage only
  build-client.sh       convenience wrapper (default icon)
  release.sh            publish the original-mark build to a GitHub release
```

`apps/android/` is **generated** by Capacitor and is gitignored; only
`android-overlay/` is tracked, and `build-variant.sh` copies it in.

## Address prompt and memory

`www/index.html` + `bootstrap.js` ask for `host[:port]` (default port `5550`).
The value is stored once and reused on every launch. A checkbox selects plain
HTTP instead of HTTPS. Behaviour differs by shell:

- **Android** navigates top-level to the address. (A subresource `fetch` probe
  is blocked there: the app's origin is `https://localhost`, so an `http://`
  probe is mixed content and an `https://` one is gated by certificate rules.)
- **Desktop** probes with Go's `net/http` (no browser restrictions), then
  navigates; on failure it shows the prompt with an error.
- **Browser** uses a `no-cors` `fetch` probe and offers **Connect anyway**.

**Settings**: the app bar has a settings button with an **Appearance** toggle
(Dark / Light) and, in a native shell, **Manage servers…** which opens this
screen. Light/dark follows the whole portal UI (login, tabs, terminals, dialogs)
and is remembered in `localStorage`; the pre-connect prompt stays dark.

**Saved servers**: the shell settings screen remembers servers by name. Pick one
to connect immediately, or delete it. Stored on desktop in
`~/.config/dominion/client.json` and on Android in Capacitor Preferences
(capped at 20).

The portal gates this entry on the shell's capability version: desktop requires
the `dominionOpenSettings` binding, and Android advertises `dominion-shell/<major>.<minor>`
via `appendUserAgent` and must be at least **1.1**. An older client hides the
entry rather than showing a button its shell cannot service — so a change to the
shell feature set needs a **client rebuild** (`apps/capacitor.config.ts` is the
Android version source).

## Certificate handling

- **Android** trusts the bundled CA (`android-overlay/sync-ca.sh` copies the
  server's `ca.pem` to `res/raw/`); `network_security_config.xml` applies it in
  `base-config` for every host, since the address is entered at runtime. Run
  `sync-ca.sh` after the server regenerates its CA. Set `DOMINION_CA` to a PEM
  path to bundle a specific CA (CI supplies the canonical one from a secret); it
  otherwise defaults to `~/.config/dominion/ca.pem`.
- **Desktop** links the system WebKitGTK, which has no certificate-bypass hook,
  so the app defaults to **plain HTTP**. HTTPS works if the CA is installed in
  the OS trust store.

Do **not** replace Capacitor's `WebViewClient`: it serves the app bundle from
`https://localhost` via `shouldInterceptRequest`. `MainActivity` *subclasses*
`BridgeWebViewClient` (keeping that hook) to (a) keep `http`/`https` navigation
in-app instead of handing it to the system browser and (b) return to the prompt
on a failed main-frame load.

## Build

Artifacts are collected in [`../client_app/`](../client_app/README.md).

Prerequisites:

```sh
# Android
export ANDROID_HOME="$HOME/Android"
export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"   # needs jlink
# Desktop
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
```

```sh
./build-client.sh                                      # default icon, both targets
DOMINION_VERSION=0.0.2 ./build-variant.sh dominion_0.0.2 icons/original.png icons/original-foreground.png
./build-desktop.sh icons/original.png dominion_0.0.2   # AppImage only
```

`DOMINION_VERSION` sets the release version: it drives the APK's
`versionName`/`versionCode` and the requested output name (default `0.0.2`).
`DOMINION_CA` selects the CA bundled into the APK.

`build-variant.sh` takes an output base, a 1024×1024 icon, and a 1024×1024
transparent adaptive-foreground. Launcher icons are drawn with alpha and the
adaptive background is transparent by default (`BG_COLOR=#rrggbb` for a solid
plate). Regenerate the square icons from the source marks with
`./icons/regenerate.sh`.

> Desktop note: `webview_go` asks pkg-config for `webkit2gtk-4.0`, which
> Debian 13 and Ubuntu 24.04 dropped (they ship only 4.1, which the library
> `dlopen`s at runtime). `build-desktop.sh` and CI run `desktop/gen-pkgconfig.sh`
> first. To build the module by hand:
>
> ```sh
> cd desktop
> go generate ./...                                  # writes .pkgconfig/webkit2gtk-4.0.pc
> PKG_CONFIG_PATH=$PWD/.pkgconfig go build .
> ```
>
> `desktop/www/` is a tracked, synced copy of `apps/www/` (`go:embed` cannot
> reach outside the module); `build-desktop.sh` and CI diff the two and fail on
> drift.

## Publish

The root `release.sh` is the supported path; `apps/release.sh` only republishes
the client pair (APK + AppImage) for an existing version.

```sh
./release.sh 0.0.2     # client-only: builds dominion_0.0.2.*, uploads both to release v0.0.2
```

For a full release, run the root script instead — it publishes the static server
and terminal client, `ca.pem`, the Android APK, the Linux AppImage, and
`SHA256SUMS` together. `--no-publish` builds and stages `dist/` without
publishing:

```sh
cd ..
./release.sh 0.0.2              # server + TUI + clients
./release.sh 0.0.2 --server-only  # server + TUI only
./release.sh 0.0.2 --no-publish   # build to dist/, do not publish
```

## Notes / limitations

- The desktop AppImage depends on the system WebKitGTK at runtime (present on
  any desktop Linux, not on servers).
- **Window icon**: the shell names itself `dominion` (X11 `WM_CLASS`, Wayland
  `app_id`) and loads the icon from the AppImage for X11 (`_NET_WM_ICON`). On
  Wayland the compositor has no per-window icon protocol and resolves the icon
  from an installed `dominion.desktop`/`dominion.png`; integrate the AppImage
  (e.g. AppImageLauncher or `appimaged`) for the title bar/task switcher to show
  it. Without integration the window keeps the toolkit default.
- iOS is not built here (needs macOS + signing).
