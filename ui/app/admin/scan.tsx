import { useEffect, useState } from 'react';
import { Alert, Platform, Pressable, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import * as ImagePicker from 'expo-image-picker';
import {
  useLibraryStats,
  useScanProgress,
  useSettings,
  useStartScan,
  useUpdateSettings,
} from '../../src/query/admin';
import { useAdminTracks, useSonicDuplicates, useTrackMutations } from '../../src/query/tracks';
import { useAuth } from '../../src/auth/store';
import { Song } from '../../src/api/subsonic/types';
import { Badge, Button, Card, ErrorState, Field, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle } from '../../src/components/AdminUI';
import { CoverArt } from '../../src/components/CoverArt';
import { Ionicon } from '../../src/components/Ionicon';
import { formatBytes, formatCount, formatDuration } from '../../src/utils/format';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

const PAGE_SIZE = 50;

/** Library admin: stats, full / incremental scan with live progress, the
 * automatic scan cadence, then the downloaded tracks (edit/delete). */
export default function AdminScan() {
  const t = useT();
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
        header={<AdminHeader color="#f59e0b" title={t('admin.scan.title')} subtitle={t('admin.scan.headerSubtitle')} />}
      >
        <View className="flex-row flex-wrap gap-2.5">
          <StatTile icon="people" color="#3b82f6" label={t('admin.scan.artists')} value={formatCount(stats.data?.artistCount)} />
          <StatTile icon="albums" color="#8b5cf6" label={t('admin.scan.albums')} value={formatCount(stats.data?.albumCount)} />
          <StatTile icon="musical-notes" color="#1ed760" label={t('admin.scan.songs')} value={formatCount(stats.data?.songCount)} />
          <StatTile icon="server" color="#f59e0b" label={t('admin.scan.space')} value={stats.data ? formatBytes(stats.data.totalSize) : '—'} />
        </View>
        {stats.data?.lastScan ? (
          <Text className="px-1 text-xs text-muted">{t('admin.scan.lastScan', { date: stats.data.lastScan })}</Text>
        ) : null}

        <Card className="gap-3">
          <CardTitle
            icon={scanning ? 'sync' : 'checkmark-circle'}
            color={scanning ? '#f59e0b' : colors.success}
            title={scanning ? t('admin.scan.scanning') : t('admin.scan.upToDate')}
          />

          {scanning ? (
            <View className="gap-1.5">
              <View className="h-2 w-full overflow-hidden rounded-full bg-surface-alt">
                <View className="h-full rounded-full bg-primary" style={{ width: pct != null ? `${pct}%` : '40%' }} />
              </View>
              <Text className="text-xs text-muted">
                {t('admin.scan.itemsProcessed', { value: formatCount(progress.data?.count) })}
                {pct != null ? ` · ${Math.round(pct)}%` : ''}
                {progress.data?.phase ? ` · ${progress.data.phase}` : ''}
              </Text>
            </View>
          ) : (
            <Text className="text-sm text-muted">
              {t('admin.scan.idleDescription')}
            </Text>
          )}

          <View className="flex-row gap-2 pt-1">
            <View className="flex-1">
              <Button
                title={t('admin.scan.incremental')}
                icon="refresh"
                variant="secondary"
                disabled={scanning}
                loading={startScan.isPending && !startScan.variables}
                onPress={() => startScan.mutate(false)}
              />
            </View>
            <View className="flex-1">
              <Button
                title={t('admin.scan.full')}
                icon="refresh-circle"
                disabled={scanning}
                loading={startScan.isPending && startScan.variables === true}
                onPress={() => startScan.mutate(true)}
              />
            </View>
          </View>
        </Card>

        <ScanConfigCard />
        <TracksCard />
        <DuplicatesCard />
      </AdminScroll>
    </>
  );
}

/** Automatic scan cadence (runtime settings). Self-hides when the instance
 * doesn't expose the settings endpoint. */
function ScanConfigCard() {
  const t = useT();
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
      <CardTitle icon="time-outline" color="#0ea5e9" title={t('admin.scan.autoScan')} />
      <Field
        label={t('admin.scan.intervalLabel')}
        keyboardType="number-pad"
        value={interval}
        onChangeText={setIntervalS}
      />
      <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
        <View className="flex-1 flex-row items-center gap-2 pr-2">
          <Text className="text-sm text-foreground">{t('admin.scan.watchFiles')}</Text>
          <Badge label={t('admin.scan.restart')} tone="default" />
        </View>
        <Switch value={watch} onValueChange={setWatch} trackColor={{ true: colors.primary, false: colors.border }} />
      </View>
      {pendingWatch ? (
        <Text className="text-xs text-danger">{t('admin.scan.restartNotice')}</Text>
      ) : null}
      <View className="flex-row justify-end pt-1">
        <Button title={t('admin.scan.save')} size="sm" icon="save-outline" loading={update.isPending} onPress={save} />
      </View>
    </Card>
  );
}

