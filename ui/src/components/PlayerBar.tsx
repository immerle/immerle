import { useRef, useState } from 'react';
import { ActivityIndicator, Modal, Pressable, ScrollView, Text, useWindowDimensions, View } from 'react-native';
import { router, usePathname } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import Slider from '@react-native-community/slider';
import { CoverArt } from './CoverArt';
import { Button, Field, IconButton } from './ui';
import { PlayButton } from './PlayButton';
import { Ionicon } from './Ionicon';
import { usePlayer } from '../audio/store';
import { useAuth } from '../auth/store';
import {
  useAddToPlaylist,
  useCreatePlaylist,
  usePlaylist,
  usePlaylists,
  useRemoveFromPlaylist,
} from '../query/playlists';
import { usePlaybackTargets } from '../query/account';
import { useRadioStations, useRadioLike } from '../query/radio';
import { RadioStation } from '../api/immerle/types';
import { queryClient } from '../query/queryClient';
import { qk } from '../query/keys';
import { Song } from '../api/subsonic/types';
import { formatDuration } from '../utils/format';
import { useColors } from '../theme/colors';
import { WIDE_BREAKPOINT } from '../theme/layout';
import { useT } from '../i18n/store';

// Routes where the docked bar should not appear (its own full-screen surfaces,
// and pre-auth screens).
const HIDDEN_ON = ['/player', '/queue', '/login', '/setup'];

// Main tab routes. On mobile these render their own player *above* the tab bar
// (see the tabs layout), so the root-docked bar yields to that embedded copy.
export const TAB_ROUTES = ['/', '/search', '/playlists', '/social', '/admin', '/settings'];

/**
 * Persistent, full-featured player docked at the bottom of every screen while
 * something is loaded. On wide layouts (web/tablet) it's a three-column Spotify-
 * style bar with inline transport, a seek bar, volume and queue controls; on
 * narrow layouts it collapses to a compact bar that opens the full-screen
 * player on tap.
 */
export function PlayerBar({ embedded = false }: { embedded?: boolean } = {}) {
  const insets = useSafeAreaInsets();
  const { width } = useWindowDimensions();
  const pathname = usePathname();

  const song = usePlayer((s) => (s.index >= 0 ? s.songs[s.index] : undefined));
  const status = usePlayer((s) => s.status);
  const position = usePlayer((s) => s.position);
  const duration = usePlayer((s) => s.duration);

  if (HIDDEN_ON.includes(pathname)) return null;
  const wide = width >= WIDE_BREAKPOINT;
  // On mobile tab screens the tabs layout renders an embedded player above the
  // tab bar; the root-docked copy steps aside so there's only one.
  if (!embedded && !wide && TAB_ROUTES.includes(pathname)) return null;
  // On wide layouts the bottom strip is permanently the player (Spotify-style),
  // even when idle; on narrow it only appears while something is loaded.
  if (!song && !wide) return null;

  return (
    <View
      className="border-t border-border bg-surface"
      // Embedded copy sits above the tab bar, which already owns the bottom inset.
      style={{ paddingBottom: embedded ? 0 : insets.bottom }}
    >
      {!wide ? (
        <CompactBar song={song!} status={status} position={position} duration={duration} />
      ) : song ? (
        <WideBar song={song} status={status} position={position} duration={duration} />
      ) : (
        <IdleBar />
      )}
    </View>
  );
}

/** Slim placeholder shown on wide layouts when nothing is playing. */
function IdleBar() {
  const t = useT();
  const colors = useColors();
  return (
    <View className="flex-row items-center gap-4 px-4 py-3">
      <View className="min-w-0 flex-1 flex-row items-center gap-3">
        <View className="h-14 w-14 items-center justify-center rounded-md bg-surface-alt">
          <Ionicon name="musical-notes" size={22} color={colors.muted} />
        </View>
        <Text className="text-sm text-muted">{t('components.player.nothingPlaying')}</Text>
      </View>
      <View className="flex-[1.5] items-center">
        <PlayButton size={40} />
      </View>
      <View className="flex-1 flex-row items-center justify-end gap-3">
        <VolumeControl />
      </View>
    </View>
  );
}

