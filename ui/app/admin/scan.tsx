import { useEffect, useState } from 'react';
import { Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useLibraryStats,
  useScanProgress,
  useSettings,
  useStartScan,
  useUpdateSettings,
} from '../../src/query/admin';
import { Badge, Button, Card, Field } from '../../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { formatBytes, formatCount } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';

/** Library admin: stats + full / incremental scan with live progress. */
export default function AdminScan() {
  const colors = useColors();
  const stats = useLibraryStats();
  const progress = useScanProgress();
  const startScan = useStartScan();

  const scanning = progress.data?.scanning ?? false;
  const pct =
    progress.data?.total && progress.data.total > 0
      ? Math.min((progress.data.count / progress.data.total) * 100, 100)
      : null;

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#f59e0b" title="Bibliothèque" subtitle="Scan & statistiques" />}
      >
        {/* Stats grid */}
        <View className="flex-row flex-wrap gap-2.5">
          <StatTile icon="people" color="#3b82f6" label="Artistes" value={formatCount(stats.data?.artistCount)} />
          <StatTile icon="albums" color="#8b5cf6" label="Albums" value={formatCount(stats.data?.albumCount)} />
          <StatTile icon="musical-notes" color="#1ed760" label="Titres" value={formatCount(stats.data?.songCount)} />
          <StatTile icon="server" color="#f59e0b" label="Espace" value={stats.data ? formatBytes(stats.data.totalSize) : '—'} />
        </View>
        {stats.data?.lastScan ? (
          <Text className="px-1 text-xs text-muted">Dernier scan · {stats.data.lastScan}</Text>
        ) : null}

        {/* Scan control */}
        <Card className="gap-3">
          <CardTitle
            icon={scanning ? 'sync' : 'checkmark-circle'}
            color={scanning ? '#f59e0b' : colors.success}
            title={scanning ? 'Scan en cours…' : 'Bibliothèque à jour'}
          />

          {scanning ? (
            <View className="gap-1.5">
              <View className="h-2 w-full overflow-hidden rounded-full bg-surface-alt">
                <View className="h-full rounded-full bg-primary" style={{ width: pct != null ? `${pct}%` : '40%' }} />
              </View>
              <Text className="text-xs text-muted">
                {formatCount(progress.data?.count)} éléments traités
                {pct != null ? ` · ${Math.round(pct)}%` : ''}
                {progress.data?.phase ? ` · ${progress.data.phase}` : ''}
              </Text>
            </View>
          ) : (
            <Text className="text-sm text-muted">
              Lancez un scan pour détecter les nouveaux fichiers ou reconstruire l'index complet.
            </Text>
          )}

          <View className="flex-row gap-2 pt-1">
            <View className="flex-1">
              <Button
                title="Scan incrémental"
                icon="refresh"
                variant="secondary"
                disabled={scanning}
                loading={startScan.isPending && !startScan.variables}
                onPress={() => startScan.mutate(false)}
              />
            </View>
            <View className="flex-1">
              <Button
                title="Scan complet"
                icon="refresh-circle"
                disabled={scanning}
                loading={startScan.isPending && startScan.variables === true}
                onPress={() => startScan.mutate(true)}
              />
            </View>
          </View>
        </Card>

        <ScanConfigCard />
      </AdminScroll>
    </>
  );
}

/** Automatic scan cadence (runtime settings). Self-hides when the instance
 * doesn't expose the settings endpoint. */
function ScanConfigCard() {
  const colors = useColors();
  const q = useSettings();
  const update = useUpdateSettings();
  const [interval, setIntervalS] = useState('');
  const [watch, setWatch] = useState(false);

  useEffect(() => {
    const s = q.data?.settings.scan;
    if (!s) return;
    setIntervalS(String(s.intervalSeconds ?? 0));
    setWatch(s.watch ?? false);
  }, [q.data?.settings.scan]);

  if (!q.data) return null;
  const pendingWatch = (q.data.pendingRestart ?? []).includes('scan.watch');

  const save = () =>
    update.mutate({ scan: { intervalSeconds: Number(interval) || 0, watch } });

  return (
    <Card className="gap-3">
      <CardTitle icon="time-outline" color="#0ea5e9" title="Scan automatique" />
      <Field
        label="Intervalle de scan (s, 0 = jamais)"
        keyboardType="number-pad"
        value={interval}
        onChangeText={setIntervalS}
      />
      <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
        <View className="flex-1 flex-row items-center gap-2 pr-2">
          <Text className="text-sm text-foreground">Surveiller les fichiers en continu</Text>
          <Badge label="Redémarrage" tone="default" />
        </View>
        <Switch value={watch} onValueChange={setWatch} trackColor={{ true: colors.primary, false: colors.border }} />
      </View>
      {pendingWatch ? (
        <Text className="text-xs text-danger">Redémarrez l'instance pour appliquer la surveillance.</Text>
      ) : null}
      <View className="flex-row justify-end pt-1">
        <Button title="Enregistrer" size="sm" icon="save-outline" loading={update.isPending} onPress={save} />
      </View>
    </Card>
  );
}

function StatTile({ icon, color, label, value }: { icon: string; color: string; label: string; value: string }) {
  return (
    <View className="min-w-[46%] flex-1 flex-row items-center gap-3 rounded-2xl bg-surface p-3.5">
      <View className="h-10 w-10 items-center justify-center rounded-xl" style={{ backgroundColor: color + '22' }}>
        <Ionicon name={icon} size={20} color={color} />
      </View>
      <View className="flex-1">
        <Text className="text-xl font-bold text-foreground" numberOfLines={1}>
          {value}
        </Text>
        <Text className="text-xs text-muted">{label}</Text>
      </View>
    </View>
  );
}