/** Downloaded tracks: search, edit metadata/cover, delete. Self-hides when the
 * instance doesn't expose library admin. */
function TracksCard() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);
  const q = useAdminTracks(query, PAGE_SIZE, page * PAGE_SIZE);
  const [expanded, setExpanded] = useState<string | null>(null);

  if (!client?.has('libraryAdmin')) return null;

  const tracks = q.data?.tracks ?? [];
  const total = q.data?.total ?? 0;
  const from = total === 0 ? 0 : page * PAGE_SIZE + 1;
  const to = Math.min((page + 1) * PAGE_SIZE, total);

  const onSearch = (v: string) => {
    setQuery(v);
    setPage(0);
  };

  return (
    <Card className="gap-3">
      <CardTitle icon="musical-notes" color="#1ed760" title={t('admin.tracks.title')} />
      <Text className="text-xs text-muted">{t('admin.tracks.trackCount', { count: total })}</Text>
      <Field
        label={t('admin.tracks.searchLabel')}
        placeholder={t('admin.tracks.searchPlaceholder')}
        autoCapitalize="none"
        value={query}
        onChangeText={onSearch}
      />

      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message={t('admin.tracks.loadError')} onRetry={q.refetch} />
      ) : tracks.length === 0 ? (
        <Text className="py-8 text-center text-muted">{t('admin.tracks.empty')}</Text>
      ) : (
        <View className="overflow-hidden rounded-xl bg-surface-alt">
          {tracks.map((track, i) => (
            <TrackRow
              key={track.id}
              track={track}
              first={i === 0}
              expanded={expanded === track.id}
              onToggle={() => setExpanded((cur) => (cur === track.id ? null : track.id))}
            />
          ))}
        </View>
      )}

      {total > PAGE_SIZE ? (
        <View className="flex-row items-center justify-between pt-1">
          <Button
            title={t('admin.tracks.prev')}
            size="sm"
            variant="secondary"
            icon="chevron-back"
            disabled={page === 0}
            onPress={() => setPage((p) => Math.max(0, p - 1))}
          />
          <Text className="text-xs text-muted">{t('admin.tracks.range', { from, to, total })}</Text>
          <Button
            title={t('admin.tracks.next')}
            size="sm"
            variant="secondary"
            icon="chevron-forward"
            disabled={to >= total}
            onPress={() => setPage((p) => p + 1)}
          />
        </View>
      ) : null}
    </Card>
  );
}

/** The copy worth keeping in a group: best bitrate, then the biggest file. */
function bestCopy(group: Song[]): string | undefined {
  return [...group].sort((a, b) => (b.bitRate ?? 0) - (a.bitRate ?? 0) || (b.size ?? 0) - (a.size ?? 0))[0]?.id;
}

/** Admin: groups of tracks that sound identical, to delete the extra copies. */
function DuplicatesCard() {
  const t = useT();
  const client = useAuth((s) => s.client);
  const [asked, setAsked] = useState(false);
  const q = useSonicDuplicates(asked);
  const { remove } = useTrackMutations();

  if (!client?.isFeatureEnabled('sonicDuplicates')) return null;

  const confirmDelete = (track: Song) => {
    const doDelete = () => remove.mutate(track.id);
    if (Platform.OS === 'web') doDelete();
    else
      Alert.alert(t('admin.tracks.deleteConfirmTitle'), track.title, [
        { text: t('admin.tracks.cancel'), style: 'cancel' },
        { text: t('admin.tracks.delete'), style: 'destructive', onPress: doDelete },
      ]);
  };

  return (
    <Card className="gap-3">
      <CardTitle icon="copy-outline" color="#f59e0b" title={t('admin.duplicates.title')} />
      <Text className="text-xs text-muted">{t('admin.duplicates.description')}</Text>
      {!asked ? (
        <Button title={t('admin.duplicates.search')} icon="search" variant="secondary" onPress={() => setAsked(true)} />
      ) : q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message={t('admin.duplicates.loadError')} onRetry={q.refetch} />
      ) : !q.data?.length ? (
        <Text className="py-4 text-center text-muted">{t('admin.duplicates.none')}</Text>
      ) : (
        <>
          <Text className="text-xs text-muted">{t('admin.duplicates.groupCount', { count: q.data.length })}</Text>
          {q.data.map((group) => {
            const keep = bestCopy(group);
            return (
              <View key={group.map((s) => s.id).join()} className="overflow-hidden rounded-xl bg-surface-alt">
                {group.map((track, i) => (
                  <View key={track.id} className={`flex-row items-center gap-3 px-3 py-2.5 ${i ? 'border-t border-border' : ''}`}>
                    <CoverArt coverArt={track.coverArt} size={40} rounded="rounded-md" />
                    <View className="flex-1">
                      <Text numberOfLines={1} className="text-sm text-foreground">
                        {track.title} · {track.artist}
                      </Text>
                      <Text numberOfLines={1} className="text-xs text-muted">
                        {[track.album, track.suffix?.toUpperCase(), track.bitRate ? `${track.bitRate} kbps` : '', track.size ? formatBytes(track.size) : '']
                          .filter(Boolean)
                          .join(' · ')}
                      </Text>
                    </View>
                    {track.id === keep ? (
                      <Badge label={t('admin.duplicates.keep')} tone="primary" />
                    ) : (
                      <Button title={t('admin.tracks.delete')} size="sm" icon="trash-outline" variant="danger" loading={remove.isPending && remove.variables === track.id} onPress={() => confirmDelete(track)} />
                    )}
                  </View>
                ))}
              </View>
            );
          })}
        </>
      )}
    </Card>
  );
}

