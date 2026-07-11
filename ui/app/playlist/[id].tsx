import { useEffect, useState } from 'react';
import { Alert, Modal, Platform, Pressable, Switch, Text, View } from 'react-native';
import * as ImagePicker from 'expo-image-picker';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import {
  useDeletePlaylist,
  usePlaylist,
  useRenamePlaylist,
  useReorderPlaylist,
  useSetPlaylistCover,
  useSetPlaylistPublic,
  useSubscriptionMutations,
} from '../../src/query/playlists';
import { PlaylistCover } from '../../src/components/PlaylistCover';
import { TrackList } from '../../src/components/TrackList';
import { Badge, Button, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { DownloadButton } from '../../src/components/DownloadButton';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { useAddCollaborator } from '../../src/query/social';
import { Song } from '../../src/api/subsonic/types';
import { formatDuration } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/**
 * Playlist detail with full CRUD. View mode plays the playlist; edit mode lets
 * the user rename, reorder (up/down — works identically on web and native), and
 * remove tracks, then persists the new order in one rewrite. Delete removes the
 * whole playlist.
 */
export default function PlaylistDetail() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = usePlaylist(id);
  const rename = useRenamePlaylist();
  const reorder = useReorderPlaylist();
  const del = useDeletePlaylist();
  const playSongs = usePlayer((s) => s.playSongs);

  const client = useAuth((s) => s.client);
  const username = useAuth((s) => s.client?.username);
  const canCollaborate = useAuth((s) => s.client?.has('collaborativePlaylists') ?? false);
  const canPublic = useAuth((s) => s.client?.has('publicPlaylists') ?? false);
  const addCollaborator = useAddCollaborator();
  const setPublic = useSetPlaylistPublic();
  const setCover = useSetPlaylistCover();
  const { subscribe, unsubscribe } = useSubscriptionMutations();

  const [coverOpen, setCoverOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [name, setName] = useState('');
  const [ordered, setOrdered] = useState<Song[]>([]);
  const [collaborator, setCollaborator] = useState('');
  const [isPublic, setIsPublic] = useState(false);

  useEffect(() => {
    if (q.data) {
      setName(q.data.name);
      setOrdered(q.data.entry ?? []);
      setIsPublic(!!q.data.public);
    }
  }, [q.data]);
  useWebTitle(q.data?.name);

  if (q.isLoading) return <Loading />;
  if (q.isError || !q.data) return <ErrorState message={t('media.playlist.notFound')} onRetry={q.refetch} />;

  const playlist = q.data;
  const songs = playlist.entry ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);
  // Subsonic omits `owner` for one's own playlists on some servers; treat a
  // missing owner as owned. A non-owner here is a subscriber → read-only. A
  // federated playlist is never "owned" — its owner field is just an internal
  // attribution the server had to pick — so it's always read-only/subscribable,
  // even for the account that field happens to name.
  const isOwner = !playlist.federated && (playlist.owner ? playlist.owner === username : true);

  const togglePublic = (next: boolean) => {
    setIsPublic(next);
    setPublic.mutate({ id, isPublic: next }, { onError: () => setIsPublic(!next) });
  };

  // A federated-playlist track with no local match yet: resolve it now (local
  // catalog, then providers) and play the result in place, keeping the rest of
  // the queue intact.
  // ponytail: only covers tapping a row or "play all" from track 0 — skipping
  // to a later unresolved track mid-queue during playback still won't
  // resolve; add if that turns out to matter in practice.
  const playUnresolved = async (_song: Song, index: number) => {
    if (!client) return;
    try {
      const resolved = await client.resolvePlaylistTrack(id, index);
      playSongs([...songs.slice(0, index), resolved, ...songs.slice(index + 1)], index);
    } catch {
      Alert.alert(t('media.playlist.resolveErrorTitle'), t('media.playlist.resolveErrorMessage'));
    }
  };

  const pickPhoto = async () => {
    if (Platform.OS !== 'web') {
      const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!perm.granted) return;
    }
    const res = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], quality: 0.9 });
    if (res.canceled || !res.assets?.length) return;
    const asset = res.assets[0];
    setCover.mutate(
      { id, uri: asset.uri, mime: asset.mimeType ?? 'image/jpeg' },
      { onSuccess: () => setCoverOpen(false) },
    );
  };

  const confirmUnsubscribe = () => {
    const doIt = () => unsubscribe.mutate(id, { onSuccess: () => router.back() });
    if (Platform.OS === 'web') doIt();
    else
      Alert.alert(t('media.playlist.unsubscribeTitle'), t('media.playlist.unsubscribeMessage', { name: playlist.name }), [
        { text: t('media.playlist.cancel'), style: 'cancel' },
        { text: t('media.playlist.unsubscribeConfirm'), style: 'destructive', onPress: doIt },
      ]);
  };

  const move = (from: number, to: number) => {
    if (to < 0 || to >= ordered.length) return;
    const next = [...ordered];
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item);
    setOrdered(next);
  };

  const removeAt = (index: number) => {
    setOrdered((prev) => prev.filter((_, i) => i !== index));
  };

  const save = async () => {
    if (name.trim() && name.trim() !== playlist.name) {
      await rename.mutateAsync({ id, name: name.trim() });
    }
    const changed =
      ordered.length !== songs.length ||
      ordered.some((s, i) => s.id !== songs[i]?.id);
    if (changed) {
      await reorder.mutateAsync({ id, ordered });
    }
    setEditing(false);
  };

  const confirmDelete = () => {
    const doDelete = () => del.mutate(id, { onSuccess: () => router.back() });
    if (Platform.OS === 'web') {
      doDelete();
    } else {
      Alert.alert(t('media.playlist.deleteTitle'), t('media.playlist.deleteMessage', { name: playlist.name }), [
        { text: t('media.playlist.cancel'), style: 'cancel' },
        { text: t('media.playlist.deleteConfirm'), style: 'destructive', onPress: doDelete },
      ]);
    }
  };

  const Header = (
    <View className="w-full max-w-2xl items-center gap-3 self-center px-4 pb-4 pt-2">
      <Pressable onPress={isOwner ? () => setCoverOpen(true) : undefined} disabled={!isOwner}>
        <PlaylistCover
          coverArt={playlist.coverArt}
          covers={songs.slice(0, 4).map((s) => s.coverArt)}
          size={180}
          rounded="rounded-xl"
        />
      </Pressable>
      {editing ? (
        <View className="w-full">
          <Field value={name} onChangeText={setName} placeholder={t('media.playlist.namePlaceholder')} />
        </View>
      ) : (
        <Text className="text-center text-2xl font-bold text-foreground">{playlist.name}</Text>
      )}
      <Text className="text-xs text-muted">
        {t('media.playlist.trackCount', { count: songs.length })} · {formatDuration(totalDuration)}
      </Text>
      <View className="flex-row gap-2">
        {!isOwner && playlist.subscribed ? <Badge label={t('media.playlist.subscriptionBadge')} /> : null}
        {playlist.public ? <Badge label={t('media.playlist.publicBadge')} tone="primary" /> : null}
      </View>
      {!editing ? (
        <View className="w-full flex-row items-center justify-between pt-1">
          {isOwner ? (
            <IconButton name="create-outline" size={24} color={colors.muted} onPress={() => setEditing(true)} accessibilityLabel={t('media.playlist.edit')} />
          ) : playlist.subscribed ? (
            <IconButton name="heart-dislike-outline" size={24} color={colors.danger} onPress={confirmUnsubscribe} accessibilityLabel={t('media.playlist.unsubscribe')} />
          ) : (
            <IconButton
              name="heart-outline"
              size={24}
              color={colors.primary}
              disabled={subscribe.isPending}
              onPress={() => subscribe.mutate(id)}
              accessibilityLabel={t('media.playlist.subscribe')}
            />
          )}
          <View className="flex-row items-center gap-4">
            <DownloadButton songs={songs} size={24} />
            <PlayButton
              onPress={() => {
                if (!songs.length) return;
                if (songs[0].unresolved) void playUnresolved(songs[0], 0);
                else playSongs(songs, 0);
              }}
              size={56}
              accessibilityLabel={t('media.playlist.play')}
            />
          </View>
        </View>
      ) : (
        <View className="w-full gap-2 pt-1">
          {canPublic ? (
            <View className="flex-row items-center justify-between rounded-xl bg-surface px-3 py-2">
              <View className="flex-1">
                <Text className="text-base text-foreground">{t('media.playlist.publicToggle')}</Text>
                <Text className="text-xs text-muted">{t('media.playlist.publicHint')}</Text>
              </View>
              <Switch value={isPublic} onValueChange={togglePublic} trackColor={{ true: colors.primary, false: colors.border }} />
            </View>
          ) : null}
          {canCollaborate ? (
            <View className="flex-row items-end gap-2">
              <View className="flex-1">
                <Field
                  label={t('media.playlist.collaborator')}
                  placeholder={t('media.playlist.usernamePlaceholder')}
                  autoCapitalize="none"
                  value={collaborator}
                  onChangeText={setCollaborator}
                />
              </View>
              <Button
                title={t('media.playlist.add')}
                loading={addCollaborator.isPending}
                onPress={() =>
                  collaborator.trim() &&
                  addCollaborator.mutate(
                    { playlistId: id, username: collaborator.trim() },
                    { onSuccess: () => setCollaborator('') },
                  )
                }
              />
            </View>
          ) : null}
          <Button title={t('media.playlist.save')} icon="checkmark" loading={reorder.isPending || rename.isPending} onPress={save} />
          <Button title={t('media.playlist.delete')} icon="trash-outline" variant="danger" onPress={confirmDelete} />
        </View>
      )}
    </View>
  );

  return (
    <>
      <Stack.Screen
        options={{
          title: playlist.name,
          headerRight: () =>
            editing ? (
              <IconButton name="close" color={colors.primary} onPress={() => setEditing(false)} />
            ) : null,
        }}
      />
      <View className="flex-1 bg-background">
        {editing ? (
          <FlashList<Song>
            data={ordered}
            keyExtractor={(s, i) => `${s.id}:${i}`}
            estimatedItemSize={64}
            ListHeaderComponent={Header}
            renderItem={({ item, index }) => (
              <View className="flex-row items-center gap-2 px-3 py-2">
                <View className="flex-1">
                  <Text numberOfLines={1} className="text-base text-foreground">
                    {item.title}
                  </Text>
                  <Text numberOfLines={1} className="text-sm text-muted">
                    {item.artist}
                  </Text>
                </View>
                <IconButton name="chevron-up" size={22} color={colors.muted} onPress={() => move(index, index - 1)} />
                <IconButton name="chevron-down" size={22} color={colors.muted} onPress={() => move(index, index + 1)} />
                <IconButton name="remove-circle" size={22} color={colors.danger} onPress={() => removeAt(index)} />
              </View>
            )}
            contentContainerStyle={{ paddingBottom: 24 }}
          />
        ) : (
          <TrackList
            songs={songs}
            header={Header}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            onPlayUnresolved={playUnresolved}
          />
        )}
      </View>

      <Modal visible={coverOpen} transparent animationType="fade" onRequestClose={() => setCoverOpen(false)}>
        <View className="flex-1 items-center justify-center bg-black/90 px-6">
          <View className="absolute right-4 top-12">
            <IconButton name="close" size={32} color="#fff" onPress={() => setCoverOpen(false)} accessibilityLabel={t('media.playlist.cover.close')} />
          </View>
          <PlaylistCover
            coverArt={playlist.coverArt}
            covers={songs.slice(0, 4).map((s) => s.coverArt)}
            size={280}
            rounded="rounded-2xl"
          />
          <View className="mt-8 w-full max-w-sm gap-3">
            <Button
              title={t('media.playlist.cover.create')}
              icon="color-palette-outline"
              onPress={() => {
                setCoverOpen(false);
                router.push(`/playlist/cover/${id}`);
              }}
            />
            <Button
              title={t('media.playlist.cover.pick')}
              icon="image-outline"
              variant="secondary"
              loading={setCover.isPending}
              onPress={pickPhoto}
            />
          </View>
        </View>
      </Modal>
    </>
  );
}
