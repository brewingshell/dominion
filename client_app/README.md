# client_app

Built client binaries land here. Everything except this README is **gitignored**
— these are build outputs.

| File | Icon | Platform | Size |
|------|------|----------|------|
| `dominion_0.0.2.apk` | original mark | Android | ~3.7 MB |
| `dominion_0.0.2.AppImage` | original mark | Linux desktop | ~3.2 MB |
| `dominion.apk` | alternate (personal) mark | Android | ~3.9 MB |
| `dominion.AppImage` | alternate (personal) mark | Linux desktop | ~3.2 MB |

`dominion_<version>.*` uses the project's original mark and is the build to
publish. The `dominion.*` (no version) builds use a personal icon kept
**outside** the repository (`apps/icons/terran.png`, gitignored) — keep those
local.

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
