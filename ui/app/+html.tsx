import { ScrollViewStyleReset } from 'expo-router/html';
import { type PropsWithChildren } from 'react';

// +html.tsx wraps the root HTML document for the static web export only (it never
// runs on native). We use it to turn the web build into an installable PWA:
// manifest, theme color, Apple home-screen meta, and service-worker registration.
export default function Root({ children }: PropsWithChildren) {
  return (
    <html lang="en">
      <head>
        <meta charSet="utf-8" />
        <meta httpEquiv="X-UA-Compatible" content="IE=edge" />
        <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no, viewport-fit=cover" />

        {/* PWA */}
        <link rel="manifest" href="/manifest.json" />
        <meta name="theme-color" content="#121212" />
        <link rel="apple-touch-icon" href="/icons/icon-192.png" />
        <meta name="apple-mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-title" content="Immerle" />
        <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />

        {/* Expo's reset so body/#root fill the viewport and scroll behaves. */}
        <ScrollViewStyleReset />

        <script dangerouslySetInnerHTML={{ __html: swRegister }} />
      </head>
      <body>{children}</body>
    </html>
  );
}

// Registers the service worker after load. A failed/absent /sw.js (e.g. in dev)
// is swallowed — the app works fine without it, just without offline/install.
const swRegister = `
if ('serviceWorker' in navigator) {
  window.addEventListener('load', function () {
    navigator.serviceWorker.register('/sw.js').catch(function () {});
  });
}`;
