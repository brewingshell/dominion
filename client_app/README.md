# client_app

Built client binaries land here. The contents of this directory (other than this
README) are **gitignored**: they are build outputs, and the AppImage is larger
than GitHub's 100 MiB per-file limit for a repository. Distribute them as
**release assets** instead — see [Publishing](#publishing).

| File | Icon | Platform |
|------|------|----------|
| `dominion_<version>.apk` | original mark | Android |
| `dominion_<version>.AppImage` | original mark | Linux desktop |
| `dominion.apk` | alternate (personal) mark | Android |
| `dominion.AppImage` | alternate (personal) mark | Linux desktop |

The `dominion_<version>.*` builds use the project's original mark
(`apps/icons/original.png`) and are the ones to publish. The `dominion.*` builds
use a personal icon kept **outside** the repository (`apps/icons/terran.png`,
gitignored) — keep those local.

## Build

From the `apps/` directory:

```sh
./build-variant.sh dominion_0.1 icons/original.png icons/original-foreground.png
```

Regenerate the square icons from the source marks first if needed:

```sh
./icons/regenerate.sh
```

Android builds need an SDK and a JDK that includes `jlink`:

```sh
export ANDROID_HOME="$HOME/Android"
export JAVA_HOME="$HOME/jdk/jdk-17.0.12+7"
```

## Publishing

The AppImage is ~104 MiB, above GitHub's 100 MiB git limit, so publish both
artifacts as a **release** (assets allow up to 2 GiB):

```sh
cd apps
./release.sh 0.1        # builds dominion_0.1.* and uploads them to tag v0.1
```

Needs a token with **Contents: write** in `GITHUB_TOKEN`, or the `gh` CLI logged
in. Only the original-mark build is published.

## Use

- **AppImage**: `chmod +x dominion_0.1.AppImage && ./dominion_0.1.AppImage`
- **APK**: `adb install dominion_0.1.apk` (or copy to the device and open it;
  allow install from unknown sources).

Both prompt for the portal `host:port` on first run. See
[`../apps/README.md`](../apps/README.md) for certificate handling and details.
