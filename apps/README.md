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

## Certificate handling

- **Android** trusts the bundled CA (`android-overlay/sync-ca.sh` copies the
  server's `ca.pem` to `res/raw/`); `network_security_config.xml` applies it in
  `base-config` for every host, since the address is entered at runtime. Run
  `sync-ca.sh` after the server regenerates its CA.
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
./build-variant.sh dominion_0.1 icons/original.png icons/original-foreground.png
./build-desktop.sh icons/original.png dominion_0.1      # AppImage only
```

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

```sh
./release.sh 0.1     # builds dominion_0.1.*, uploads both to GitHub release v0.1
```

## Notes / limitations

- The desktop AppImage depends on the system WebKitGTK at runtime (present on
  any desktop Linux, not on servers).
- iOS is not built here (needs macOS + signing).
