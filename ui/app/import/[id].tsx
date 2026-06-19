import { useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useImportStatus, useResolveImportItem } from '../../src/query/imports';
import { Badge, Button, Card, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { CoverArt } from '../../src/components/CoverArt';
import { Ionicon } from '../../src/components/Ionicon';
import { usePlayer } from '../../src/audio/store';
import { ImportItemDTO } from '../../src/api/immerleApi';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { useWebTitle } from '../../src/utils/documentTitle';

type Filter = 'all' | 'matched' | 'doubtful' | 'missing' | 'failed';

const ITEM_TONE: Record<string, 'success' | 'primary' | 'danger' | 'default'> = {
  matched: 'success',
  doubtful: 'primary',
  missing: 'default',
  failed: 'danger',
};

/** Import detail: per-track items, filterable by status to review the doubtful
 * and missing matches. */
export default function ImportDetail() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const q = useImportStatus(id ?? '');
  const [filter, setFilter] = useState<Filter>('all');
  useWebTitle(q.data?.sourcePlaylistName);

  if (q.isLoading) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <Loading />
      </>
    );
  }
  if (q.isError || !q.data) {
    return (
      <>
        <Stack.Screen options={{ headerShown: false }} />
        <ErrorState message={t('tools.importDetail.notFound')} onRetry={q.refetch} />
      </>
    );
  }

  const imp = q.data;
  const items = imp.items ?? [];
  const counts = {
    all: items.length,
    matched: imp.matched ?? 0,
    doubtful: imp.doubtful ?? 0,
    missing: imp.missing ?? 0,
    failed: imp.failed ?? 0,
  };
  const shown = filter === 'all' ? items : items.filter((i) => i.status === filter);

  const FILTERS: { key: Filter; label: string }[] = [
    { key: 'all', label: t('tools.importDetail.filterAll', { count: counts.all }) },
    { key: 'doubtful', label: t('tools.importDetail.filterDoubtful', { count: counts.doubtful }) },
    { key: 'missing', label: t('tools.importDetail.filterMissing', { count: counts.missing }) },
    { key: 'matched', label: t('tools.importDetail.filterMatched', { count: counts.matched }) },
    ...(counts.failed ? [{ key: 'failed' as Filter, label: t('tools.importDetail.filterFailed', { count: counts.failed }) }] : []),
  ];

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={
          <AdminHeader
            color="#1db954"
            title={imp.sourcePlaylistName || t('tools.importDetail.fallbackName')}
            subtitle={`${imp.source ?? ''} · ${t('tools.importDetail.matchedCount', { matched: counts.matched, total: imp.total ?? 0 })}`}
          />
        }
      >
        {imp.playlistId ? (
          <Button
            title={t('tools.importDetail.openPlaylist')}
            icon="open-outline"
            variant="secondary"
            onPress={() => router.push(`/playlist/${imp.playlistId}` as never)}
          />
        ) : null}

        {/* Status filter */}
        <View className="flex-row flex-wrap gap-2">
          {FILTERS.map((f) => {
            const active = filter === f.key;
            return (
              <Pressable
                key={f.key}
                onPress={() => setFilter(f.key)}
                className={`rounded-full px-3 py-1.5 ${active ? 'bg-primary/15' : 'bg-surface-alt'}`}
              >
                <Text className={`text-xs ${active ? 'font-semibold text-primary' : 'text-muted'}`}>{f.label}</Text>
              </Pressable>
            );
          })}
        </View>

        {/* Hint about doubtful items (no validate/modify API yet). */}
        {filter === 'doubtful' && counts.doubtful > 0 ? (
          <View className="flex-row items-start gap-2 rounded-xl bg-surface-alt p-3">
            <Ionicon name="information-circle" size={18} color={colors.muted} />
            <Text className="flex-1 text-xs text-muted">
              {t('tools.importDetail.doubtfulHint')}
            </Text>
          </View>
        ) : null}

        <Card className="p-0">
          {shown.length ? (
            shown.map((it, i) => <ItemRow key={it.id ?? i} importId={id ?? ''} item={it} first={i === 0} />)
          ) : (
            <Text className="px-4 py-4 text-sm text-muted">{t('tools.importDetail.emptyCategory')}</Text>
          )}
        </Card>
      </AdminScroll>
    </>
  );
}

