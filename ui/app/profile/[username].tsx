import { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { useProfile, useMyJam, useJamMutations, useJamInviteMutations } from '../../src/query/social';
import { Badge, Button, Card, EmptyState, ErrorState, Loading, SectionHeader } from '../../src/components/ui';
import { CoverArt } from '../../src/components/CoverArt';
import { PlaylistCover } from '../../src/components/PlaylistCover';
import { TrackRow } from '../../src/components/TrackRow';
import { colorFor } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { usePlayer } from '../../src/audio/store';
import { useAuth } from '../../src/auth/store';
import { useJam } from '../../src/jam/store';
import { ActivityEventDTO, ProfilePlaylistDTO } from '../../src/api/immerleApi';
import { Song } from '../../src/api/subsonic/types';
import { formatRelativeTime, formatDuration } from '../../src/utils/format';
import { ScrollRow } from '../../src/components/ScrollRow';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

/** Listen-type events double as the "Recent plays" rail; the "Recent activity"
 * list below only shows the rest (add/like/playlist updates). */
function isPlayEvent(e: ActivityEventDTO): boolean {
  return e.type === 'listen' || e.type === 'scrobble';
}

/** First play of each distinct track, in feed order (most recent first). */
function recentPlaySongs(activity: ActivityEventDTO[]): Song[] {
  const seen = new Set<string>();
  const out: Song[] = [];
  for (const e of activity) {
    if (!isPlayEvent(e) || !e.itemId || !e.item?.title || seen.has(e.itemId)) continue;
    seen.add(e.itemId);
    out.push({
      id: e.itemId,
      title: e.item.title,
      artist: e.item.artist,
      album: e.item.album,
      albumId: e.item.albumId,
      artistId: e.item.artistId,
      coverArt: e.item.coverArt,
      duration: e.item.duration,
    });
    if (out.length >= 10) break;
  }
  return out;
}

function activityStyle(type?: string): { icon: string; color: string } {
  switch (type) {
    case 'listen':
    case 'scrobble':
      return { icon: 'musical-notes', color: '#22c55e' };
    case 'add':
    case 'playlist':
      return { icon: 'add-circle', color: '#3b82f6' };
    case 'like':
    case 'star':
    case 'favorite':
      return { icon: 'heart', color: '#f43f5e' };
    default:
      return { icon: 'sparkles', color: '#a78bfa' };
  }
}

function activityVerb(type: string | undefined, t: (key: string) => string): string {
  switch (type) {
    case 'listen':
    case 'scrobble':
      return t('social.profile.verbListened');
    case 'add':
      return t('social.profile.verbAdded');
    case 'playlist':
      return t('social.profile.verbUpdated');
    case 'like':
    case 'star':
    case 'favorite':
      return t('social.profile.verbLiked');
    default:
      return type ?? t('social.profile.verbDefault');
  }
}

/** Public profile of a user: identity, recent activity (privacy-honoured) and
 * their public playlists. Reached from the activity feed. */
export default function Profile() {
  const t = useT();
  const { username } = useLocalSearchParams<{ username: string }>();
  const q = useProfile(username ?? '');
  const playSongs = usePlayer((s) => s.playSongs);
  useWebTitle(q.data?.user.displayName ?? q.data?.user.username ?? username);

  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ title: t('social.profile.title') }} />
        <Loading />
      </>
    );
  }
  if (q.isError || !q.data) {
    return (
      <>
        <Stack.Screen options={{ title: t('social.profile.title') }} />
        <ErrorState message={t('social.profile.notFound')} onRetry={q.refetch} />
      </>
    );
  }

  const p = q.data;
  const name = p.user.displayName || p.user.username || t('social.profile.fallbackName');
  const handle = p.user.username ?? '';
  const accent = colorFor(name);
  const recentPlays = recentPlaySongs(p.activity);
  const otherActivity = p.activity.filter((e) => !isPlayEvent(e));

  return (
    <View className="flex-1 bg-background">
      <Stack.Screen options={{ title: name }} />
      <ScrollView contentContainerStyle={{ paddingBottom: 32 }}>
        <View className="overflow-hidden">
                <LinearGradient
                  colors={[accent + '66', accent + '1f', 'transparent']}
                  start={{ x: 0, y: 0 }}
                  end={{ x: 0, y: 1 }}
                  style={StyleSheet.absoluteFill}
                />
                <View className="gap-3 px-4 pb-6 pt-6">
                  <View className="flex-row items-center gap-4">
                    <View className="h-20 w-20 items-center justify-center rounded-full" style={{ backgroundColor: accent }}>
                      <Text className="text-3xl font-bold text-white">{name.charAt(0).toUpperCase()}</Text>
                    </View>
                    <View className="flex-1 gap-1">
                      <Text numberOfLines={1} className="text-2xl font-bold tracking-tight text-foreground">
                        {name}
                      </Text>
                      {handle && handle !== name ? <Text className="text-sm text-muted">@{handle}</Text> : null}
                      <View className="flex-row flex-wrap items-center gap-2 pt-0.5">
                        {p.user.isAdmin ? <Badge label={t('social.profile.admin')} tone="success" /> : null}
                        {p.isSelf ? <Badge label={t('social.profile.you')} tone="primary" /> : null}
                      </View>
                    </View>
                    {!p.isSelf && handle ? <InviteToJamButton username={handle} /> : null}
                  </View>
                </View>
              </View>

        {/* Stats */}
        <View className="flex-row gap-3 px-4 pt-4">
          <Stat label={t('social.profile.statPlays')} value={String(p.stats.plays)} />
          <Stat label={t('social.profile.statHours')} value={String(Math.round(p.stats.listenSeconds / 3600))} />
          <Stat label={t('social.profile.statPlaylists')} value={String(p.stats.playlists)} />
        </View>

        {/* Recent plays */}
        {recentPlays.length ? (
          <View>
            <SectionHeader title={t('social.profile.recentPlays')} />
            <ScrollRow showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12, paddingHorizontal: 16 }}>
              {recentPlays.map((s, i) => (
                <Pressable key={`${s.id}-${i}`} onPress={() => playSongs(recentPlays, i)} className="w-28 active:opacity-70">
                  <CoverArt coverArt={s.coverArt} size={112} rounded="rounded-lg" />
                  <Text numberOfLines={1} className="pt-1.5 text-sm font-medium text-foreground">
                    {s.title}
                  </Text>
                  <Text numberOfLines={1} className="text-xs text-muted">
                    {s.artist}
                  </Text>
                </Pressable>
              ))}
            </ScrollRow>
          </View>
        ) : null}

        {/* Hall of Fame top 3 (omitted server-side when empty) — plain ranked
            rows (same TrackRow used everywhere else) rather than the
            standalone Hall of Fame page's podium, so it reads as part of the
            profile instead of an embedded widget. */}
        {p.hallOfFame ? (
          <>
            <SectionHeader
              title={t('social.profile.hallOfFame')}
              action={
                <Pressable onPress={() => router.push((p.isSelf ? '/halloffame' : `/halloffame/${handle}`) as never)}>
                  <Text className="text-sm font-medium text-primary">{t('social.profile.seeAll', { count: p.hallOfFame.total })}</Text>
                </Pressable>
              }
            />
            <Card className="mx-4 p-0">
              {p.hallOfFame.top.map((s, i) => (
                <View key={s.id} className={i === 0 ? '' : 'border-t border-border'}>
                  <TrackRow song={s} rank={i + 1} onPress={() => playSongs(p.hallOfFame!.top, i)} />
                </View>
              ))}
            </Card>
          </>
        ) : null}

        {p.playlists.length ? (
          <>
            <SectionHeader title={t('social.profile.publicPlaylists')} />
            <View className="gap-2 px-4">
              {p.playlists.map((pl) => (
                <PlaylistRow key={pl.id} playlist={pl} onPress={() => pl.id && router.push(`/playlist/${pl.id}` as never)} />
              ))}
            </View>
          </>
        ) : null}

        {/* Other activity (add/like/playlist updates — plays have their own rail above) */}
        {otherActivity.length || (!recentPlays.length && !p.hallOfFame) ? (
          <>
            <SectionHeader title={t('social.profile.recentActivity')} />
            <View className="px-4">
              {!otherActivity.length ? (
                <EmptyState icon="pulse-outline" title={t('social.profile.emptyTitle')} subtitle={t('social.profile.emptySubtitle')} />
              ) : (
                <Card className="p-0">
                  {otherActivity.map((e, i) => (
                    <ActivityItem key={e.id ?? i} event={e} first={i === 0} />
                  ))}
                </Card>
              )}
            </View>
          </>
        ) : null}
      </ScrollView>
    </View>
  );
}

