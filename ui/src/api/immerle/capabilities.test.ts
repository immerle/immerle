import { adaptCapabilities, SUBSONIC_ONLY_CAPABILITIES } from './capabilities';
import type { CapabilitiesResponse } from '../immerleApi';

describe('adaptCapabilities', () => {
  it('maps an empty capability map to the conservative feature set', () => {
    const caps = adaptCapabilities({ ok: true, capabilities: {} } as CapabilitiesResponse);
    expect(caps.features).toEqual(SUBSONIC_ONLY_CAPABILITIES.features);
  });

  it('enables only the features the server advertises, masking the rest', () => {
    const caps = adaptCapabilities({
      ok: true,
      server: 'immerle',
      protocolVersion: '1.0.0',
      capabilities: {
        federation: { version: 1 },
        jam: { version: 1 },
        friendships: { version: 1 },
        collaborativePlaylists: { version: 1 },
      },
    } as CapabilitiesResponse);

    expect(caps.version).toBe('1.0.0');
    expect(caps.features.federation).toBe(true);
    expect(caps.features.jam).toBe(true);
    expect(caps.features.collaborativePlaylists).toBe(true);
    // friendships ⇒ social
    expect(caps.features.social).toBe(true);
    // Unadvertised features stay off.
    expect(caps.features.onDemandCatalog).toBe(false);
    expect(caps.features.adminExtended).toBe(false);
    expect(caps.features.immerleAuth).toBe(false);
  });

  it('treats activityFeed or shares as social too', () => {
    const caps = adaptCapabilities({
      ok: true,
      capabilities: { activityFeed: { version: 1 } },
    } as CapabilitiesResponse);
    expect(caps.features.social).toBe(true);
  });

  it('keeps a togglable feature "supported" while off, but reports it disabled in toggles', () => {
    // Regression: an admin switching Hall of Fame off must hide its sidebar
    // entry (isFeatureEnabled), without breaking the admin toggle UI, which
    // needs the feature to stay "supported" (has) so it can be switched back on.
    const caps = adaptCapabilities({
      ok: true,
      capabilities: { hallOfFame: { version: 1, admin: true, enabled: false } },
    } as CapabilitiesResponse);
    expect(caps.features.hallOfFame).toBe(true);
    expect(caps.toggles.hallOfFame).toBe(false);
  });

  it('reports a togglable feature enabled when the server says so', () => {
    const caps = adaptCapabilities({
      ok: true,
      capabilities: { hallOfFame: { version: 1, admin: true, enabled: true } },
    } as CapabilitiesResponse);
    expect(caps.features.hallOfFame).toBe(true);
    expect(caps.toggles.hallOfFame).toBe(true);
  });

  it('leaves toggles empty for an unadvertised feature', () => {
    const caps = adaptCapabilities({ ok: true, capabilities: {} } as CapabilitiesResponse);
    expect(caps.toggles.hallOfFame).toBeUndefined();
  });
});
