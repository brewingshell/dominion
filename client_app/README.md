# client_app

Built client binaries land here. The contents of this directory (other than this
README) are **gitignored** — they are build outputs, not source.

| File | Icon | Platform | Built by |
|------|------|----------|----------|
| `dominion.AppImage` | Terran Dominion | Linux desktop | `build-variant.sh dominion` |
| `dominion.apk` | Terran Dominion | Android | `build-variant.sh dominion` |
| `dominion_0.1.AppImage` | original mark | Linux desktop | `build-variant.sh dominion_0.1` |
| `dominion_0.1.apk` | original mark | Android | `build-variant.sh dominion_0.1` |

The `dominion.*` build uses a personal/third-party icon kept **outside** the
repository (`apps/icons/terran.png`, gitignored); `dominion_0.1.*` uses the
project's original mark (`apps/icons/original.png`, committed).

## Build

From the `apps/` directory:

```sh
./build-client.sh            # both (default icons)
./build-client.sh desktop    # AppImage only
./build-client.sh android    # APK only

# icon variants
./build-variant.sh dominion_0.1 icons/original.png icons/original-foreground.png
BG_COLOR='#000000' ./build-variant.sh dominion icons/terran.png icons/terran-foreground.png
```

Android builds need an SDK and a JDK that includes `jlink`:

```sh
export ANDROID_HOME="$HOME/Android"
export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
```

The AppImage is written directly here via the `directories.output` setting in
`apps/package.json`; the script copies the Gradle APK in and renames it.

## Use

- **AppImage**: `chmod +x dominion-*.AppImage && ./dominion-*.AppImage`
- **APK**: sideload with `adb install dominion-debug.apk` (or copy to the device
  and open it; allow install from unknown sources).

Both prompt for the portal `host:port` on first run. See
[`../apps/README.md`](../apps/README.md) for certificate handling and details.
