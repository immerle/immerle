import { useState } from 'react';
import { Modal, Pressable, ScrollView, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LinearGradient } from 'expo-linear-gradient';
import { useAuth } from '../../src/auth/store';
import {
  useActivity,
  useFriendMutations,
  useFriends,
  useJamMutations,
  usePendingFriends,
} from '../../src/query/social';
import { useJam } from '../../src/jam/store';
import { Badge, Button, Card, EmptyState, Field, Loading, SectionHeader } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { CoverArt } from '../../src/components/CoverArt';
import { ActivityEventDTO } from '../../src/api/immerleApi';
import { useColors } from '../../src/theme/colors';
import { formatRelativeTime } from '../../src/utils/format';
import { useT } from '../../src/i18n/store';

// Deterministic, vivid avatar gradients — picked by username hash so each person
// keeps a stable, recognizable colour.
const AVATAR_GRADIENTS: [string, string][] = [
  ['#f97316', '#db2777'],
  ['#3b82f6', '#06b6d4'],
  ['#8b5cf6', '#6366f1'],
  ['#10b981', '#059669'],
  ['#f59e0b', '#ef4444'],
  ['#ec4899', '#8b5cf6'],
  ['#14b8a6', '#3b82f6'],
  ['#eab308', '#f97316'],
];

function hashString(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return h;
}

/** Per-activity icon + accent colour, used for the timeline chips. */
function activityStyle(type?: string): { icon: string; color: string } {
  switch (type) {
    case 'listen':
    case 'scrobble':
      return { icon: 'musical-notes', color: '#22c55e' };
    case 'playlist':
      return { icon: 'list', color: '#3b82f6' };
    case 'friend':
      return { icon: 'person-add', color: '#ec4899' };
    case 'like':
    case 'star':
    case 'favorite':
      return { icon: 'heart', color: '#f43f5e' };
    default:
      return { icon: 'sparkles', color: '#a78bfa' };
  }
}

function activityVerb(e: ActivityEventDTO, t: (key: string) => string): string {
  switch (e.type) {
    case 'listen':
    case 'scrobble':
      return t('social.feed.verbListened');
    case 'playlist':
      return t('social.feed.verbPlaylist');
    case 'friend':
      return t('social.feed.verbFriend');
    case 'like':
    case 'star':
    case 'favorite':
      return t('social.feed.verbLiked');
    default:
      return e.type ?? t('social.feed.verbDefault');
  }
}

/**
 * Social hub: friends, pending requests, and the activity feed — plus an entry
 * point to Jam listening sessions. Gated by the `social` capability; the whole
 * tab is hidden when the server doesn't advertise it.
 */
