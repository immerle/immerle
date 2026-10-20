import AsyncStorage from '@react-native-async-storage/async-storage';
import { useSearchUI } from './store';

jest.mock('@react-native-async-storage/async-storage', () => ({
  getItem: jest.fn().mockResolvedValue(null),
  setItem: jest.fn().mockResolvedValue(undefined),
  removeItem: jest.fn().mockResolvedValue(undefined),
}));

const RECENTS_KEY = 'immerle.searchRecents.v1';

describe('recent searches', () => {
  beforeEach(() => {
    useSearchUI.setState({
      query: '',
      open: false,
      activeIndex: 0,
      typeFilter: 'all',
      recents: [],
    });
    jest.clearAllMocks();
    jest.spyOn(AsyncStorage, 'getItem').mockResolvedValue(null);
    jest.spyOn(AsyncStorage, 'setItem').mockResolvedValue(undefined);
    jest.spyOn(AsyncStorage, 'removeItem').mockResolvedValue(undefined);
  });

  it('removes one search and persists the remaining entries', async () => {
    useSearchUI.setState({ recents: ['flaca', 'dirt', 'pan y mantequilla'] });

    useSearchUI.getState().removeRecent('dirt');
    await Promise.resolve();

    expect(useSearchUI.getState().recents).toEqual(['flaca', 'pan y mantequilla']);
    expect(AsyncStorage.setItem).toHaveBeenCalledWith(
      RECENTS_KEY,
      JSON.stringify(['flaca', 'pan y mantequilla']),
    );
  });

  it('clears every search and removes its persisted value', async () => {
    useSearchUI.setState({ recents: ['flaca', 'dirt'] });

    useSearchUI.getState().clearRecents();
    await Promise.resolve();

    expect(useSearchUI.getState().recents).toEqual([]);
    expect(AsyncStorage.removeItem).toHaveBeenCalledWith(RECENTS_KEY);
  });

  it('ignores malformed entries when restoring recent searches', async () => {
    jest.spyOn(AsyncStorage, 'getItem').mockResolvedValue(JSON.stringify(['flaca', 42, '', 'dirt']));

    await useSearchUI.getState().loadRecents();

    expect(useSearchUI.getState().recents).toEqual(['flaca', 'dirt']);
  });

  it('does not write storage when asked to remove a search that is not present', () => {
    useSearchUI.setState({ recents: ['flaca'] });

    useSearchUI.getState().removeRecent('dirt');

    expect(useSearchUI.getState().recents).toEqual(['flaca']);
    expect(AsyncStorage.setItem).not.toHaveBeenCalled();
  });
});
