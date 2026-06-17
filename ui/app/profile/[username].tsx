import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { LinearGradient } from 'expo-linear-gradient';
import { useProfile, useFriendMutations } from '../../src/query/social';
import { Badge, Button, Card, EmptyState, ErrorState, Loading, SectionHeader } from '../../src/components/ui';
import { CoverArt } from '../../src/components/CoverArt';
import { PlaylistMosaic } from '../../src/components/PlaylistMosaic';
import { colorFor } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { ActivityEventDTO, ProfilePlaylistDTO } from '../../src/api/immerleApi';
import { formatRelativeTime, formatDuration } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';

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

function activityVerb(type?: string): string {
  switch (type) {
    case 'listen':
    case 'scrobble':
      return 'a écouté';
    case 'add':
      return 'a ajouté';
    case 'playlist':
      return 'a mis à jour';
    case 'like':
    case 'star':
    case 'favorite':
      return 'a aimé';
    default:
      return type ?? 'a fait une action';
  }
}

/** Public profile of a user: identity, recent activity (privacy-honoured) and
 * their public playlists. Reached from the friends list / activity feed. */
export default function Profile() {
  const colors = useColors();
  const { username } = useLocalSearchParams<{ username: string }>();
  const q = useProfile(username ?? '');
  const { request } = useFriendMutations();

  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ title: 'Profil' }} />
        <Loading />
      </>
    );
  }
  if (q.isError || !q.data) {
    return (
      <>
        <Stack.Screen options={{ title: 'Profil' }} />
        <ErrorState message="Profil introuvable." onRetry={q.refetch} />
      </>
    );
  }

  const p = q.data;
  const name = p.user.displayName || p.user.username || 'Utilisateur';
  const handle = p.user.username ?? '';
  const accent = colorFor(name);

  return (
    <View className="flex-1 bg-background">
      <Stack.Screen options={{ title: name }} />
      <ScrollView contentContainerStyle={{ paddingBottom: 32 }}>
        {/* Hero */}
        <View className="overflow-hidden">
                <LinearGradient
                  colors={[accent + '66', accent + '1f', 'transparent']}
                  start={{ x: 0, y: 0 }}
                  end={{ x: 0, y: 1 }}
                  style={StyleSheet.absoluteFill}
                />
                <View className="items-center gap-2 px-4 pb-6 pt-6">
                  <View className="h-24 w-24 items-center justify-center rounded-full" style={{ backgroundColor: accent }}>
                    <Text className="text-4xl font-bold text-white">{name.charAt(0).toUpperCase()}</Text>
                  </View>
                  <Text className="text-2xl font-bold tracking-tight text-foreground">{name}</Text>
                  {handle && handle !== name ? <Text className="text-sm text-muted">@{handle}</Text> : null}
                  <View className="flex-row flex-wrap items-center justify-center gap-2 pt-1">
                    {p.user.isAdmin ? <Badge label="Admin" tone="success" /> : null}
                    {p.isSelf ? <Badge label="Vous" tone="primary" /> : p.isFriend ? <Badge label="Ami" tone="default" /> : null}
                  </View>
                  {!p.isSelf && !p.isFriend ? (
                    <View className="pt-2">
                      <Button
                        title="Ajouter en ami"
                        icon="person-add"
                        size="sm"
                        loading={request.isPending}
                        onPress={() => handle && request.mutate(handle, { onSuccess: () => q.refetch() })}
                      />
                    </View>
                  ) : null}
                </View>
              </View>

              {/* Activity */}
              <SectionHeader title="Activité récente" />
              <View className="px-4">
                {!p.activity.length ? (
                  <EmptyState icon="pulse-outline" title="Rien à afficher" subtitle="Aucune activité visible." />
                ) : (
                  <Card className="p-0">
                    {p.activity.map((e, i) => (
                      <ActivityItem key={e.id ?? i} event={e} first={i === 0} />
                    ))}
                  </Card>
                )}
              </View>

        {/* Public playlists */}
        {p.playlists.length ? (
          <>
            <SectionHeader title="Playlists publiques" />
            <View className="gap-2 px-4">
              {p.playlists.map((pl) => (
                <PlaylistRow key={pl.id} playlist={pl} onPress={() => pl.id && router.push(`/playlist/${pl.id}` as never)} />
              ))}
            </View>
          </>
        ) : null}
      </ScrollView>
    </View>
  );
}

function ActivityItem({ event, first }: { event: ActivityEventDTO; first: boolean }) {
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
          {activityVerb(event.type)}
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
  return (
    <Pressable onPress={onPress} className="active:opacity-70">
      <Card className="flex-row items-center gap-3">
        <PlaylistMosaic covers={playlist.coverArts ?? []} size={44} rounded="rounded-lg" fallbackIcon="musical-notes" />
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground" numberOfLines={1}>
            {playlist.name || 'Playlist'}
          </Text>
          <Text className="text-xs text-muted">
            {playlist.songCount ?? 0} titre{(playlist.songCount ?? 0) > 1 ? 's' : ''}
            {playlist.duration ? ` · ${formatDuration(playlist.duration)}` : ''}
          </Text>
        </View>
        <Ionicon name="chevron-forward" size={18} color="#888" />
      </Card>
    </Pressable>
  );
}
