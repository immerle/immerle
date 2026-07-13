import { Alert, Platform, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useDevices, useTokenMutations } from '../src/query/account';
import { Badge, Card, EmptyState, ErrorState, IconButton, Loading } from '../src/components/ui';
import { AdminHeader, AdminScroll } from '../src/components/AdminUI';
import { Ionicon } from '../src/components/Ionicon';
import { APITokenDTO } from '../src/api/immerleApi';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Connected devices: the caller's app-login sessions (API tokens minted by
 * the app's own login flow, `isDevice`, from `GET /tokens` — see
 * useDevices), each flagged online/offline based on whether it's made an
 * authenticated request in the last few minutes (server-computed
 * `connected`). Distinct from manually-created personal/CLI tokens, which
 * are managed on the API tokens screen.
 */
export default function Devices() {
  const t = useT();
  const q = useDevices();
  const { revoke } = useTokenMutations();

  const confirmRevoke = (id: string, label: string) => {
    const doIt = () => revoke.mutate(id);
    if (Platform.OS === 'web') doIt();
    else
      Alert.alert(t('tools.devices.revokeConfirmTitle'), label, [
        { text: t('tools.devices.cancel'), style: 'cancel' },
        { text: t('tools.devices.revoke'), style: 'destructive', onPress: doIt },
      ]);
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#14b8a6" title={t('tools.devices.title')} subtitle={t('tools.devices.subtitle')} />}
      >
        {q.isLoading ? (
          <Loading />
        ) : q.isError ? (
          <ErrorState message={t('tools.devices.loadError')} onRetry={q.refetch} />
        ) : !q.data?.length ? (
          <EmptyState icon="phone-portrait-outline" title={t('tools.devices.emptyTitle')} subtitle={t('tools.devices.emptySubtitle')} />
        ) : (
          q.data.map((d) => (
            <DeviceRow
              key={d.id}
              device={d}
              onRevoke={() => confirmRevoke(d.id!, d.name || t('tools.devices.fallbackName'))}
            />
          ))
        )}
      </AdminScroll>
    </>
  );
}

function DeviceRow({ device, onRevoke }: { device: APITokenDTO; onRevoke: () => void }) {
  const t = useT();
  const colors = useColors();
  const fmt = (d?: string) => (d ? new Date(d).toLocaleString() : t('tools.devices.neverSeen'));

  return (
    <Card className="gap-2">
      <View className="flex-row items-center gap-2">
        <Ionicon name="hardware-chip-outline" size={18} color={colors.primary} />
        <Text className="flex-1 text-base font-semibold text-foreground">{device.name || t('tools.devices.fallbackName')}</Text>
        <Badge label={device.connected ? t('tools.devices.online') : t('tools.devices.offline')} tone={device.connected ? 'success' : 'default'} />
        <IconButton name="trash-outline" size={20} color={colors.danger} onPress={onRevoke} accessibilityLabel={t('tools.devices.revoke')} />
      </View>
      <Text numberOfLines={1} className="text-xs text-muted">
        {t('tools.devices.lastSeen', { date: fmt(device.lastUsedAt) })}
      </Text>
    </Card>
  );
}