function ActivityItem({ event, first }: { event: ActivityEventDTO; first: boolean }) {
  const t = useT();
  const { icon, color } = activityStyle(event.type);
  const item = event.item;
  const goAlbum = () => {
    if (item?.albumId) router.push(`/album/${item.albumId}` as never);
  };
  return (
    <Pressable
      onPress={goAlbum}
      disabled={!item?.albumId}
      className={`flex-row items-center gap-3 px-4 py-2 ${first ? '' : 'border-t border-border'} ${item?.albumId ? 'active:bg-surface-alt' : ''}`}
    >
      <View>
        {item?.coverArt ? (
          <CoverArt coverArt={item.coverArt} size={40} rounded="rounded-md" />
        ) : (
          <View className="h-10 w-10 items-center justify-center rounded-md" style={{ backgroundColor: color + '26' }}>
            <Ionicon name={icon} size={18} color={color} />
          </View>
        )}
        <View className="absolute -bottom-1 -right-1 h-5 w-5 items-center justify-center rounded-full border-2 border-surface" style={{ backgroundColor: color }}>
          <Ionicon name={icon} size={11} color="#ffffff" />
        </View>
      </View>
      <View className="flex-1">
        <Text className="text-sm text-foreground" numberOfLines={1}>
          {activityVerb(event.type, t)}
          {item?.title ? <Text className="font-semibold"> {item.title}</Text> : null}
        </Text>
        {item?.artist ? (
          <Text className="text-xs text-muted" numberOfLines={1}>
            {item.artist}
          </Text>
        ) : null}
      </View>
      {event.createdAt ? <Text className="text-xs text-muted">{formatRelativeTime(event.createdAt)}</Text> : null}
    </Pressable>
  );
}

