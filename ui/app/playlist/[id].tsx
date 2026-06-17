import { useEffect, useState } from 'react';
import { Alert, Platform, Switch, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import {
  useDeletePlaylist,
  usePlaylist,
  useRenamePlaylist,
  useReorderPlaylist,
  useSetPlaylistPublic,
  useSubscriptionMutations,
} from '../../src/query/playlists';
import { PlaylistMosaic } from '../../src/components/PlaylistMosaic';
import { TrackList } from '../../src/components/TrackList';
import { Badge, Button, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { PlayButton } from '../../src/components/PlayButton';
import { Ionicon } from '../../src/components/Ionicon';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { useAddCollaborator } from '../../src/query/social';
import { Song } from '../../src/api/subsonic/types';
import { formatDuration } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';

/**
 * Playlist detail with full CRUD. View mode plays the playlist; edit mode lets
 * the user rename, reorder (up/down — works identically on web and native), and
 * remove tracks, then persists the new order in one rewrite. Delete removes the
 * whole playlist.
 */
export default function PlaylistDetail() {
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = usePlaylist(id);
  const rename = useRenamePlaylist();
  const reorder = useReorderPlaylist();
  const del = useDeletePlaylist();
  const playSongs = usePlayer((s) => s.playSongs);

  const username = useAuth((s) => s.client?.username);
  const canCollaborate = useAuth((s) => s.client?.has('collaborativePlaylists') ?? false);
  const canPublic = useAuth((s) => s.client?.has('publicPlaylists') ?? false);
  const addCollaborator = useAddCollaborator();
  const setPublic = useSetPlaylistPublic();
  const { unsubscribe } = useSubscriptionMutations();

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

  if (q.isLoading) return <Loading />;
  if (q.isError || !q.data) return <ErrorState message="Playlist introuvable." onRetry={q.refetch} />;

  const playlist = q.data;
  const songs = playlist.entry ?? [];
  const totalDuration = songs.reduce((n, s) => n + (s.duration ?? 0), 0);
  // Subsonic omits `owner` for one's own playlists on some servers; treat a
  // missing owner as owned. A non-owner here is a subscriber → read-only.
  const isOwner = playlist.owner ? playlist.owner === username : true;

  const togglePublic = (next: boolean) => {
    setIsPublic(next);
    setPublic.mutate({ id, isPublic: next }, { onError: () => setIsPublic(!next) });
  };

  const confirmUnsubscribe = () => {
    const doIt = () => unsubscribe.mutate(id, { onSuccess: () => router.back() });
    if (Platform.OS === 'web') doIt();
    else
      Alert.alert('Se désabonner ?', `« ${playlist.name} » sera retirée de votre bibliothèque.`, [
        { text: 'Annuler', style: 'cancel' },
        { text: 'Se désabonner', style: 'destructive', onPress: doIt },
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
      Alert.alert('Supprimer la playlist ?', `« ${playlist.name} » sera supprimée.`, [
        { text: 'Annuler', style: 'cancel' },
        { text: 'Supprimer', style: 'destructive', onPress: doDelete },
      ]);
    }
  };

  const Header = (
    <View className="w-full max-w-2xl items-center gap-3 self-center px-4 pb-4 pt-2">
      <PlaylistMosaic covers={songs.slice(0, 4).map((s) => s.coverArt)} size={180} fallbackIcon="list" />
      {editing ? (
        <View className="w-full">
          <Field value={name} onChangeText={setName} placeholder="Nom de la playlist" />
        </View>
      ) : (
        <Text className="text-center text-2xl font-bold text-foreground">{playlist.name}</Text>
      )}
      <Text className="text-xs text-muted">
        {songs.length} titres · {formatDuration(totalDuration)}
      </Text>
      <View className="flex-row gap-2">
        {!isOwner ? <Badge label="Abonnement" /> : null}
        {playlist.public ? <Badge label="Publique" tone="primary" /> : null}
      </View>
      {!editing ? (
        <View className="w-full flex-row items-center justify-between pt-1">
          {isOwner ? (
            <IconButton name="create-outline" size={24} color={colors.muted} onPress={() => setEditing(true)} accessibilityLabel="Modifier" />
          ) : (
            <IconButton name="heart-dislike-outline" size={24} color={colors.danger} onPress={confirmUnsubscribe} accessibilityLabel="Se désabonner" />
          )}
          <PlayButton onPress={() => songs.length && playSongs(songs, 0)} size={56} accessibilityLabel="Lecture de la playlist" />
        </View>
      ) : (
        <View className="w-full gap-2 pt-1">
          {canPublic ? (
            <View className="flex-row items-center justify-between rounded-xl bg-surface px-3 py-2">
              <View className="flex-1">
                <Text className="text-base text-foreground">Playlist publique</Text>
                <Text className="text-xs text-muted">Visible dans « Playlists publiques » ; abonnement opt-in.</Text>
              </View>
              <Switch value={isPublic} onValueChange={togglePublic} trackColor={{ true: colors.primary, false: colors.border }} />
            </View>
          ) : null}
          {canCollaborate ? (
            <View className="flex-row items-end gap-2">
              <View className="flex-1">
                <Field
                  label="Collaborateur"
                  placeholder="Nom d'utilisateur"
                  autoCapitalize="none"
                  value={collaborator}
                  onChangeText={setCollaborator}
                />
              </View>
              <Button
                title="Ajouter"
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
          <Button title="Enregistrer" icon="checkmark" loading={reorder.isPending || rename.isPending} onPress={save} />
          <Button title="Supprimer la playlist" icon="trash-outline" variant="danger" onPress={confirmDelete} />
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
          <TrackList songs={songs} header={Header} refreshing={q.isRefetching} onRefresh={q.refetch} />
        )}
      </View>
    </>
  );
}
