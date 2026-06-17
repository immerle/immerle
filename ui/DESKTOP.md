# Immerle Desktop (Electron)

The desktop app reuses the exact same code as the web target: Electron loads the
Expo **web** build. No separate UI codebase. The web audio engine
(`engine.web.ts`, `HTMLAudioElement` + Media Session) and the `localStorage`
secure-store fallback both work inside Electron's Chromium.

## Layout

- `electron/main.js` — main process. In dev it loads the Expo web dev server
  (hot reload); when packaged it serves the static `dist/` over a loopback HTTP
  server on a **fixed** port (`41734`) and loads it. The port is fixed on purpose:
  the session lives in `localStorage`, keyed by origin, so it must stay stable
  across launches. A single-instance lock keeps that port free.
- `electron/preload.js` — minimal, hardened bridge (`window.desktop`). Context
  isolation on, Node integration off, sandbox on.
- `electron-builder.yml` — packaging config (mac/win/linux targets, icons).

## Install

The desktop toolchain lives in `devDependencies` (`electron`,
`electron-builder`, `concurrently`, `wait-on`). Install them once:

```bash
npm install
```

> Downloading the Electron binary needs network access (~100 MB). Behind a proxy,
> set `ELECTRON_MIRROR` / `electron_config_cache` as usual.

## Develop

Run the web dev server and the Electron shell together:

```bash
npm run desktop:dev
```

This starts `expo start --web`, waits for `:8081`, then launches Electron pointed
at it (with DevTools). Edit React code and it hot-reloads in the window.

To attach Electron to an already-running web server instead:

```bash
npm run web        # in one terminal
npm run desktop    # in another (uses ELECTRON_START_URL, default http://localhost:8081)
```

## Build installers

Each script first exports the web bundle (`expo export --platform web` → `dist/`),
then runs electron-builder. Output goes to `release/`.

```bash
npm run desktop:pack          # unpacked app for the current OS (quick smoke test)
npm run desktop:dist:mac      # macOS .dmg + .zip (x64 + arm64)
npm run desktop:dist:win      # Windows NSIS installer
npm run desktop:dist:linux    # Linux AppImage + .deb
npm run desktop:dist          # current-OS targets from electron-builder.yml
```

### Cross-compilation caveat

electron-builder packages for the host OS best:

- **macOS** builds (`.dmg`) must run on macOS (code signing / `hdiutil`).
- **Windows** (`.exe`) builds on Windows, or on macOS/Linux with Wine installed.
- **Linux** (`AppImage`/`.deb`) builds on Linux (or via Docker).

For all three reliably, build each on its own OS or in CI (GitHub Actions
`runs-on: [macos-latest, windows-latest, ubuntu-latest]`).

## Notes

- The backend URL is whatever you enter at login (e.g. `http://localhost:4533`).
  The backend must allow CORS from the desktop origin (`http://127.0.0.1:41734`
  in packaged builds, `http://localhost:8081` in dev) — same as the web client.
- Icons are reused from `assets/icon.png`. electron-builder derives the
  per-platform formats; drop a 512×512+ PNG there (or platform-specific
  `build/icon.icns` / `build/icon.ico`) to customize.
