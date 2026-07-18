import { useState } from 'react';
import { Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useBandcampStatus,
  useConnectBandcamp,
  useDisconnectBandcamp,
  useBandcampCollection,
  useImportBandcampItem,
} from '../src/query/purchases';
import { Badge, Button, Card, EmptyState, Field, Loading } from '../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle } from '../src/components/AdminUI';
import { CoverArt } from '../src/components/CoverArt';
import { BandcampCollectionItem, ImmerleApiError } from '../src/api/immerle/types';
import { useT } from '../src/i18n/store';

const STATUS_TONE: Record<string, 'success' | 'danger' | 'primary'> = {
  completed: 'success',
  failed: 'danger',
  queued: 'primary',
  running: 'primary',
};

/** Import your own Bandcamp purchases — gated by the `bandcampImport`
 * capability. Reached from Settings → Bandcamp purchases. */
export default function BandcampScreen() {
  const t = useT();
  const status = useBandcampStatus();
  const connect = useConnectBandcamp();
  const disconnect = useDisconnectBandcamp();
  const collection = useBandcampCollection(!!status.data?.connected);
  const importItem = useImportBandcampItem();
  const [cookie, setCookie] = useState('');

  const connected = !!status.data?.connected;

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color="#629aa9" title={t('bandcamp.title')} subtitle={t('bandcamp.subtitle')} />}>
        {status.isLoading ? (
          <Loading />
        ) : !connected ? (
          <Card className="gap-3">
            <CardTitle icon="cart" color="#629aa9" title={t('bandcamp.connectPrompt')} />
            <Field
              label={t('bandcamp.cookieLabel')}
              help={t('bandcamp.cookieHelp')}
              placeholder={t('bandcamp.cookiePlaceholder')}
              autoCapitalize="none"
              autoCorrect={false}
              secureTextEntry
              value={cookie}
              onChangeText={setCookie}
              onSubmitEditing={() => cookie.trim() && connect.mutate(cookie.trim())}
            />
            {connect.isError ? (
              <Text className="text-xs text-danger">
                {(connect.error as ImmerleApiError)?.code === 'invalid_cookie' ? t('bandcamp.invalidCookie') : t('bandcamp.connectError')}
              </Text>
            ) : null}
            <Button
              title={t('bandcamp.connectButton')}
              icon="link"
              loading={connect.isPending}
              disabled={!cookie.trim()}
              onPress={() => connect.mutate(cookie.trim(), { onSuccess: () => setCookie('') })}
            />
          </Card>
        ) : (
          <>
            <Card className="flex-row items-center justify-between gap-3">
              <View className="flex-1 gap-1">
                <Text className="text-base font-semibold text-foreground">{t('bandcamp.connected')}</Text>
                {status.data?.needsReconnect ? <Text className="text-xs text-danger">{t('bandcamp.reconnectNeeded')}</Text> : null}
              </View>
              <Button title={t('bandcamp.disconnect')} variant="danger" size="sm" loading={disconnect.isPending} onPress={() => disconnect.mutate()} />
            </Card>

            {collection.isLoading ? (
              <Loading />
            ) : collection.isError ? (
              <EmptyState icon="cloud-offline-outline" title={t('bandcamp.collectionError')} />
            ) : !collection.data?.length ? (
              <EmptyState icon="cart-outline" title={t('bandcamp.collectionEmpty')} />
            ) : (
              collection.data.map((item) => (
                <PurchaseRow
                  key={item.saleItemType + item.saleItemId}
                  item={item}
                  onImport={() => importItem.mutate(item)}
                />
              ))
            )}
          </>
        )}
      </AdminScroll>
    </>
  );
}

function PurchaseRow({ item, onImport }: { item: BandcampCollectionItem; onImport: () => void }) {
  const t = useT();
  const status = item.jobStatus;
  return (
    <Card className="flex-row items-center gap-3">
      <CoverArt url={item.artUrl} size={52} rounded="rounded-lg" fallbackIcon="disc" />
      <View className="flex-1 gap-0.5">
        <Text className="text-base font-semibold text-foreground" numberOfLines={1}>
          {item.itemTitle}
        </Text>
        <Text className="text-xs text-muted" numberOfLines={1}>
          {item.artistName}
        </Text>
      </View>
      {status ? (
        <Badge label={t(`bandcamp.status${status.charAt(0).toUpperCase()}${status.slice(1)}`)} tone={STATUS_TONE[status] ?? 'default'} />
      ) : (
        <Button title={t('bandcamp.importButton')} size="sm" onPress={onImport} />
      )}
    </Card>
  );
}
