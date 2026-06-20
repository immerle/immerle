// Immerle service worker — gives the web app installability + an offline app
// shell. It deliberately does NOT touch the API or media: /api/ and /rest/ (which
// includes streaming, downloads and cover art) always go straight to the network.
// Offline *media* on web is handled separately by the app (downloaded tracks are
// stored in IndexedDB and played from a blob: URL — see src/offline/fs.web.ts),
// not by this cache.
//
// ponytail: hand-rolled runtime caching, no Workbox/precache-manifest build step.
// Upgrade path: if exact asset versioning ever matters, generate a precache list
// at build time instead of the cache-first heuristic below.

const CACHE = 'immerle-shell-v1';
const APP_SHELL = '/';

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((c) => c.add(APP_SHELL))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  // Leave anything that isn't this origin, and all API/media traffic, untouched.
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/rest/')) return;

  // Page navigations: network-first, fall back to the cached shell when offline.
  if (req.mode === 'navigate') {
    event.respondWith(fetch(req).catch(() => caches.match(APP_SHELL)));
    return;
  }

  // Hashed, immutable build assets: cache-first.
  if (url.pathname.startsWith('/_expo/') || url.pathname.startsWith('/assets/') || url.pathname.startsWith('/icons/')) {
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ||
          fetch(req).then((res) => {
            const copy = res.clone();
            caches.open(CACHE).then((c) => c.put(req, copy));
            return res;
          }),
      ),
    );
    return;
  }

  // Other same-origin GETs (manifest, favicon, …): network, fall back to cache.
  event.respondWith(fetch(req).catch(() => caches.match(req)));
});