export default function Social() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const friends = useFriends();
  const pending = usePendingFriends();
  const activity = useActivity();
  const { request, accept } = useFriendMutations();
  const jam = useJamMutations();

  const [addName, setAddName] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [joinId, setJoinId] = useState('');
  const [joinOpen, setJoinOpen] = useState(false);

  if (!client?.has('social')) {
    return (
      <SafeAreaView edges={['top']} className="flex-1 bg-background">
        <EmptyState icon="people" title={t('social.unavailable.title')} subtitle={t('social.unavailable.subtitle')} />
      </SafeAreaView>
    );
  }

  const sendRequest = () => {
    if (!addName.trim()) return;
    request.mutate(addName.trim(), {
      onSuccess: () => {
        setAddName('');
        setAddOpen(false);
      },
    });
  };

  const startJam = () => {
    jam.create.mutate(
      { name: t('social.jam.defaultName', { name: client.username }) },
      {
        onSuccess: (res) => {
          if (res.session?.id) {
            useJam.getState().start(res.session.id, true);
            router.push(`/jam/${res.session.id}` as never);
          }
        },
      },
    );
  };

  const joinJam = () => {
    if (!joinId.trim()) return;
    jam.join.mutate(joinId.trim(), {
      onSuccess: (res) => {
        const id = res.session?.id ?? joinId.trim();
        setJoinId('');
        setJoinOpen(false);
        useJam.getState().start(id, false);
        router.push(`/jam/${id}` as never);
      },
    });
  };

  const friendCount = friends.data?.length ?? 0;
  const pendingCount = pending.data?.length ?? 0;

  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      <ScrollView contentContainerStyle={{ paddingBottom: 40 }}>
        <Text className="px-4 pb-2 pt-3 text-3xl font-bold tracking-tight text-foreground">Social</Text>

        {/* Jam hero */}
        {client.has('jam') ? (
          <View className="px-4 pt-1">
            <LinearGradient
              colors={[colors.primary, '#0c0c0c']}
              start={{ x: 0, y: 0 }}
              end={{ x: 1, y: 1 }}
              style={{ borderRadius: 20, overflow: 'hidden' }}
            >
              <View className="p-5">
                {/* Faint watermark icon */}
                <View style={{ position: 'absolute', right: -8, top: -8, opacity: 0.18 }}>
                  <Ionicon name="radio" size={120} color="#ffffff" />
                </View>
                <View className="flex-row items-center gap-2">
                  <Ionicon name="radio" size={18} color="#ffffff" />
                  <Text className="text-xs font-bold uppercase tracking-widest text-white/90">{t('social.jam.label')}</Text>
                </View>
                <Text className="pt-3 text-2xl font-bold tracking-tight text-white">{t('social.jam.heroTitle')}</Text>
                <Text className="max-w-[80%] pt-1 text-sm text-white/80">
                  {t('social.jam.heroSubtitle')}
                </Text>
                <View className="mt-4 flex-row flex-wrap items-center gap-2">
                  <Pressable
                    onPress={startJam}
                    disabled={jam.create.isPending}
                    className={`flex-row items-center gap-2 self-start rounded-full bg-white px-5 py-2.5 ${jam.create.isPending ? 'opacity-60' : 'active:opacity-80'}`}
                  >
                    <Ionicon name="add" size={18} color="#000000" />
                    <Text className="font-bold tracking-tight text-black">{t('social.jam.start')}</Text>
                  </Pressable>
                  <Pressable
                    onPress={() => setJoinOpen(true)}
                    className="flex-row items-center gap-2 self-start rounded-full border border-white/70 px-5 py-2.5 active:bg-white/10"
                  >
                    <Ionicon name="enter-outline" size={18} color="#ffffff" />
                    <Text className="font-bold tracking-tight text-white">{t('social.jam.join')}</Text>
                  </Pressable>
                </View>
              </View>
            </LinearGradient>
          </View>
        ) : null}

        {/* Pending requests */}
        {pendingCount > 0 ? (
          <>
            <SectionHeader
              title={t('social.pending.title')}
              action={<Badge label={String(pendingCount)} tone="primary" />}
            />
            <View className="gap-2 px-4">
              {pending.data!.map((p) => (
                <Card key={p.id ?? p.username} className="flex-row items-center gap-3">
                  <Avatar name={p.displayName || p.username} size={44} />
                  <View className="flex-1">
                    <Text className="text-base font-semibold text-foreground">{p.displayName || p.username}</Text>
                    <Text className="text-xs text-muted">
                      {t('social.pending.invitedYou')}{p.since ? ` · ${t('social.pending.ago', { time: formatRelativeTime(p.since) })}` : ''}
                    </Text>
                  </View>
                  <Button
                    title={t('social.pending.accept')}
                    size="sm"
                    icon="checkmark"
                    loading={accept.isPending}
                    onPress={() => p.username && accept.mutate(p.username)}
                  />
                </Card>
              ))}
            </View>
          </>
        ) : null}

        {/* Friends */}
        <SectionHeader
          title={t('social.friends.title')}
          action={
            <View className="flex-row items-center gap-2">
              {friendCount > 0 ? (
                <View className="h-8 min-w-8 items-center justify-center rounded-full bg-surface-alt px-2.5">
                  <Text className="text-sm font-bold text-foreground">{friendCount}</Text>
                </View>
              ) : null}
              <Pressable
                onPress={() => setAddOpen(true)}
                accessibilityLabel={t('social.friends.add')}
                className="h-8 w-8 items-center justify-center rounded-full bg-primary active:opacity-80"
              >
                <Ionicon name="add" size={20} color={colors.primaryForeground} />
              </Pressable>
            </View>
          }
        />
        <View className="px-4">
          {friends.isLoading ? (
            <Loading />
          ) : friendCount === 0 ? (
            <EmptyState icon="people-outline" title={t('social.friends.emptyTitle')} subtitle={t('social.friends.emptySubtitle')} />
          ) : (
            <View className="gap-2">
              {friends.data!.map((f) => (
                <Pressable
                  key={f.id ?? f.username}
                  onPress={() => f.username && router.push(`/profile/${f.username}` as never)}
                  className="active:opacity-70"
                >
                  <Card className="flex-row items-center gap-3">
                    <Avatar name={f.displayName || f.username} size={44} />
                    <View className="flex-1">
                      <Text className="text-base font-semibold text-foreground">{f.displayName || f.username}</Text>
                      <Text className="text-xs text-muted">{f.displayName && f.displayName !== f.username ? `@${f.username}` : t('social.friends.friendTag')}</Text>
                    </View>
                    <Ionicon name="chevron-forward" size={18} color={colors.muted} />
                  </Card>
                </Pressable>
              ))}
            </View>
          )}
        </View>

        {/* Activity */}
        <SectionHeader title={t('social.feed.title')} />
        <View className="px-4">
          {activity.isLoading ? (
            <Loading />
          ) : !activity.data?.length ? (
            <EmptyState icon="pulse-outline" title={t('social.feed.emptyTitle')} subtitle={t('social.feed.emptySubtitle')} />
          ) : (
            <Card className="p-0">
              <ScrollView
                style={{ maxHeight: 320 }}
                nestedScrollEnabled
                showsVerticalScrollIndicator
              >
                {activity.data.map((e, i) => (
                  <ActivityRow key={e.id ?? i} event={e} first={i === 0} />
                ))}
              </ScrollView>
            </Card>
          )}
        </View>
      </ScrollView>

      {/* Add-a-friend popin */}
      <Modal transparent animationType="fade" visible={addOpen} onRequestClose={() => setAddOpen(false)}>
        <Pressable
          className="flex-1 items-center justify-center bg-black/60 px-6"
          onPress={() => setAddOpen(false)}
        >
          <Pressable
            className="w-full max-w-[420px] gap-3 rounded-2xl bg-surface p-5"
            onPress={(e) => e.stopPropagation()}
          >
            <View className="flex-row items-center justify-between">
              <Text className="text-lg font-bold tracking-tight text-foreground">{t('social.addModal.title')}</Text>
              <Pressable onPress={() => setAddOpen(false)} accessibilityLabel={t('social.common.close')}>
                <Ionicon name="close" size={22} color={colors.muted} />
              </Pressable>
            </View>
            <Text className="text-sm text-muted">{t('social.addModal.subtitle')}</Text>
            <Field
              icon="person-add-outline"
              placeholder={t('social.addModal.placeholder')}
              autoCapitalize="none"
              autoCorrect={false}
              autoFocus
              value={addName}
              onChangeText={setAddName}
              onSubmitEditing={sendRequest}
            />
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('social.common.cancel')} variant="ghost" onPress={() => setAddOpen(false)} />
              </View>
              <View className="flex-1">
                <Button
                  title={t('social.addModal.invite')}
                  icon="send"
                  loading={request.isPending}
                  disabled={!addName.trim()}
                  onPress={sendRequest}
                />
              </View>
            </View>
          </Pressable>
        </Pressable>
      </Modal>

      {/* Join-a-Jam popin */}
      <Modal transparent animationType="fade" visible={joinOpen} onRequestClose={() => setJoinOpen(false)}>
        <Pressable
          className="flex-1 items-center justify-center bg-black/60 px-6"
          onPress={() => setJoinOpen(false)}
        >
          <Pressable
            className="w-full max-w-[420px] gap-3 rounded-2xl bg-surface p-5"
            onPress={(e) => e.stopPropagation()}
          >
            <View className="flex-row items-center justify-between">
              <Text className="text-lg font-bold tracking-tight text-foreground">{t('social.joinModal.title')}</Text>
              <Pressable onPress={() => setJoinOpen(false)} accessibilityLabel={t('social.common.close')}>
                <Ionicon name="close" size={22} color={colors.muted} />
              </Pressable>
            </View>
            <Text className="text-sm text-muted">{t('social.joinModal.subtitle')}</Text>
            <Field
              icon="enter-outline"
              placeholder={t('social.joinModal.placeholder')}
              autoCapitalize="none"
              autoCorrect={false}
              autoFocus
              value={joinId}
              onChangeText={setJoinId}
              onSubmitEditing={joinJam}
            />
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('social.common.cancel')} variant="ghost" onPress={() => setJoinOpen(false)} />
              </View>
              <View className="flex-1">
                <Button
                  title={t('social.joinModal.join')}
                  icon="enter-outline"
                  loading={jam.join.isPending}
                  disabled={!joinId.trim()}
                  onPress={joinJam}
                />
              </View>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
    </SafeAreaView>
  );
}

