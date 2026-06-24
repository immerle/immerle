# EAS — iOS deployment

The repo is EAS-ready: see [`eas.json`](./eas.json) for the build profiles
(`development`, `preview`, `production`). This is the iOS-only runbook. (Android
is intentionally left out for now — just don't pass `--platform android`.)

## Prerequisites (one-time)

- An **Expo account** (free): https://expo.dev/signup
- A **paid Apple Developer account** ($99/yr) — required for device/TestFlight/App
  Store distribution. A free Apple ID only signs local dev builds, not EAS
  distribution.
- The CLI, via npx (no need to install globally or pin it):
  ```bash
  npx eas-cli@latest --version
  ```

## 1. Link the project (writes `extra.eas.projectId` into app.json)

```bash
cd ui
npx eas-cli login
npx eas-cli init           # creates/links the EAS project, fills the projectId
```

## 2. iOS credentials

Let EAS manage them (recommended) — it creates the distribution cert + provisioning
profile on the first build, or set them up explicitly:

```bash
npx eas-cli credentials -p ios
```

## 3. Build

- **Simulator** build (no Apple account needed, runs in the iOS Simulator):
  ```bash
  npx eas-cli build -p ios --profile development
  ```
- **Internal/device** build (ad-hoc, installable via a link on registered
  devices — needs the paid account):
  ```bash
  npx eas-cli build -p ios --profile preview
  ```
- **App Store / TestFlight** build:
  ```bash
  npx eas-cli build -p ios --profile production
  ```

`appVersionSource` is `remote` and `production` has `autoIncrement: true`, so the
build number is managed by EAS — no need to bump it in `app.json`.

## 4. Submit to App Store Connect (TestFlight)

```bash
npx eas-cli submit -p ios --latest
```
This prompts for App Store Connect auth (Apple ID or an ASC API key). To make it
non-interactive in CI, add an `ios` block under `submit.production` in `eas.json`
with `appleId`, `ascAppId`, `appleTeamId` (or an `ascApiKeyPath`).

## Notes

- EAS runs a **managed prebuild** in the cloud, so it regenerates `ios/` from
  `app.json` + any config plugins and applies `patches/` (patch-package) on every
  build — you don't commit `ios/`. EAS images ship their own Xcode, so the local
  Xcode-26.5 workarounds aren't needed there.
- OTA JS updates (optional) go through EAS Update on the matching `channel`
  (`development` / `preview` / `production`): `npx eas-cli update --branch <channel>`.