function TrackRow({
  track,
  first,
  expanded,
  onToggle,
}: {
  track: Song;
  first: boolean;
  expanded: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const colors = useColors();
  const { update, uploadCover, remove } = useTrackMutations();

  const [title, setTitle] = useState(track.title);
  const [genre, setGenre] = useState(track.genre ?? '');
  const [year, setYear] = useState(track.year ? String(track.year) : '');
  const [trackNo, setTrackNo] = useState(track.track ? String(track.track) : '');

  const save = () =>
    update.mutate({
      id: track.id,
      edit: {
        title: title.trim(),
        genre: genre.trim(),
        year: Number(year) || 0,
        trackNo: Number(trackNo) || 0,
      },
    });

  const pickCover = async () => {
    if (Platform.OS !== 'web') {
      const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
      if (!perm.granted) return;
    }
    const res = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ['images'],
      quality: 0.9,
    });
    if (res.canceled || !res.assets?.length) return;
    const asset = res.assets[0];
    uploadCover.mutate({ id: track.id, uri: asset.uri, mime: asset.mimeType ?? 'image/jpeg' });
  };

  const confirmDelete = () => {
    const doDelete = () => remove.mutate(track.id);
    if (Platform.OS === 'web') doDelete();
    else
      Alert.alert(t('admin.tracks.deleteConfirmTitle'), track.title, [
        { text: t('admin.tracks.cancel'), style: 'cancel' },
        { text: t('admin.tracks.delete'), style: 'destructive', onPress: doDelete },
      ]);
  };

  return (
    <View className={first ? '' : 'border-t border-border'}>
      <Pressable onPress={onToggle} className="flex-row items-center gap-3 px-3 py-2.5 active:bg-surface-alt">
        <CoverArt coverArt={track.coverArt} size={44} />
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground" numberOfLines={1}>
            {track.title}
          </Text>
          <Text className="text-xs text-muted" numberOfLines={1}>
            {track.artist} · {track.album}
          </Text>
        </View>
        <Text className="text-xs text-muted">{formatDuration(track.duration)}</Text>
        <Ionicon name={expanded ? 'chevron-up' : 'chevron-down'} size={18} color={colors.muted} />
      </Pressable>

      {expanded ? (
        <View className="gap-2 px-3 pb-3">
          <View className="flex-row items-center gap-3">
            <CoverArt coverArt={track.coverArt} size={64} />
            <View className="flex-1">
              <Button
                title={t('admin.tracks.changeCover')}
                size="sm"
                variant="secondary"
                icon="image-outline"
                loading={uploadCover.isPending}
                onPress={pickCover}
              />
            </View>
          </View>

          <Field label={t('admin.tracks.titleLabel')} value={title} onChangeText={setTitle} />
          <Field label={t('admin.tracks.genreLabel')} value={genre} onChangeText={setGenre} />
          <View className="flex-row gap-2">
            <View className="flex-1">
              <Field label={t('admin.tracks.yearLabel')} keyboardType="number-pad" value={year} onChangeText={setYear} />
            </View>
            <View className="flex-1">
              <Field label={t('admin.tracks.trackNoLabel')} keyboardType="number-pad" value={trackNo} onChangeText={setTrackNo} />
            </View>
          </View>

          <View className="flex-row gap-2">
            <View className="flex-1">
              <Button title={t('admin.tracks.save')} size="sm" icon="checkmark" loading={update.isPending} onPress={save} />
            </View>
            <Button title={t('admin.tracks.delete')} size="sm" icon="trash-outline" variant="danger" loading={remove.isPending} onPress={confirmDelete} />
          </View>
        </View>
      ) : null}
    </View>
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