function ItemRow({ importId, item, first }: { importId: string; item: ImportItemDTO; first: boolean }) {
  const t = useT();
  const colors = useColors();
  const status = item.status ?? '';
  const ITEM_LABEL: Record<string, string> = {
    matched: t('tools.importDetail.itemMatched'),
    doubtful: t('tools.importDetail.itemDoubtful'),
    missing: t('tools.importDetail.itemMissing'),
    failed: t('tools.importDetail.itemFailed'),
  };
  const resolve = useResolveImportItem();
  const playTrackById = usePlayer((s) => s.playTrackById);
  const [editing, setEditing] = useState(false);
  const [query, setQuery] = useState('');

  const resolved = item.resolvedTitle ? `${item.resolvedTitle}${item.resolvedArtist ? ` · ${item.resolvedArtist}` : ''}` : null;
  const confidence = item.confidence != null ? Math.round(item.confidence * 100) : null;
  const isMatched = status === 'matched';
  const hasCandidate = !!item.resolvedTitle || status === 'doubtful';
  // Matched → the library track; doubtful → the (remote) candidate, for preview.
  const coverId = item.matchedTrackId ?? item.candidateCoverArt;
  const playId = item.matchedTrackId ?? item.candidateTrackId;

  const startEdit = () => {
    setQuery(`${item.sourceArtist ?? ''} ${item.sourceTitle ?? ''}`.trim());
    setEditing(true);
  };
  const validate = () => item.id && resolve.mutate({ importId, itemId: item.id });
  const modify = () => {
    if (item.id && query.trim())
      resolve.mutate({ importId, itemId: item.id, query: query.trim() }, { onSuccess: () => setEditing(false) });
  };

  return (
    <View className={`gap-1 px-4 py-2.5 ${first ? '' : 'border-t border-border'}`}>
      <View className="flex-row items-center gap-3">
        <CoverArt coverArt={coverId} size={40} rounded="rounded-md" fallbackIcon="musical-notes" />
        <View className="flex-1 gap-0.5">
          <View className="flex-row items-center gap-2">
            <Text className="flex-1 text-sm font-medium text-foreground" numberOfLines={1}>
              {item.sourceTitle || '—'}
              {item.sourceArtist ? <Text className="font-normal text-muted"> · {item.sourceArtist}</Text> : null}
            </Text>
            {playId ? (
              <IconButton
                name="play-circle"
                size={22}
                color={colors.primary}
                onPress={() => playTrackById(playId, 0, true)}
                accessibilityLabel={t('tools.importDetail.listen')}
              />
            ) : null}
            <Badge
              label={`${ITEM_LABEL[status] ?? status}${status === 'doubtful' && confidence != null ? ` ${confidence}%` : ''}`}
              tone={ITEM_TONE[status] ?? 'default'}
            />
          </View>
          {resolved ? (
            <Text className="text-xs text-muted" numberOfLines={1}>
              → {resolved}
            </Text>
          ) : status === 'missing' ? (
            <Text className="text-xs text-muted">{t('tools.importDetail.noMatch')}</Text>
          ) : null}
          {item.note ? <Text className="text-xs text-muted">{item.note}</Text> : null}
        </View>
      </View>

      {/* Validate / correct flagged items */}
      {!isMatched && item.id ? (
        editing ? (
          <View className="gap-2 pt-1">
            <Field
              placeholder={t('tools.importDetail.searchPlaceholder')}
              autoCapitalize="none"
              autoCorrect={false}
              value={query}
              onChangeText={setQuery}
              onSubmitEditing={modify}
            />
            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('tools.importDetail.cancel')} size="sm" variant="ghost" onPress={() => setEditing(false)} />
              </View>
              <View className="flex-1">
                <Button title={t('tools.importDetail.searchAdd')} size="sm" icon="search" loading={resolve.isPending} onPress={modify} />
              </View>
            </View>
          </View>
        ) : (
          <View className="flex-row gap-2 pt-1">
            {hasCandidate ? (
              <View className="flex-1">
                <Button title={t('tools.importDetail.validate')} size="sm" icon="checkmark" loading={resolve.isPending} onPress={validate} />
              </View>
            ) : null}
            <View className="flex-1">
              <Button title={t('tools.importDetail.correct')} size="sm" variant="secondary" icon="create-outline" onPress={startEdit} />
            </View>
          </View>
        )
      ) : null}
    </View>
  );
}
