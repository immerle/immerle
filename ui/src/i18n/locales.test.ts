import en from './locales/en.json';
import fr from './locales/fr.json';

function leaves(value: Record<string, unknown>, prefix = ''): Record<string, string> {
  return Object.fromEntries(Object.entries(value).flatMap(([key, item]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    return typeof item === 'string'
      ? [[path, item]]
      : Object.entries(leaves(item as Record<string, unknown>, path));
  }));
}

describe('translation catalogs', () => {
  it('keeps English and French keys and interpolation parameters aligned', () => {
    const english = leaves(en);
    const french = leaves(fr);
    expect(Object.keys(english).sort()).toEqual(Object.keys(french).sort());
    for (const [key, value] of Object.entries(english)) {
      expect(value.trim()).not.toBe('');
      expect(french[key].trim()).not.toBe('');
      const params = (text: string) => [...text.matchAll(/\{\{(\w+)\}\}/g)].map((m) => m[1]).sort();
      expect({ key, params: params(value) }).toEqual({ key, params: params(french[key]) });
    }
  });
});
