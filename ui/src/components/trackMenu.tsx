import { useState } from 'react';
import { Modal, Pressable, Text, View } from 'react-native';
import { create } from 'zustand';
import { router } from 'expo-router';
import { Song } from '../api/subsonic/types';
import { CoverArt } from './CoverArt';
import { Ionicon } from './Ionicon';
import { Field } from './ui';
import { usePlayer } from '../audio/store';
import { useAddToPlaylist, useCreatePlaylist, usePlaylists } from '../query/playlists';
import { useAuth } from '../auth/store';
import { useDownloads } from '../offline/store';
import { isSupported as offlineSupported } from '../offline/fs';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/** Global target for the contextual track menu. */
interface TrackMenuState {
  song: Song | null;
  /** Optional "Edit" callback, set per-open (e.g. on the local library screen). */
  onEdit?: (song: Song) => void;
  open: (song: Song, opts?: { onEdit?: (song: Song) => void }) => void;
  close: () => void;
}

export const useTrackMenu = create<TrackMenuState>((set) => ({
  song: null,
  onEdit: undefined,
  open: (song, opts) => set({ song, onEdit: opts?.onEdit }),
  close: () => set({ song: null, onEdit: undefined }),
}));

interface ActionProps {
  icon: string;
  label: string;
  onPress: () => void;
  tone?: 'default' | 'danger';
}

function Action({ icon, label, onPress, tone = 'default' }: ActionProps) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      className="flex-row items-center gap-4 px-5 py-4 active:bg-surface-alt"
    >
      <Ionicon name={icon} size={22} color={tone === 'danger' ? colors.danger : colors.foreground} />
      <Text className={`text-base ${tone === 'danger' ? 'text-danger' : 'text-foreground'}`}>
        {label}
      </Text>
    </Pressable>
  );
}

/**
 * Root-mounted contextual menu for a track. Offers playback queue actions, an
 * "add to playlist" sub-flow, and navigation to the album/artist. Rendered once
 * at the app root; opened from anywhere via `useTrackMenu().open(song)`.
 */
