// Electron main process: wraps the exported Expo web build into a desktop app
// for macOS, Windows and Linux.
//
// In development it loads the running Expo web dev server (hot reload). In a
// packaged build it serves the static `dist/` over a LOOPBACK HTTP server on a
// FIXED port and loads that. The fixed port matters: the app persists its
// session in `localStorage`, which is keyed by origin (host + port), so the
// port must stay stable across launches for the user to remain logged in. A
// single-instance lock guarantees the port is always free for the first window.

const { app, BrowserWindow, shell } = require('electron');
const http = require('http');
const fs = require('fs');
const path = require('path');

const DEV_URL = process.env.ELECTRON_START_URL || 'http://localhost:8081';
// Arbitrary high port, kept constant so the renderer origin (and thus its
// localStorage / session) is stable between runs.
const PROD_PORT = 41734;
const PROD_HOST = '127.0.0.1';

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.map': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.ttf': 'font/ttf',
  '.otf': 'font/otf',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.wasm': 'application/wasm',
  '.txt': 'text/plain; charset=utf-8',
};

/**
 * Serve the static web build from `root`. Unknown paths fall back to
 * `index.html` so client-side routing keeps working after a hard reload.
 * Reads work transparently through the asar archive in packaged builds.
 */
function startStaticServer(root) {
  return new Promise((resolve, reject) => {
    const server = http.createServer((req, res) => {
      let pathname = '/';
      try {
        pathname = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
      } catch {
        pathname = '/';
      }
      let filePath = path.normalize(path.join(root, pathname));
      // Guard against path traversal outside the served root.
      if (filePath !== root && !filePath.startsWith(root + path.sep)) {
        res.writeHead(403);
        res.end('Forbidden');
        return;
      }

      const sendFile = (p, status = 200) => {
        fs.readFile(p, (err, data) => {
          if (err) {
            res.writeHead(404);
            res.end('Not found');
            return;
          }
          res.writeHead(status, { 'Content-Type': MIME[path.extname(p).toLowerCase()] || 'application/octet-stream' });
          res.end(data);
        });
      };

      fs.stat(filePath, (err, stat) => {
        if (!err && stat.isDirectory()) filePath = path.join(filePath, 'index.html');
        fs.access(filePath, fs.constants.R_OK, (accessErr) => {
          if (accessErr) {
            // SPA fallback: let expo-router resolve the path on the client.
            sendFile(path.join(root, 'index.html'));
            return;
          }
          sendFile(filePath);
        });
      });
    });
    server.on('error', reject);
    server.listen(PROD_PORT, PROD_HOST, () => resolve(server));
  });
}

let mainWindow = null;

async function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 900,
    minHeight: 600,
    title: 'Immerle',
    backgroundColor: '#121212',
    show: false,
    icon: path.join(__dirname, '..', 'assets', 'icon.png'),
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  });

  // Avoid the white flash before the renderer paints.
  mainWindow.once('ready-to-show', () => mainWindow.show());

  // Open external links (http/https not on our origin) in the system browser.
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('http://') || url.startsWith('https://')) {
      void shell.openExternal(url);
    }
    return { action: 'deny' };
  });

  if (!app.isPackaged) {
    await mainWindow.loadURL(DEV_URL);
    mainWindow.webContents.openDevTools({ mode: 'detach' });
  } else {
    const root = path.join(app.getAppPath(), 'dist');
    await startStaticServer(root);
    await mainWindow.loadURL(`http://${PROD_HOST}:${PROD_PORT}/`);
  }

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

// Only allow a single running instance so the fixed port is always available
// (and a second launch focuses the existing window instead of failing).
const gotLock = app.requestSingleInstanceLock();
if (!gotLock) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.focus();
    }
  });

  app.whenReady().then(createWindow);

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) void createWindow();
  });

  app.on('window-all-closed', () => {
    // Standard platform behaviour: quit everywhere except macOS.
    if (process.platform !== 'darwin') app.quit();
  });
}
