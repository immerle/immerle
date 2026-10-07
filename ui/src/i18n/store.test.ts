import AsyncStorage from '@react-native-async-storage/async-storage';
import { i18n } from './index';
import { useLocale } from './store';

jest.mock('@react-native-async-storage/async-storage', () => ({
  getItem: jest.fn().mockResolvedValue(null),
  setItem: jest.fn().mockResolvedValue(undefined),
}));

describe('locale preference', () => {
  afterEach(() => { useLocale.getState().setPreference('system'); jest.restoreAllMocks(); });

  it('updates the translator before notifying subscribers', () => {
    let translated = '';
    const unsubscribe = useLocale.subscribe(() => { translated = i18n.t('media.player.queue'); });
    useLocale.getState().setPreference('en');
    expect(translated).toBe('Queue');
    useLocale.getState().setPreference('fr');
    expect(translated).toBe('File');
    unsubscribe();
  });

  it('ignores unsupported persisted preferences', async () => {
    useLocale.getState().setPreference('en');
    jest.spyOn(AsyncStorage, 'getItem').mockResolvedValue('invalid');
    await useLocale.getState().hydrate();
    expect(useLocale.getState().preference).toBe('en');
    expect(i18n.locale).toBe('en');
  });
});
