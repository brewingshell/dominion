# client_app

Built client binaries land here. Everything except this README is **gitignored**
— these are build outputs.

| File | Icon | Platform | Size |
|------|------|----------|------|
| `dominion_0.1.apk` | original mark | Android | ~3.7 MB |
| `dominion_0.1.AppImage` | original mark | Linux desktop | ~3.2 MB |
| `dominion.apk` | alternate (personal) mark | Android | ~3.9 MB |
| `dominion.AppImage` | alternate (personal) mark | Linux desktop | ~3.2 MB |

`dominion_0.1.*` uses the project's original mark and is the build to publish;
`dominion.apk` is also committed to git. The `dominion.*` (no version) builds use
a personal icon kept **outside** the repository (`apps/icons/terran.png`,
gitignored) — keep those local.

## Build

From `apps/`:

```sh
./build-variant.sh dominion_0.1 icons/original.png icons/original-foreground.png
```

Prerequisites: an Android SDK + a JDK with `jlink` (`ANDROID_HOME`,
`JAVA_HOME`), and — for the desktop AppImage — `libgtk-3-dev` and
`libwebkit2gtk-4.1-dev`. Regenerate the square icons from the source marks with
`./icons/regenerate.sh`.

## Publishing

```sh
cd apps
./release.sh 0.1        # builds dominion_0.1.*, uploads both to GitHub release v0.1
```

Needs a token with **Contents: write** in `GITHUB_TOKEN`, or the `gh` CLI logged
in.

## Use

- **AppImage**: `chmod +x dominion_0.1.AppImage && ./dominion_0.1.AppImage`
  (needs system WebKitGTK at runtime)
- **APK**: `adb install dominion_0.1.apk` (or copy to the device and open it;
  allow install from unknown sources).

Both remember the portal `host:port` once entered, and the login screen has a
**Change server** button to correct it. See
[`../apps/README.md`](../apps/README.md) for certificate handling and details.
