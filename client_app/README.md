# client_app

Built client binaries land here. Everything except this README is **gitignored**
— these are build outputs.

| File | Platform | Size |
|------|----------|------|
| `dominion_<version>.apk` | Android | ~3.7 MB |
| `dominion_<version>.AppImage` | Linux desktop | ~3.2 MB |

Both use the project's original mark and are the builds to publish; the root
`release.sh` builds them here and stages copies for the GitHub release.
`DOMINION_VERSION` sets the `_<version>` suffix (default `0.0.2`).

> These are build outputs and do **not** update themselves. When the portal
> gains or changes a shell feature (for example saved servers, which need
> client capability `dominion-shell/1.1`), every client must be **rebuilt**;
> an older build simply hides the new control. Run `./build-client.sh` (or
> `./build-variant.sh` per icon) after any change under `apps/`.

## Build

From `apps/` (the version suffix comes from `DOMINION_VERSION`, default `0.0.2`):

```sh
DOMINION_VERSION=0.0.2 ./build-variant.sh dominion_0.0.2 icons/original.png icons/original-foreground.png
```

Prerequisites: an Android SDK + a JDK with `jlink` (`ANDROID_HOME`,
`JAVA_HOME`), and — for the desktop AppImage — `libgtk-3-dev` and
`libwebkit2gtk-4.1-dev`. Regenerate the square icons from the source marks with
`./icons/regenerate.sh`.

## Publishing

```sh
cd ..
./release.sh 0.0.2      # server + TUI + clients -> GitHub release v0.0.2
```

Needs a token with **Contents: write** in `GITHUB_TOKEN`, or the `gh` CLI logged
in. CI runs the same script from a tag (see `.github/workflows/release.yml`).

## Use

- **AppImage**: `chmod +x dominion_0.0.2.AppImage && ./dominion_0.0.2.AppImage`
  (needs system WebKitGTK at runtime)
- **APK**: `adb install dominion_0.0.2.apk` (or copy to the device and open it;
  allow install from unknown sources).

Both remember the portal `host:port` once entered, and the login screen has a
**Change server** button to correct it. See
[`../apps/README.md`](../apps/README.md) for certificate handling and details.
