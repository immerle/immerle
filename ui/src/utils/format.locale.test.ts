import { i18n } from '../i18n';
import { formatBytes, formatRelativeTime } from './format';

describe('localized formatters', () => {
  const originalLocale = i18n.locale;
  beforeEach(() => { jest.spyOn(Date, 'now').mockReturnValue(Date.UTC(2026, 0, 1)); });
  afterEach(() => { i18n.locale = originalLocale; jest.restoreAllMocks(); });
  const ago = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString();

  it('uses English units, relative labels and year pluralization', () => {
    i18n.locale = 'en';
    expect(formatBytes(0)).toBe('0 B');
    expect(formatBytes(1024 ** 3)).toBe('1.0 GB');
    expect(formatRelativeTime(ago(0))).toBe('just now');
    expect(formatRelativeTime(ago(2))).toBe('2 d');
    expect(formatRelativeTime(ago(14))).toBe('2 wk');
    expect(formatRelativeTime(ago(365))).toBe('1 year');
    expect(formatRelativeTime(ago(730))).toBe('2 years');
  });

  it('preserves French units and relative labels', () => {
    i18n.locale = 'fr';
    expect(formatBytes(1024 ** 3)).toBe('1.0 Go');
    expect(formatRelativeTime(ago(0))).toBe("à l'instant");
    expect(formatRelativeTime(ago(2))).toBe('2 j');
    expect(formatRelativeTime(ago(365))).toBe('1 an');
    expect(formatRelativeTime(ago(730))).toBe('2 ans');
  });

  it('handles invalid input without producing untranslated or invalid values', () => {
    i18n.locale = 'en';
    expect(formatBytes(Infinity)).toBe('0 B');
    expect(formatRelativeTime('invalid')).toBe('');
  });
});
