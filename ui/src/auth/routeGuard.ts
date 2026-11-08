const PUBLIC_PATHS = new Set(['/', '/login', '/setup']);
const APP_HOME = '/(tabs)';

/** Paths that may render before a restored session exists. */
export function isPublicAuthPath(pathname: string): boolean {
  return PUBLIC_PATHS.has(pathname.replace(/\/$/, '') || '/');
}

/** Keep a login return path local to this app and avoid redirect loops. */
export function safeReturnTo(value: unknown): string {
  const candidate = Array.isArray(value) ? value[0] : value;
  if (typeof candidate !== 'string' || !candidate.startsWith('/') || candidate.startsWith('//') || candidate.includes('\\')) {
    return APP_HOME;
  }
  if (candidate === '/' || candidate === '/login' || candidate.startsWith('/login?') || candidate === '/setup' || candidate.startsWith('/setup?')) {
    return APP_HOME;
  }
  return candidate;
}

/** Build a same-origin login/setup URL that preserves the requested deep link. */
export function authRedirectHref(pathname: string, needsSetup: boolean): string {
  const route = needsSetup ? '/setup' : '/login';
  const returnTo = safeReturnTo(pathname);
  return `${route}?returnTo=${encodeURIComponent(returnTo)}`;
}

export function loginHref(returnTo: unknown): string {
  const destination = safeReturnTo(returnTo);
  return destination === APP_HOME ? '/login' : `/login?returnTo=${encodeURIComponent(destination)}`;
}
