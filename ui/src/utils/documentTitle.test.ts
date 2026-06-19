import { documentTitle } from './documentTitle';

describe('documentTitle', () => {
  it('titles static routes', () => {
    expect(documentTitle('/')).toBe('Accueil · Immerle');
    expect(documentTitle('/login')).toBe('Connexion · Immerle');
    expect(documentTitle('/playlists')).toBe('Playlists · Immerle');
  });

  it('titles dynamic detail routes by their segment', () => {
    expect(documentTitle('/album/42')).toBe('Album · Immerle');
    expect(documentTitle('/profile/kilian')).toBe('Profil · Immerle');
  });

  it('titles admin subpages', () => {
    expect(documentTitle('/admin')).toBe('Admin · Immerle');
    expect(documentTitle('/admin/users')).toBe('Utilisateurs · Immerle');
    expect(documentTitle('/admin/unknown')).toBe('Admin · Immerle');
  });

  it('falls back to the app name for unknown routes', () => {
    expect(documentTitle('/nope')).toBe('Immerle');
  });
});
