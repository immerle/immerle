import { useEffect } from 'react';
import { Pressable, ScrollView, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useDebounced, useSearch } from '../query/search';
import { searchNav, SearchTypeFilter, useSearchUI } from '../search/store';
import { CoverArt } from './CoverArt';
import { PlaylistCover } from './PlaylistCover';
import { TrackRow } from './TrackRow';
import { Chip, EmptyState, Loading } from './ui';
import { Ionicon } from './Ionicon';
import { usePlayer } from '../audio/store';
import { useTrackMenu } from './trackMenu';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';
import { formatCount } from '../utils/format';
import { SearchHit } from '../api/immerle/catalog';

/** Filter chips shown above the results, in display order. */
const FILTERS: { key: SearchTypeFilter; labelKey: string }[] = [
  { key: 'all', labelKey: 'components.search.filterAll' },
  { key: 'artist', labelKey: 'components.search.artists' },
  { key: 'album', labelKey: 'components.search.albums' },
  { key: 'song', labelKey: 'components.search.songs' },
  { key: 'playlist', labelKey: 'components.search.playlists' },
];

/** i18n key for a hit's type label, shown in its row's subtitle. */
const TYPE_LABEL_KEY: Record<SearchHit['type'], string> = {
  artist: 'components.search.typeArtist',
  album: 'components.search.typeAlbum',
  song: 'components.search.typeSong',
  playlist: 'components.search.typePlaylist',
};

/**
 * Live search results, shared by the web header popover and the mobile
 * full-screen overlay. When the query is empty it shows recent searches.
 *
 * Artists, albums, songs and public playlists render as one vertical,
 * keyboard-navigable list (↑/↓/Enter, driven by `SearchOverlay`), in the
 * relevance order the backend already ranked them in — no per-type
 * grouping, just a filter row to narrow it down to one type. Selecting
 * anything records the query as recent and dismisses the search.
 */
export function SearchResults({ onClose }: { onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const query = useSearchUI((s) => s.query);
  const recents = useSearchUI((s) => s.recents);
  const setQuery = useSearchUI((s) => s.setQuery);
  const addRecent = useSearchUI((s) => s.addRecent);
  const activeIndex = useSearchUI((s) => s.activeIndex);
  const typeFilter = useSearchUI((s) => s.typeFilter);
  const setTypeFilter = useSearchUI((s) => s.setTypeFilter);
  const debounced = useDebounced(query, 250);
  const { data, isLoading, isFetching } = useSearch(debounced);
  const playSongs = usePlayer((s) => s.playSongs);
  const openMenu = useTrackMenu((s) => s.open);

  const trimmed = debounced.trim();
  const allHits = data ?? [];
  const hits = typeFilter === 'all' ? allHits : allHits.filter((h) => h.type === typeFilter);
  const songs = hits.filter((h) => h.type === 'song').map((h) => h.song);

  const select = (action: () => void) => {
    addRecent(trimmed);
    action();
    onClose();
  };

  const actionFor = (hit: SearchHit): (() => void) => {
    switch (hit.type) {
      case 'artist':
        return () => select(() => router.push(`/artist/${hit.artist.id}` as never));
      case 'album':
        return () => select(() => router.push(`/album/${hit.album.id}` as never));
      case 'playlist':
        return () => select(() => router.push(`/playlist/${hit.playlist.id}` as never));
      case 'song':
        return () => select(() => playSongs(songs, songs.indexOf(hit.song)));
    }
  };

  // Publish the flat list for SearchOverlay's keyboard handler.
  useEffect(() => {
    const flat = hits.map(actionFor);
    searchNav.count = flat.length;
    searchNav.selectAt = (i) => flat[i]?.();
    return () => {
      searchNav.count = 0;
      searchNav.selectAt = () => {};
    };
  });

  // --- Empty query → recent searches -------------------------------------
  if (trimmed.length === 0) {
    if (recents.length === 0) {
      return <EmptyState icon="search" title={t('components.search.searchTitle')} subtitle={t('components.search.searchSubtitle')} />;
    }
    return (
      <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ paddingBottom: 12 }}>
        <Text className="px-4 pb-1 pt-3 text-base font-bold text-foreground">{t('components.search.recentSearches')}</Text>
        {recents.map((r) => (
          <Pressable
            key={r}
            onPress={() => setQuery(r)}
            className="flex-row items-center gap-3 px-4 py-3 active:bg-surface-alt"
          >
            <Ionicon name="time-outline" size={20} color={colors.muted} />
            <Text className="flex-1 text-base text-foreground">{r}</Text>
            <Ionicon name="arrow-forward-outline" size={16} color={colors.muted} />
          </Pressable>
        ))}
      </ScrollView>
    );
  }

  if (isLoading) return <Loading />;
  if (!isFetching && allHits.length === 0) {
    return <EmptyState icon="sad-outline" title={t('components.search.noResults')} subtitle={t('components.search.nothingFor', { query: trimmed })} />;
  }

  return (
    <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ paddingBottom: 16 }}>
      <View className="flex-row flex-wrap gap-2 px-4 pb-2 pt-3">
        {FILTERS.map((f) => (
          <Chip key={f.key} label={t(f.labelKey)} active={typeFilter === f.key} onPress={() => setTypeFilter(f.key)} />
        ))}
      </View>
      {hits.length === 0 ? (
        <EmptyState icon="funnel-outline" title={t('components.search.noResults')} subtitle={t('components.search.nothingFor', { query: trimmed })} />
      ) : (
        hits.map((hit, i) => (
          <View key={`${hit.type}:${i}`} className={activeIndex === i ? 'bg-surface-alt' : ''}>
            <SearchHitRow hit={hit} onPress={actionFor(hit)} onMore={hit.type === 'song' ? () => openMenu(hit.song) : undefined} />
          </View>
        ))
      )}
    </ScrollView>
  );
}

