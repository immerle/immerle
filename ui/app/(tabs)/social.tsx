import { useState } from 'react';
import { Modal, Pressable, ScrollView, Text, View } from 'react-native';
import { router } from 'expo-router';
import { SafeAreaView } from 'react-native-safe-area-context';
import { LinearGradient } from 'expo-linear-gradient';
import { useAuth } from '../../src/auth/store';
import { useActivity, useJamMutations, useMyJam } from '../../src/query/social';
import { useJam } from '../../src/jam/store';
import { Button, Card, EmptyState, Field, Loading, SectionHeader } from '../../src/components/ui';
import { Ionicon } from '../../src/components/Ionicon';
import { CoverArt } from '../../src/components/CoverArt';
import { ActivityEventDTO } from '../../src/api/immerleApi';
import { useColors } from '../../src/theme/colors';
import { formatRelativeTime } from '../../src/utils/format';
import { useT } from '../../src/i18n/store';

/** Per-activity icon + accent colour, used for the timeline chips. */
function activityStyle(type?: string): { icon: string; color: string } {
  switch (type) {
    case 'listen':
    case 'scrobble':
      return { icon: 'musical-notes', color: '#22c55e' };
    case 'playlist':
      return { icon: 'list', color: '#3b82f6' };
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
    case 'like':
    case 'star':
    case 'favorite':
      return t('social.feed.verbLiked');
    default:
      return e.type ?? t('social.feed.verbDefault');
  }
}

/**
 * Social hub: the activity feed plus an entry point to Jam listening
 * sessions. Gated by the `social` capability; the whole tab is hidden when
 * the server doesn't advertise it.
 */
export default function Social() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const activity = useActivity();
  const jam = useJamMutations();
  const myJam = useMyJam();

  const [joinId, setJoinId] = useState('');
  const [joinOpen, setJoinOpen] = useState(false);

  if (!client?.has('social')) {
    return (
      <SafeAreaView edges={['top']} className="flex-1 bg-background">
        <EmptyState icon="people" title={t('social.unavailable.title')} subtitle={t('social.unavailable.subtitle')} />
      </SafeAreaView>
    );
  }

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

  // The caller may already be hosting a session created elsewhere (e.g. from
  // a profile page) — the local sync store isn't persisted, so a reload can
  // lose it even though the server still has them as host.
  const activeSession = myJam.data?.session;
  const openMyJam = () => {
    if (!activeSession?.id) return;
    if (useJam.getState().sessionId !== activeSession.id) {
      useJam.getState().start(activeSession.id, true);
    }
    router.push(`/jam/${activeSession.id}` as never);
  };

  return (
    <SafeAreaView edges={['top']} className="flex-1 bg-background">
      <ScrollView contentContainerStyle={{ paddingBottom: 40 }}>
        <Text className="px-4 pb-2 pt-3 text-3xl font-bold tracking-tight text-foreground">Social</Text>

        {client.has('jam') ? (
          <View className="px-4 pt-1">
            <LinearGradient
              colors={[colors.primary, '#0c0c0c']}
              start={{ x: 0, y: 0 }}
              end={{ x: 1, y: 1 }}
              style={{ borderRadius: 20, overflow: 'hidden' }}
            >
              <View className="p-5">
                <View style={{ position: 'absolute', right: -8, top: -8, opacity: 0.18 }}>
                  <Ionicon name="radio" size={120} color="#ffffff" />
                </View>
                <View className="flex-row items-center gap-2">
                  <Ionicon name="radio" size={18} color="#ffffff" />
                  <Text className="text-xs font-bold uppercase tracking-widest text-white/90">{t('social.jam.label')}</Text>
                </View>
                {activeSession ? (
                  <>
                    <Text className="pt-3 text-2xl font-bold tracking-tight text-white">
                      {activeSession.name || t('social.jam.heroTitle')}
                    </Text>
                    <Text className="max-w-[80%] pt-1 text-sm text-white/80">{t('social.jam.activeHint')}</Text>
                    <View className="mt-4 flex-row flex-wrap items-center gap-2">
                      <Pressable
                        onPress={openMyJam}
                        className="flex-row items-center gap-2 self-start rounded-full bg-white px-5 py-2.5 active:opacity-80"
                      >
                        <Ionicon name="radio" size={18} color="#000000" />
                        <Text className="font-bold tracking-tight text-black">{t('social.jam.openMine')}</Text>
                      </Pressable>
                    </View>
                  </>
                ) : (
                  <>
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
                  </>
                )}
              </View>
            </LinearGradient>
          </View>
        ) : null}

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