interface BarProps {
  song: Song;
  status: string;
  position: number;
  duration: number;
}

function CompactBar({ song, status, position, duration }: BarProps) {
  const t = useT();
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  const progress = duration > 0 ? Math.min(position / duration, 1) : 0;
  const isPlaying = status === 'playing';
  const isRadio = useIsRadio(song.id);

  return (
    <View>
      <Pressable
        onPress={() => router.push('/player')}
        className="flex-row items-center gap-3 px-3 py-2 active:opacity-90"
      >
        <CoverArt coverArt={song.coverArt} url={song.coverUrl} size={44} rounded="rounded-md" />
        <View className="flex-1">
          <Text numberOfLines={1} className="text-sm font-semibold text-foreground">
            {song.title}
          </Text>
          <Text numberOfLines={1} className="text-xs text-muted">
            {song.artist}
          </Text>
        </View>
        <IconButton name={isPlaying ? 'pause' : 'play'} size={26} onPress={toggle} accessibilityLabel={isPlaying ? t('components.player.pause') : t('components.player.play')} />
        <IconButton name="play-skip-forward" size={22} onPress={next} disabled={isRadio} accessibilityLabel={t('components.player.next')} />
      </Pressable>
      <View className="mx-2 mb-1 h-1 overflow-hidden rounded-full bg-white/15">
        <View className="h-full rounded-full bg-foreground" style={{ width: `${progress * 100}%` }} />
      </View>
    </View>
  );
}

