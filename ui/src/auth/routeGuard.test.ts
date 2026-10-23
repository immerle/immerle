import { authRedirectHref, isPublicAuthPath, loginHref, safeReturnTo } from './routeGuard';

describe('auth route guard', () => {
  it('allows only entry and authentication routes without a session', () => {
    expect(isPublicAuthPath('/')).toBe(true);
    expect(isPublicAuthPath('/login')).toBe(true);
    expect(isPublicAuthPath('/setup')).toBe(true);
    expect(isPublicAuthPath('/album/123')).toBe(false);
  });

  it('preserves a protected deep link through login or first-run setup', () => {
    expect(authRedirectHref('/album/123', false)).toBe('/login?returnTo=%2Falbum%2F123');
    expect(authRedirectHref('/album/123', true)).toBe('/setup?returnTo=%2Falbum%2F123');
    expect(loginHref('/album/123')).toBe('/login?returnTo=%2Falbum%2F123');
  });

  it('rejects external and authentication-loop return paths', () => {
    expect(safeReturnTo('//example.com/path')).toBe('/(tabs)');
    expect(safeReturnTo('/\\\\example.com/path')).toBe('/(tabs)');
    expect(safeReturnTo('/login')).toBe('/(tabs)');
    expect(safeReturnTo('/setup?returnTo=/album/123')).toBe('/(tabs)');
    expect(safeReturnTo(undefined)).toBe('/(tabs)');
  });
});
