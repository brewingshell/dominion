# dominion client apps

Thin native shells around the dominion web portal. They do **not** reimplement
the terminal: each app prompts for the portal address on first run, then loads
the server-served UI in a WebView (Electron / Android). All portal behaviour —
xterm, the mobile key toolbar, lock/logout, session create/kill — comes from the
server unchanged.

## Why the apps exist

The portal is served over HTTPS with a **local, self-signed CA** (see the root
README). A native shell can:

- accept that certificate programmatically (no browser warning), and
- optionally verify it by bundling the CA.

A browser cannot do either without installing the CA into the OS trust store.

## Layout

```
apps/
  package.json            Capacitor + Electron toolchain
  capacitor.config.ts
  www/                    bootstrap: address prompt -> navigate to the portal
  electron/               Electron main process (AppImage)
  android-overlay/        custom Android files to copy into the Capacitor project
```

`apps/android/` is **generated** by Capacitor and is gitignored. Only the custom
files live in the repo, under `android-overlay/`.

## Address prompt

`www/index.html` asks for `host[:port]` on first run (default port `5550`),
probes `https://host/healthz`, and stores the value. On a later launch it
reconnects automatically; if the host is unreachable the prompt returns for
editing. A checkbox allows plain HTTP instead of HTTPS. Nothing about your
network is hardcoded — replace `YOUR-HOST` only if you want a default
placeholder.

## Certificate handling

There is no fingerprint entry by design. Two options, best first:

1. **Bundle the CA (recommended).** Run `android-overlay/sync-ca.sh`, then
   reference `@raw/dominion_ca` from `network_security_config.xml`. The WebView
   then *verifies* the certificate chain. Electron can do the equivalent with a
   `session.setCertificateVerifyProc` handler that trusts the bundled CA.
2. **Accept the certificate.** Electron's `certificate-error` handler and the
   Android `SslErrorHandler.proceed()` in `android-overlay/MainActivity.java`
   accept the certificate without validation. This gives encryption but **not**
   MITM protection.

Installing the CA into the device trust store upgrades option 2 to full
verification with no app changes.

## Build

### Desktop (AppImage)

```sh
cd apps
npm install
npm run electron          # run in place
npm run electron:dist     # build AppImage into dist/
```

### Android (debug APK)

Requires the Android SDK (`sdkmanager`, platform + build-tools). Capacitor
generates the native project, then the overlay is copied in:

```sh
cd apps
npm install
npx cap add android
mkdir -p android/app/src/main/res/xml android/app/src/main/res/raw
cp android-overlay/network_security_config.xml android/app/src/main/res/xml/
cp android-overlay/MainActivity.java android/app/src/main/java/net/dominion/client/
./android-overlay/sync-ca.sh && cp android-overlay/res-raw/dominion_ca.pem android/app/src/main/res/raw/
# add android:networkSecurityConfig="@xml/network_security_config" to <application> in AndroidManifest.xml
npm run android           # cd android && ./gradlew assembleDebug
```

The debug APK lands in `android/app/build/outputs/apk/debug/`.

## Notes / limitations

- `android-overlay/MainActivity.java` and `network_security_config.xml` are
  templates: set `ALLOWED_HOST` / `<domain>` to your server address, and verify
  the `MainActivity` override against your Capacitor version (it must forward the
  other `WebViewClient` callbacks — or omit it entirely when bundling the CA).
- iOS is not built here (needs macOS + signing).