export function TrackMenu() {
  const t = useT();
  const song = useTrackMenu((s) => s.song);
  const onEdit = useTrackMenu((s) => s.onEdit);
  const close = useTrackMenu((s) => s.close);
  const playNext = usePlayer((s) => s.playNext);
  const enqueue = usePlayer((s) => s.enqueue);
  const canOffline = useAuth((s) => s.client?.has('offlineDownloads') ?? false) && offlineSupported;
  const downloaded = useDownloads((s) => (song ? !!s.entries[song.id] : false));
  const downloading = useDownloads((s) => (song ? s.progress[song.id] != null : false));
  const [picker, setPicker] = useState(false);

  if (!song) return null;

  const dismiss = () => {
    setPicker(false);
    close();
  };

  return (
    <Modal transparent animationType="slide" visible onRequestClose={dismiss}>
      <Pressable className="flex-1 justify-end bg-black/50" onPress={dismiss}>
        <Pressable
          className="rounded-t-3xl bg-surface pb-8 pt-2"
          onPress={(e) => e.stopPropagation()}
        >
          <View className="mb-2 items-center pt-1">
            <View className="h-1 w-10 rounded-full bg-border" />
          </View>

          <View className="flex-row items-center gap-3 border-b border-border px-5 pb-3">
            <CoverArt coverArt={song.coverArt} size={48} rounded="rounded-md" />
            <View className="flex-1">
              <Text numberOfLines={1} className="text-base font-semibold text-foreground">
                {song.title}
              </Text>
              <Text numberOfLines={1} className="text-sm text-muted">
                {song.artist}
              </Text>
            </View>
          </View>

          {picker ? (
            <PlaylistPicker song={song} onDone={dismiss} />
          ) : (
            <View>
              <Action
                icon="play"
                label={t('components.trackMenu.playNow')}
                onPress={() => {
                  void usePlayer.getState().playSongs([song], 0);
                  dismiss();
                }}
              />
              <Action
                icon="play-skip-forward"
                label={t('components.trackMenu.playNext')}
                onPress={() => {
                  void playNext([song]);
                  dismiss();
                }}
              />
              <Action
                icon="list"
                label={t('components.trackMenu.addToQueue')}
                onPress={() => {
                  void enqueue([song]);
                  dismiss();
                }}
              />
              <Action icon="add-circle-outline" label={t('components.trackMenu.addToPlaylist')} onPress={() => setPicker(true)} />
              {canOffline ? (
                downloaded ? (
                  <Action
                    icon="cloud-done-outline"
                    label={t('components.trackMenu.removeDownload')}
                    tone="danger"
                    onPress={() => {
                      void useDownloads.getState().remove(song.id);
                      dismiss();
                    }}
                  />
                ) : downloading ? (
                  <Action icon="cloud-download-outline" label={t('components.trackMenu.downloading')} onPress={() => {}} />
                ) : (
                  <Action
                    icon="cloud-download-outline"
                    label={t('components.trackMenu.download')}
                    onPress={() => {
                      void useDownloads.getState().download(song);
                      dismiss();
                    }}
                  />
                )
              ) : null}
              {onEdit ? (
                <Action
                  icon="create-outline"
                  label={t('components.trackMenu.edit')}
                  onPress={() => {
                    const target = song;
                    dismiss();
                    onEdit(target);
                  }}
                />
              ) : null}
              {song.albumId ? (
                <Action
                  icon="disc-outline"
                  label={t('components.trackMenu.goToAlbum')}
                  onPress={() => {
                    dismiss();
                    router.push(`/album/${song.albumId}`);
                  }}
                />
              ) : null}
              {song.artistId ? (
                <Action
                  icon="person-outline"
                  label={t('components.trackMenu.goToArtist')}
                  onPress={() => {
                    dismiss();
                    router.push(`/artist/${song.artistId}`);
                  }}
                />
              ) : null}
            </View>
          )}
        </Pressable>
      </Pressable>
    </Modal>
  );
}

function PlaylistPicker({ song, onDone }: { song: Song; onDone: () => void }) {
  const t = useT();
  const { data: playlists } = usePlaylists();
  const addTo = useAddToPlaylist();
  const createPlaylist = useCreatePlaylist();
  const [newName, setNewName] = useState('');

  const add = (id: string) => {
    addTo.mutate({ id, songIds: [song.id] }, { onSuccess: onDone });
  };

  const createAndAdd = () => {
    if (!newName.trim()) return;
    createPlaylist.mutate(
      { name: newName.trim(), songIds: [song.id] },
      { onSuccess: onDone },
    );
  };

  return (
    <View className="px-5 pt-3">
      <Text className="pb-2 text-sm font-medium text-muted">{t('components.trackMenu.choosePlaylist')}</Text>
      <View className="max-h-64">
        {(playlists ?? []).map((p) => (
          <Pressable
            key={p.id}
            onPress={() => add(p.id)}
            className="flex-row items-center justify-between py-3 active:opacity-60"
          >
            <Text className="text-base text-foreground">{p.name}</Text>
            <Text className="text-xs text-muted">{p.songCount ?? 0}</Text>
          </Pressable>
        ))}
      </View>
      <View className="mt-2 flex-row items-end gap-2">
        <View className="flex-1">
          <Field
            label={t('components.trackMenu.newPlaylist')}
            placeholder={t('components.trackMenu.namePlaceholder')}
            value={newName}
            onChangeText={setNewName}
            onSubmitEditing={createAndAdd}
          />
        </View>
        <Pressable
          onPress={createAndAdd}
          className="mb-0.5 rounded-xl bg-primary px-4 py-3 active:opacity-80"
        >
          <Text className="font-semibold text-primary-foreground">{t('components.trackMenu.create')}</Text>
        </Pressable>
      </View>
    </View>
  );
}
