const PARAM = '__EXPO_ROUTER_key';

/**
 * Drop expo-router's internal `__EXPO_ROUTER_key` query param from a URL/path.
 * expo-router adds it on PUSH actions to force a fresh stack entry, after which
 * it leaks into the web address bar (e.g. `/liked?__EXPO_ROUTER_key=...`). It is
 * write-only — never read back — so removing it from the visible URL is safe.
 */
export function stripRouterKey(url: string): string {
  if (!url.includes(PARAM)) return url;
  const u = new URL(url, 'http://_');
  u.searchParams.delete(PARAM);
  return u.pathname + u.search + u.hash;
}

let installed = false;

/**
 * Web only: wrap `history.pushState`/`replaceState` so the router key never
 * reaches the address bar. Idempotent; a no-op off the web. The history `state`
 * object is preserved untouched, so back/forward keep working.
 */
export function installRouterKeyStripper(): void {
  if (installed || typeof window === 'undefined' || !window.history) return;
  installed = true;
  (['pushState', 'replaceState'] as const).forEach((name) => {
    const original = window.history[name].bind(window.history);
    window.history[name] = (state: unknown, unused: string, url?: string | URL | null) => {
      const next = typeof url === 'string' ? stripRouterKey(url) : url;
      original(state, unused, next ?? undefined);
    };
  });
}