function Avatar({ name, size = 40 }: { name?: string; size?: number }) {
  const initial = (name ?? '?').charAt(0).toUpperCase();
  const [c1, c2] = AVATAR_GRADIENTS[hashString(name ?? '?') % AVATAR_GRADIENTS.length];
  return (
    <LinearGradient
      colors={[c1, c2]}
      start={{ x: 0, y: 0 }}
      end={{ x: 1, y: 1 }}
      style={{ width: size, height: size, borderRadius: size / 2, alignItems: 'center', justifyContent: 'center' }}
    >
      <Text style={{ fontSize: size * 0.42 }} className="font-bold text-white">
        {initial}
      </Text>
    </LinearGradient>
  );
}

function ActivityRow({ event, first }: { event: ActivityEventDTO; first: boolean }) {
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
      {/* Artwork with a small action-type badge */}
      <View>
        {item?.coverArt ? (
          <CoverArt coverArt={item.coverArt} size={40} rounded="rounded-md" />
        ) : (
          <View className="h-10 w-10 items-center justify-center rounded-md" style={{ backgroundColor: color + '26' }}>
            <Ionicon name={icon} size={18} color={color} />
          </View>
        )}
        <View
          className="absolute -bottom-1 -right-1 h-5 w-5 items-center justify-center rounded-full border-2 border-surface"
          style={{ backgroundColor: color }}
        >
          <Ionicon name={icon} size={11} color="#ffffff" />
        </View>
      </View>

      <View className="flex-1">
        <Text className="text-sm text-foreground" numberOfLines={1}>
          <Text
            className="font-semibold"
            onPress={(e) => {
              e.stopPropagation?.();
              if (event.username) router.push(`/profile/${event.username}` as never);
            }}
          >
            {event.displayName || event.username || t('social.feed.someone')}
          </Text>{' '}
          {activityVerb(event, t)}
        </Text>
        {item?.title ? (
          <Text className="text-xs text-muted" numberOfLines={1}>
            {item.title}
            {item.artist ? ` · ${item.artist}` : ''}
          </Text>
        ) : null}
      </View>

      {event.createdAt ? (
        <Text className="text-xs text-muted">{formatRelativeTime(event.createdAt)}</Text>
      ) : null}
    </Pressable>
  );
}