function PlaylistRow({ playlist, onPress }: { playlist: ProfilePlaylistDTO; onPress: () => void }) {
  const t = useT();
  const songCount = playlist.songCount ?? 0;
  return (
    <Pressable onPress={onPress} className="active:opacity-70">
      <Card className="flex-row items-center gap-3">
        <PlaylistCover coverArt={playlist.coverArt} covers={playlist.coverArts ?? []} size={44} rounded="rounded-lg" fallbackIcon="musical-notes" />
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground" numberOfLines={1}>
            {playlist.name || t('social.profile.playlistFallback')}
          </Text>
          <Text className="text-xs text-muted">
            {songCount > 1 ? t('social.profile.songCountPlural', { count: songCount }) : t('social.profile.songCount', { count: songCount })}
            {playlist.duration ? ` · ${formatDuration(playlist.duration)}` : ''}
          </Text>
        </View>
        <Ionicon name="chevron-forward" size={18} color="#888" />
      </Card>
    </Pressable>
  );
}

/** Invites this profile's user directly to a Jam — no picker, since we
 * already know exactly who. "Create a Jam" when the caller isn't hosting one
 * yet (inviting immediately after); "Invite to Jam" straight away otherwise.
 *
 * When already hosting, the local useJam store may not be connected — it
 * isn't persisted, so a page reload drops it even though the server still
 * has the caller as host. Reconnecting before inviting (without navigating —
 * the caller may be inviting several people from several profiles in a row)
 * is what keeps playback actually syncing to the Jam afterwards. */
function InviteToJamButton({ username }: { username: string }) {
  const t = useT();
  const client = useAuth((s) => s.client);
  const myJam = useMyJam();
  const jam = useJamMutations();
  const { invite } = useJamInviteMutations();
  const [invited, setInvited] = useState(false);

  const session = myJam.data?.session;
  const busy = jam.create.isPending || invite.isPending;

  const onPress = () => {
    if (session?.id) {
      if (useJam.getState().sessionId !== session.id) {
        useJam.getState().start(session.id, true);
      }
      invite.mutate({ sessionId: session.id, username }, { onSuccess: () => setInvited(true) });
      return;
    }
    jam.create.mutate(
      { name: t('social.jam.defaultName', { name: client?.username ?? '' }) },
      {
        onSuccess: (res) => {
          if (!res.session?.id) return;
          invite.mutate({ sessionId: res.session.id, username });
          useJam.getState().start(res.session.id, true);
          router.push(`/jam/${res.session.id}` as never);
        },
      },
    );
  };

  return (
    <Button
      title={invited ? t('social.jam.invited') : session ? t('social.jam.inviteButton') : t('social.jam.createButton')}
      icon={invited ? 'checkmark' : 'radio'}
      size="sm"
      variant={invited ? 'secondary' : 'primary'}
      loading={busy}
      disabled={invited}
      onPress={onPress}
    />
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <View className="flex-1 items-center rounded-2xl bg-surface-alt py-4">
      <Text className="text-2xl font-extrabold text-primary">{value}</Text>
      <Text className="text-sm text-muted">{label}</Text>
    </View>
  );
}