function WideBar({ song, status, position, duration }: BarProps) {
  const t = useT();
  const colors = useColors();
  const toggle = usePlayer((s) => s.toggle);
  const next = usePlayer((s) => s.next);
  const previous = usePlayer((s) => s.previous);
  const seekTo = usePlayer((s) => s.seekTo);
  const repeat = usePlayer((s) => s.repeat);
  const cycleRepeat = usePlayer((s) => s.cycleRepeat);
  const shuffle = usePlayer((s) => s.shuffle);
  const toggleShuffle = usePlayer((s) => s.toggleShuffle);

  const isPlaying = status === 'playing';
  const [scrub, setScrub] = useState<number | null>(null);
  const shown = scrub ?? position;
  // A live radio has no queue, no seeking and no prev/next: grey those out.
  const isRadio = useIsRadio(song.id);
  // Another device has claimed active playback (see CastButton) — this one
  // just mirrors state, so its transport controls are inert until reclaimed.
  const myId = useAuth((s) => s.client?.getSession()?.deviceId);
  const castTargetId = usePlayer((s) => s.castTargetId);
  const remoteControlled = !!castTargetId && castTargetId !== myId;
  const transportDisabled = isRadio || remoteControlled;
  // A not-yet-downloaded track streams progressively — the server can't serve
  // byte ranges for it yet. The bar stays interactive though: dragging it
  // triggers usePlayer.seekTo's check-and-upgrade-or-toast flow, which seeks
  // for real once the background download has finished.
  const seekDisabled = transportDisabled;

  return (
    <View className="flex-row items-center gap-4 px-4 py-2">
      {/* Left — now playing */}
      <View className="min-w-0 flex-1 flex-row items-center gap-3">
        <Pressable onPress={() => router.push('/player')} className="active:opacity-80">
          <CoverArt coverArt={song.coverArt} url={song.coverUrl} size={56} rounded="rounded-md" />
        </Pressable>
        <View className="min-w-0 flex-1">
          <Text numberOfLines={1} className="text-sm font-semibold text-foreground">
            {song.title}
          </Text>
          <Text numberOfLines={1} className="text-xs text-muted">
            {remoteControlled ? t('components.player.castPlayingElsewhere') : song.artist}
          </Text>
        </View>
        <LikeButton key={song.id} song={song} />
        <AddToPlaylistButton song={song} disabled={isRadio} />
      </View>

      {/* Center — transport + seek */}
      <View className="flex-[1.5] items-center gap-1">
        <View className="flex-row items-center gap-5">
          <IconButton
            name="shuffle"
            size={20}
            color={shuffle ? colors.primary : colors.muted}
            onPress={toggleShuffle}
            disabled={transportDisabled}
            accessibilityLabel={t('components.player.shuffle')}
          />
          <IconButton name="play-skip-back" size={22} onPress={previous} disabled={transportDisabled} accessibilityLabel={t('components.player.previous')} />
          <PlayButton playing={isPlaying} onPress={toggle} size={40} disabled={transportDisabled} />
          <IconButton name="play-skip-forward" size={22} onPress={next} disabled={transportDisabled} accessibilityLabel={t('components.player.next')} />
          <View>
            <IconButton
              name={repeat === 'track' ? 'repeat-outline' : 'repeat'}
              size={20}
              color={repeat !== 'off' ? colors.primary : colors.muted}
              onPress={cycleRepeat}
              disabled={transportDisabled}
              accessibilityLabel={t('components.player.repeat')}
            />
            {repeat === 'track' ? (
              <Text className="absolute -right-1 -top-1 text-[9px] font-bold text-primary">1</Text>
            ) : null}
          </View>
        </View>
        <View className="w-full max-w-xl flex-row items-center gap-2">
          <Text className="w-10 text-right text-[11px] text-muted">{formatDuration(shown)}</Text>
          <Slider
            style={{ flex: 1, opacity: seekDisabled ? 0.4 : song.remote ? 0.7 : 1 }}
            disabled={seekDisabled}
            minimumValue={0}
            maximumValue={duration > 0 ? duration : 1}
            value={shown}
            minimumTrackTintColor={colors.primary}
            maximumTrackTintColor={colors.border}
            thumbTintColor={colors.primary}
            onValueChange={setScrub}
            onSlidingComplete={(v) => {
              setScrub(null);
              void seekTo(v);
            }}
          />
          <Text className="w-10 text-[11px] text-muted">{formatDuration(duration)}</Text>
        </View>
      </View>

      {/* Right — queue + cast + volume + fullscreen */}
      <View className="flex-1 flex-row items-center justify-end gap-3">
        <IconButton name="list" size={22} onPress={() => router.push('/queue')} disabled={isRadio} accessibilityLabel={t('components.player.queue')} />
        <CastButton active={remoteControlled} disabled={isRadio} />
        <VolumeControl />
        <IconButton name="expand" size={20} onPress={() => router.push('/player')} accessibilityLabel={t('components.player.fullscreen')} />
      </View>
    </View>
  );
}

function VolumeControl() {
  const t = useT();
  const colors = useColors();
  const volume = usePlayer((s) => s.volume);
  const setVolume = usePlayer((s) => s.setVolume);
  const [lastNonZero, setLastNonZero] = useState(1);

  const icon = volume === 0 ? 'volume-mute' : volume < 0.5 ? 'volume-low' : 'volume-high';
  const toggleMute = () => {
    if (volume === 0) setVolume(lastNonZero || 1);
    else {
      setLastNonZero(volume);
      setVolume(0);
    }
  };

  return (
    <View className="flex-row items-center gap-2">
      <IconButton name={icon} size={20} color={colors.muted} onPress={toggleMute} accessibilityLabel={t('components.player.volume')} />
      <VolumeBar volume={volume} onChange={setVolume} />
    </View>
  );
}

