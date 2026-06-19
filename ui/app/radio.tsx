import { Linking, Pressable, Text, View } from 'react-native';
import { Stack, router } from 'expo-router';
import { FlashList } from '@shopify/flash-list';
import { useRadioStations } from '../src/query/radio';
import { useAuth } from '../src/auth/store';
import { usePlayer } from '../src/audio/store';
import { EmptyState, ErrorState, IconButton, Loading } from '../src/components/ui';
import { Ionicon } from '../src/components/Ionicon';
import { StationCover } from '../src/components/StationCover';
import { RadioStation } from '../src/api/immerle/types';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/** Internet radio: built-in + admin-managed stations. Tap to stream live. */
export default function Radio() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const isAdmin = useAuth((s) => s.client?.isAdmin ?? false);
  const playRadio = usePlayer((s) => s.playRadio);
  const q = useRadioStations();

  if (!client?.has('internetRadio')) {
    return (
      <>
        <Stack.Screen options={{ title: t('radio.title') }} />
        <View className="flex-1 bg-background">
          <EmptyState icon="radio-outline" title={t('radio.unavailableTitle')} subtitle={t('radio.unavailableSubtitle')} />
        </View>
      </>
    );
  }

  return (
    <>
      <Stack.Screen
        options={{
          title: t('radio.title'),
          headerRight: () =>
            isAdmin ? (
              <IconButton name="settings-outline" color={colors.primary} onPress={() => router.push('/admin/radio' as never)} accessibilityLabel={t('radio.manage')} />
            ) : null,
        }}
      />
      <View className="flex-1 bg-background">
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('radio.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="radio-outline" title={t('radio.emptyTitle')} subtitle={t('radio.emptySubtitle')} />
        ) : (
          <FlashList<RadioStation>
            data={q.data}
            keyExtractor={(s) => s.id}
            estimatedItemSize={64}
            refreshing={q.isRefetching}
            onRefresh={q.refetch}
            renderItem={({ item }) => (
              <Pressable
                onPress={() => playRadio({ id: item.id, name: item.name, streamUrl: item.streamUrl })}
                className="flex-row items-center gap-3 px-4 py-2.5 active:bg-surface-alt"
              >
                <StationCover uri={item.hasCover ? client.radioCoverUrl(item.id) : undefined} size={48} rounded={8} />
                <View className="flex-1">
                  <Text numberOfLines={1} className="text-base font-semibold text-foreground">{item.name}</Text>
                  {item.homepageUrl ? (
                    <Text numberOfLines={1} className="text-sm text-muted" onPress={() => Linking.openURL(item.homepageUrl)}>
                      {item.homepageUrl}
                    </Text>
                  ) : null}
                </View>
                <Ionicon name="play" size={22} color={colors.foreground} />
              </Pressable>
            )}
            contentContainerStyle={{ paddingVertical: 8 }}
          />
        )}
      </View>
    </>
  );
}
