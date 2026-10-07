import { i18n } from '../i18n';
import { documentTitle } from './documentTitle';

describe('localized document titles', () => {
  const originalLocale = i18n.locale;
  afterEach(() => { i18n.locale = originalLocale; });

  it('uses English for queue, casting and admin routes', () => {
    i18n.locale = 'en';
    expect(documentTitle('/')).toBe('Home · Immerle');
    expect(documentTitle('/login')).toBe('Sign in · Immerle');
    expect(documentTitle('/playlists')).toBe('Playlists · Immerle');
    expect(documentTitle('/album/42')).toBe('Album · Immerle');
    expect(documentTitle('/profile/kilian')).toBe('Profile · Immerle');
    expect(documentTitle('/queue')).toBe('Queue · Immerle');
    expect(documentTitle('/cast-target')).toBe('Play on · Immerle');
    expect(documentTitle('/admin/users')).toBe('Users · Immerle');
  });

  it('resolves the same route again after switching languages', () => {
    i18n.locale = 'en';
    expect(documentTitle('/queue')).toBe('Queue · Immerle');
    i18n.locale = 'fr';
    expect(documentTitle('/queue')).toBe('File · Immerle');
    expect(documentTitle('/cast-target')).toBe('Écouter sur · Immerle');
    expect(documentTitle('/admin/users')).toBe('Utilisateurs · Immerle');
  });

  it('keeps unknown routes safe and gives unknown admin sections a localized fallback', () => {
    i18n.locale = 'en';
    expect(documentTitle('/not-a-route')).toBe('Immerle');
    expect(documentTitle('/admin/not-a-route')).toBe('Admin · Immerle');
  });
});