/** Thin volume bar with no thumb — click or drag anywhere to set the level. */
function VolumeBar({ volume, onChange }: { volume: number; onChange: (v: number) => void }) {
  const t = useT();
  const colors = useColors();
  const [width, setWidth] = useState(0);
  const apply = (x: number) => {
    if (width > 0) onChange(Math.max(0, Math.min(1, x / width)));
  };
  return (
    <View
      onLayout={(e) => setWidth(e.nativeEvent.layout.width)}
      style={{ width: 96, height: 16, justifyContent: 'center' }}
      accessibilityRole="adjustable"
      accessibilityLabel={t('components.player.volume')}
      onStartShouldSetResponder={() => true}
      onMoveShouldSetResponder={() => true}
      onResponderGrant={(e) => apply(e.nativeEvent.locationX)}
      onResponderMove={(e) => apply(e.nativeEvent.locationX)}
    >
      <View className="h-1 w-full overflow-hidden rounded-full" style={{ backgroundColor: colors.border }}>
        <View style={{ width: `${volume * 100}%`, height: '100%', backgroundColor: colors.foreground }} />
      </View>
    </View>
  );
}

/** A playing radio is a fake Song carrying the station id — find that station. */
function useStation(id: string): RadioStation | undefined {
  const { data: stations } = useRadioStations();
  return stations?.find((s) => s.id === id);
}

function useIsRadio(id: string) {
  return !!useStation(id);
}

function LikeButton({ song }: { song: Song }) {
  const station = useStation(song.id);
  if (station) return <RadioLikeButton station={station} />;
  return <SongLikeButton song={song} />;
}

function RadioLikeButton({ station }: { station: RadioStation }) {
  const t = useT();
  const colors = useColors();
  const like = useRadioLike();
  const liked = !!station.liked;
  return (
    <IconButton
      name={liked ? 'heart' : 'heart-outline'}
      size={20}
      color={liked ? colors.primary : colors.muted}
      onPress={() => like.mutate({ id: station.id, liked: !liked })}
      accessibilityLabel={liked ? t('components.player.unlike') : t('components.player.like')}
    />
  );
}

function SongLikeButton({ song }: { song: Song }) {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const [liked, setLiked] = useState<boolean>(!!song.starred);

  const toggle = () => {
    if (!client) return;
    const next = !liked;
    setLiked(next); // optimistic
    const op = next ? client.star({ id: song.id }) : client.unstar({ id: song.id });
    op.then(
      // Refresh the "Mes titres likés" list so it reflects the change.
      () => void queryClient.invalidateQueries({ queryKey: qk.starred }),
      () => setLiked(!next),
    );
  };

  return (
    <IconButton
      name={liked ? 'heart' : 'heart-outline'}
      size={20}
      color={liked ? colors.primary : colors.muted}
      onPress={toggle}
      accessibilityLabel={liked ? t('components.player.unlike') : t('components.player.like')}
    />
  );
}

/** Circular "+" that opens a dropdown of playlists with checkboxes — ticking a
 * playlist adds the track, unticking removes it. A button opens a create popin. */
