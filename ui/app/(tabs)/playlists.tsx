import { useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { FlashList } from '@shopify/flash-list';
import { usePlaylists, useCreatePlaylist } from '../../src/query/playlists';
import { useStarred } from '../../src/query/library';
import { useAuth } from '../../src/auth/store';
import { PlaylistCover } from '../../src/components/PlaylistCover';
import { LikedCover } from '../../src/components/LikedCover';
import { LocalCover } from '../../src/components/LocalCover';
import { HallOfFameCover } from '../../src/components/HallOfFameCover';
import { Button, EmptyState, ErrorState, Field, Loading } from '../../src/components/ui';
import { IconButton } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { Playlist } from '../../src/api/subsonic/types';
import { formatCount } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { autoPlaylistName } from '../../src/i18n/autoPlaylists';

/** Pinned virtual playlist of starred songs — always shown first. */
function LikedRow() {
  const t = useT();
  const starred = useStarred();
  const count = starred.data?.song?.length ?? 0;
  return (
    <Pressable
      onPress={() => router.push('/liked' as never)}
      className="flex-row items-center gap-3 border-b border-border px-4 py-2 active:bg-surface-alt"
    >
      <LikedCover size={56} rounded={8} />
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{t('home.playlists.likedTracks')}</Text>
        <Text className="text-sm text-muted">{t('home.playlists.likedSubtitle', { count: formatCount(count) })}</Text>
      </View>
      <IconButton name="chevron-forward" size={18} accessibilityLabel={t('home.playlists.open')} />
    </Pressable>
  );
}

/** Pinned entry to the local (downloaded/offline) songs library. */
function LocalRow() {
  const t = useT();
  return (
    <Pressable
      onPress={() => router.push('/local' as never)}
      className="flex-row items-center gap-3 border-b border-border px-4 py-2 active:bg-surface-alt"
    >
      <LocalCover size={56} rounded={8} />
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{t('components.sidebar.localSongs')}</Text>
        <Text className="text-sm text-muted">{t('components.sidebar.playlist')}</Text>
      </View>
      <IconButton name="chevron-forward" size={18} accessibilityLabel={t('home.playlists.open')} />
    </Pressable>
  );
}

/** Pinned entry to the rule-based smart playlists hub. */
function SmartRow() {
  const t = useT();
  const colors = useColors();
  return (
    <Pressable
      onPress={() => router.push('/smart-playlists' as never)}
      className="flex-row items-center gap-3 border-b border-border px-4 py-2 active:bg-surface-alt"
    >
      <View className="h-14 w-14 items-center justify-center rounded-lg bg-primary/15">
        <Ionicon name="sparkles" size={26} color={colors.primary} />
      </View>
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{t('smart.title')}</Text>
        <Text className="text-sm text-muted">{t('smart.tabSubtitle')}</Text>
      </View>
      <IconButton name="chevron-forward" size={18} accessibilityLabel={t('home.playlists.open')} />
    </Pressable>
  );
}

/** Pinned entry to the personal Hall of Fame. */
function HallOfFameRow() {
  const t = useT();
  return (
    <Pressable
      onPress={() => router.push('/halloffame' as never)}
      className="flex-row items-center gap-3 border-b border-border px-4 py-2 active:bg-surface-alt"
    >
      <HallOfFameCover size={56} rounded={8} />
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{t('components.sidebar.hallOfFame')}</Text>
        <Text className="text-sm text-muted">{t('components.sidebar.playlist')}</Text>
      </View>
      <IconButton name="chevron-forward" size={18} accessibilityLabel={t('home.playlists.open')} />
    </Pressable>
  );
}

/** Pinned entry to internet radio. */
function RadioRow() {
  const t = useT();
  const colors = useColors();
  return (
    <Pressable
      onPress={() => router.push('/radios' as never)}
      className="flex-row items-center gap-3 border-b border-border px-4 py-2 active:bg-surface-alt"
    >
      <View className="h-14 w-14 items-center justify-center rounded-lg bg-primary/15">
        <Ionicon name="radio" size={26} color={colors.primary} />
      </View>
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{t('radio.title')}</Text>
        <Text className="text-sm text-muted">{t('radio.tabSubtitle')}</Text>
      </View>
      <IconButton name="chevron-forward" size={18} accessibilityLabel={t('home.playlists.open')} />
    </Pressable>
  );
}

/** Playlists hub: list, create, and open. CRUD detail lives in /playlist/[id]. */
export default function Playlists() {
  const t = useT();
  const colors = useColors();
  const q = usePlaylists();
  const create = useCreatePlaylist();
  const canDiscover = useAuth((s) => s.client?.has('publicPlaylists') ?? false);
  const canSmart = useAuth((s) => s.client?.isFeatureEnabled('smartPlaylists') ?? false);
  const canRadio = useAuth((s) => s.client?.isFeatureEnabled('internetRadio') ?? false);
  const canHallOfFame = useAuth((s) => s.client?.isFeatureEnabled('hallOfFame') ?? false);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');

  const submit = () => {
    if (!name.trim()) return;
    create.mutate(
      { name: name.trim() },
      {
        onSuccess: () => {
          setName('');
          setCreating(false);
        },
      },
    );
  };

  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      <View className="flex-row items-center justify-between px-4 pb-1 pt-3">
        <Text className="text-3xl font-bold text-foreground">{t('home.playlists.title')}</Text>
        <View className="flex-row items-center gap-3">
          {canDiscover ? (
            <IconButton
              name="compass-outline"
              size={26}
              color={colors.primary}
              onPress={() => router.push('/discover' as never)}
              accessibilityLabel={t('home.playlists.publicPlaylists')}
            />
          ) : null}
          <IconButton
            name={creating ? 'close' : 'add'}
            size={28}
            color={colors.primary}
            onPress={() => setCreating((c) => !c)}
            accessibilityLabel={t('home.playlists.newPlaylist')}
          />
        </View>
      </View>

      {creating ? (
        <View className="gap-2 px-4 py-2">
          <Field
            placeholder={t('home.playlists.namePlaceholder')}
            value={name}
            onChangeText={setName}
            autoFocus
            onSubmitEditing={submit}
          />
          <Button title={t('home.playlists.create')} icon="add" loading={create.isPending} onPress={submit} />
        </View>
      ) : null}

      <FlashList<Playlist>
        data={q.data ?? []}
        keyExtractor={(p) => p.id}
        estimatedItemSize={72}
        refreshing={q.isRefetching}
        onRefresh={q.refetch}
        ListHeaderComponent={
          <>
            <LikedRow />
            <LocalRow />
            {canHallOfFame ? <HallOfFameRow /> : null}
            {canSmart ? <SmartRow /> : null}
            {canRadio ? <RadioRow /> : null}
          </>
        }
        ListEmptyComponent={
          q.isLoading ? (
            <Loading />
          ) : q.isError ? (
            <ErrorState message={t('home.playlists.loadError')} onRetry={q.refetch} />
          ) : (
            <EmptyState icon="list" title={t('home.playlists.empty')} subtitle={t('home.playlists.emptySubtitle')} />
          )
        }
        renderItem={({ item }) => (
          <Pressable
            onPress={() => router.push(`/playlist/${item.id}`)}
            className="flex-row items-center gap-3 px-4 py-2 active:bg-surface-alt"
          >
            <PlaylistCover coverArt={item.coverArt} covers={item.coverArts ?? []} size={56} rounded="rounded-lg" fallbackIcon="list" />
            <View className="flex-1">
              <Text numberOfLines={1} className="text-base font-semibold text-foreground">
                {autoPlaylistName(t, item.autoPlaylistKind, item.name)}
              </Text>
              <Text className="text-sm text-muted">{t('home.playlists.trackCount', { count: formatCount(item.songCount) })}</Text>
            </View>
          </Pressable>
        )}
        contentContainerStyle={{ paddingBottom: 16 }}
      />
    </SafeAreaView>
  );
}