function SearchHitRow({ hit, onPress, onMore }: { hit: SearchHit; onPress: () => void; onMore?: () => void }) {
  const t = useT();
  const typeLabel = t(TYPE_LABEL_KEY[hit.type]);
  switch (hit.type) {
    case 'song':
      return <TrackRow song={hit.song} typeLabel={typeLabel} onPress={onPress} onMore={onMore} />;
    case 'artist':
      return (
        <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt">
          <CoverArt coverArt={hit.artist.coverArt} url={hit.artist.artistImageUrl} size={44} rounded="rounded-full" fallbackIcon="person" />
          <View className="flex-1">
            <Text numberOfLines={1} className="text-base font-medium text-foreground">
              {hit.artist.name}
            </Text>
            <Text numberOfLines={1} className="text-sm text-muted">
              {typeLabel}
            </Text>
          </View>
        </Pressable>
      );
    case 'album':
      return (
        <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt">
          <CoverArt coverArt={hit.album.coverArt} size={44} />
          <View className="flex-1">
            <Text numberOfLines={1} className="text-base font-medium text-foreground">
              {hit.album.name}
            </Text>
            <Text numberOfLines={1} className="text-sm text-muted">
              {hit.album.artist ? `${typeLabel} · ${hit.album.artist}` : typeLabel}
            </Text>
          </View>
        </Pressable>
      );
    case 'playlist':
      return (
        <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt">
          <PlaylistCover coverArt={hit.playlist.coverArt} covers={hit.playlist.coverArts ?? []} size={44} fallbackIcon="list" />
          <View className="flex-1">
            <Text numberOfLines={1} className="text-base font-medium text-foreground">
              {hit.playlist.name}
            </Text>
            <Text numberOfLines={1} className="text-sm text-muted">
              {typeLabel} · {t('social.discover.byOwner', { owner: hit.playlist.owner ?? '—', count: formatCount(hit.playlist.songCount) })}
            </Text>
          </View>
        </Pressable>
      );
  }
}