function AddToPlaylistButton({ song, disabled }: { song: Song; disabled?: boolean }) {
  const t = useT();
  const colors = useColors();
  const { height: screenH } = useWindowDimensions();
  const anchorRef = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState('');
  const { data: playlists } = usePlaylists();
  const createPlaylist = useCreatePlaylist();

  const open = () => anchorRef.current?.measureInWindow((x, y) => setAnchor({ x, y }));
  const close = () => setAnchor(null);

  const createAndAdd = () => {
    if (!newName.trim()) return;
    createPlaylist.mutate(
      { name: newName.trim(), songIds: [song.id] },
      { onSuccess: () => { setNewName(''); setCreateOpen(false); } },
    );
  };

  return (
    <>
      <Pressable
        ref={anchorRef}
        onPress={open}
        disabled={disabled}
        accessibilityState={{ disabled: !!disabled }}
        accessibilityLabel={t('components.player.addToPlaylist')}
        className={`h-8 w-8 items-center justify-center rounded-full border border-border ${disabled ? 'opacity-40' : 'active:opacity-70'}`}
      >
        <Ionicon name="add" size={18} color={colors.foreground} />
      </Pressable>

      {/* Dropdown */}
      <Modal transparent visible={!!anchor} animationType="fade" onRequestClose={close}>
        <Pressable className="flex-1" onPress={close}>
          {anchor ? (
            <View
              style={{ position: 'absolute', left: Math.max(8, anchor.x - 110), bottom: screenH - anchor.y + 8, width: 272 }}
              className="overflow-hidden rounded-2xl border border-border bg-surface"
            >
              <Pressable onPress={(e) => e.stopPropagation()}>
                <Text className="px-4 pb-1 pt-3 text-xs font-medium uppercase tracking-wider text-muted">
                  {t('components.player.addToPlaylist')}
                </Text>
                <ScrollView style={{ maxHeight: 240 }} showsVerticalScrollIndicator>
                  {(playlists ?? []).length ? (
                    (playlists ?? []).map((p) => <PlaylistCheckRow key={p.id} playlistId={p.id} name={p.name} songId={song.id} />)
                  ) : (
                    <Text className="px-4 py-2 text-sm text-muted">{t('components.player.noPlaylists')}</Text>
                  )}
                </ScrollView>
                <View className="border-t border-border p-2">
                  <Button
                    title={t('components.player.newPlaylist')}
                    icon="add"
                    size="sm"
                    variant="secondary"
                    onPress={() => {
                      close();
                      setCreateOpen(true);
                    }}
                  />
                </View>
              </Pressable>
            </View>
          ) : null}
        </Pressable>
      </Modal>

      {/* Create popin */}
      <Modal transparent visible={createOpen} animationType="fade" onRequestClose={() => setCreateOpen(false)}>
        <Pressable className="flex-1 items-center justify-center bg-black/60 px-6" onPress={() => setCreateOpen(false)}>
          <Pressable className="w-full max-w-[400px] gap-3 rounded-2xl bg-surface p-5" onPress={(e) => e.stopPropagation()}>
            <View className="flex-row items-center justify-between">
              <Text className="text-lg font-bold tracking-tight text-foreground">{t('components.player.newPlaylist')}</Text>
              <IconButton name="close" color={colors.muted} onPress={() => setCreateOpen(false)} accessibilityLabel={t('components.player.close')} />
            </View>
            <Field placeholder={t('components.player.playlistNamePlaceholder')} autoFocus value={newName} onChangeText={setNewName} onSubmitEditing={createAndAdd} />
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('components.player.cancel')} variant="ghost" onPress={() => setCreateOpen(false)} />
              </View>
              <View className="flex-1">
                <Button title={t('components.player.create')} icon="checkmark" loading={createPlaylist.isPending} disabled={!newName.trim()} onPress={createAndAdd} />
              </View>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

/** A playlist row with a checkbox reflecting whether the track is in it; toggling
 * adds or removes the track and the box updates from the refetched playlist. */
function PlaylistCheckRow({ playlistId, name, songId }: { playlistId: string; name: string; songId: string }) {
  const colors = useColors();
  const { data, isLoading } = usePlaylist(playlistId);
  const addTo = useAddToPlaylist();
  const removeFrom = useRemoveFromPlaylist();

  const index = (data?.entry ?? []).findIndex((s) => s.id === songId);
  const checked = index >= 0;
  const busy = addTo.isPending || removeFrom.isPending;

  const toggle = () => {
    if (busy) return;
    if (checked) removeFrom.mutate({ id: playlistId, indices: [index] });
    else addTo.mutate({ id: playlistId, songIds: [songId] });
  };

  return (
    <Pressable onPress={toggle} disabled={busy} className="flex-row items-center gap-3 px-4 py-2.5 active:bg-surface-alt">
      <View
        className={`h-5 w-5 items-center justify-center rounded-md border ${checked ? 'border-primary bg-primary' : 'border-border'}`}
      >
        {checked ? <Ionicon name="checkmark" size={14} color={colors.primaryForeground} /> : null}
      </View>
      <Text className="flex-1 text-sm text-foreground" numberOfLines={1}>
        {name}
      </Text>
      {isLoading || busy ? <ActivityIndicator size="small" color={colors.muted} /> : null}
    </Pressable>
  );
}

/**
 * Icon button opening a picker of this account's recently-active devices:
 * pick one to make it the sole active player (every other device pauses),
 * "This device" to take over playback here, or "Everywhere" to go back to
 * independent mode (today's default — every device manages its own playback).
 */
export function CastButton({ active, disabled }: { active?: boolean; disabled?: boolean }) {
  const t = useT();
  const colors = useColors();
  const { height: screenH } = useWindowDimensions();
  const anchorRef = useRef<View>(null);
  const [anchor, setAnchor] = useState<{ x: number; y: number } | null>(null);
  const open = !!anchor;

  const myId = useAuth((s) => s.client?.getSession()?.deviceId);
  const castTargetId = usePlayer((s) => s.castTargetId);
  const setCastTarget = usePlayer((s) => s.setCastTarget);
  const { data: targets, isLoading } = usePlaybackTargets(open);
  const others = (targets ?? []).filter((d) => d.id !== myId);

  const openPicker = () => anchorRef.current?.measureInWindow((x, y) => setAnchor({ x, y }));
  const close = () => setAnchor(null);
  const pick = (deviceId: string) => {
    close();
    void setCastTarget(deviceId);
  };

  return (
    <>
      <Pressable
        ref={anchorRef}
        onPress={openPicker}
        disabled={disabled}
        accessibilityState={{ disabled: !!disabled }}
        accessibilityLabel={t('components.player.cast')}
        className={`h-8 w-8 items-center justify-center rounded-full ${disabled ? 'opacity-40' : 'active:opacity-70'}`}
      >
        <Ionicon name={active ? 'tv' : 'tv-outline'} size={20} color={active ? colors.primary : colors.foreground} />
      </Pressable>

      <Modal transparent visible={open} animationType="fade" onRequestClose={close}>
        <Pressable className="flex-1" onPress={close}>
          {anchor ? (
            <View
              style={{ position: 'absolute', left: Math.max(8, anchor.x - 180), bottom: screenH - anchor.y + 8, width: 240 }}
              className="overflow-hidden rounded-2xl border border-border bg-surface"
            >
              <Pressable onPress={(e) => e.stopPropagation()}>
                <Text className="px-4 pb-1 pt-3 text-xs font-medium uppercase tracking-wider text-muted">
                  {t('components.player.castTitle')}
                </Text>
                <CastRow label={t('components.player.castEverywhere')} selected={!castTargetId} onPress={() => pick('')} />
                {myId ? (
                  <CastRow label={t('components.player.castThisDevice')} selected={castTargetId === myId} onPress={() => pick(myId)} />
                ) : null}
                {isLoading ? (
                  <View className="items-center py-3">
                    <ActivityIndicator size="small" color={colors.muted} />
                  </View>
                ) : others.length === 0 ? (
                  <Text className="px-4 py-2 text-sm text-muted">{t('components.player.castNoOtherDevices')}</Text>
                ) : (
                  others.map((d) => <CastRow key={d.id} label={d.name} selected={castTargetId === d.id} onPress={() => pick(d.id)} />)
                )}
              </Pressable>
            </View>
          ) : null}
        </Pressable>
      </Modal>
    </>
  );
}

function CastRow({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-2.5 active:bg-surface-alt">
      <Text className="flex-1 text-sm text-foreground" numberOfLines={1}>
        {label}
      </Text>
      {selected ? <Ionicon name="checkmark" size={16} color={colors.primary} /> : null}
    </Pressable>
  );
}
