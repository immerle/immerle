import { useEffect } from 'react';
import { Pressable, ScrollView, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useDebounced, useSearch } from '../query/search';
import { searchNav, useSearchUI } from '../search/store';
import { CoverArt } from './CoverArt';
import { AlbumTile } from './AlbumCard';
import { TrackRow } from './TrackRow';
import { EmptyState, Loading } from './ui';
import { Ionicon } from './Ionicon';
import { usePlayer } from '../audio/store';
import { useTrackMenu } from './trackMenu';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/**
 * Live search results, shared by the web header popover and the mobile
 * full-screen overlay. When the query is empty it shows recent searches.
 *
 * Artists and songs form a single vertical, keyboard-navigable list (↑/↓/Enter,
 * driven by `SearchOverlay`); albums are a mouse/touch carousel at the bottom.
 * Selecting anything records the query as recent and dismisses the search.
 */
export function SearchResults({ onClose }: { onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const query = useSearchUI((s) => s.query);
  const recents = useSearchUI((s) => s.recents);
  const setQuery = useSearchUI((s) => s.setQuery);
  const addRecent = useSearchUI((s) => s.addRecent);
  const activeIndex = useSearchUI((s) => s.activeIndex);
  const debounced = useDebounced(query, 250);
  const { data, isLoading, isFetching } = useSearch(debounced);
  const playSongs = usePlayer((s) => s.playSongs);
  const openMenu = useTrackMenu((s) => s.open);

  const trimmed = debounced.trim();
  const songs = data?.song ?? [];
  const albums = data?.album ?? [];
  const artists = (data?.artist ?? []).slice(0, 5);

  const select = (action: () => void) => {
    addRecent(trimmed);
    action();
    onClose();
  };

  // Flat keyboard list = artists then songs (both vertical rows).
  const flat = [
    ...artists.map((a) => () => select(() => router.push(`/artist/${a.id}` as never))),
    ...songs.map((s, i) => () => select(() => playSongs(songs, i))),
  ];

  // Publish the flat list for SearchOverlay's keyboard handler.
  useEffect(() => {
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
  const empty = !isFetching && !songs.length && !albums.length && !artists.length;
  if (empty) {
    return <EmptyState icon="sad-outline" title={t('components.search.noResults')} subtitle={t('components.search.nothingFor', { query: trimmed })} />;
  }

  return (
    <ScrollView keyboardShouldPersistTaps="handled" contentContainerStyle={{ paddingBottom: 16 }}>
      {artists.length > 0 ? (
        <Section title={t('components.search.artists')}>
          {artists.map((a, i) => (
            <Pressable
              key={a.id}
              onPress={() => select(() => router.push(`/artist/${a.id}` as never))}
              className={`flex-row items-center gap-3 px-4 py-2 ${activeIndex === i ? 'bg-surface-alt' : ''} active:bg-surface-alt`}
            >
              <CoverArt coverArt={a.coverArt} url={a.artistImageUrl} size={44} rounded="rounded-full" fallbackIcon="person" />
              <Text numberOfLines={1} className="flex-1 text-base font-medium text-foreground">
                {a.name}
              </Text>
            </Pressable>
          ))}
        </Section>
      ) : null}

      {songs.length > 0 ? (
        <Section title={t('components.search.songs')}>
          {songs.map((s, i) => {
            const flatIndex = artists.length + i;
            return (
              <View key={`${s.id}:${i}`} className={activeIndex === flatIndex ? 'bg-surface-alt' : ''}>
                <TrackRow
                  song={s}
                  onPress={() => select(() => playSongs(songs, i))}
                  onMore={() => openMenu(s)}
                />
              </View>
            );
          })}
        </Section>
      ) : null}

      {albums.length > 0 ? (
        <Section title={t('components.search.albums')}>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ paddingHorizontal: 16 }}>
            {albums.map((a) => (
              <AlbumTile key={a.id} album={a} size={120} />
            ))}
          </ScrollView>
        </Section>
      ) : null}
    </ScrollView>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <View className="pb-1">
      <Text className="px-4 pb-1 pt-3 text-base font-bold text-foreground">{title}</Text>
      {children}
    </View>
  );
}
