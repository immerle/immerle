// Minimal, hardened preload. Context isolation is on and Node integration is
// off, so the renderer (the Expo web app) runs as a plain web page. We expose a
// tiny read-only surface for desktop-aware UI tweaks if ever needed — nothing
// privileged.
const { contextBridge } = require('electron');

contextBridge.exposeInMainWorld('desktop', {
  isElectron: true,
  platform: process.platform,
});
