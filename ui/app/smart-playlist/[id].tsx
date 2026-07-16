import { useEffect, useState } from 'react';
import { Text, View } from 'react-native';
import { Stack, router, useLocalSearchParams } from 'expo-router';
import { useSmartPlaylists, useSmartPlaylistMutations } from '../../src/query/smartPlaylists';
import { useAuth } from '../../src/auth/store';
import { usePlayer } from '../../src/audio/store';
import { SmartCondition, SmartRules } from '../../src/api/immerle/types';
import { Song } from '../../src/api/subsonic/types';
import { AdminScroll, AdminHeader, CardTitle } from '../../src/components/AdminUI';
import { Button, Card, Field, IconButton, Loading, Select } from '../../src/components/ui';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

type Kind = 'text' | 'number' | 'bool' | 'none' | 'days';

const FIELDS: { value: string; kind: Kind }[] = [
  { value: 'genre', kind: 'text' },
  { value: 'artist', kind: 'text' },
  { value: 'album', kind: 'text' },
  { value: 'title', kind: 'text' },
  { value: 'year', kind: 'number' },
  { value: 'bpm', kind: 'number' },
  { value: 'playCount', kind: 'number' },
  { value: 'rating', kind: 'number' },
  { value: 'starred', kind: 'bool' },
  { value: 'neverPlayed', kind: 'none' },
  { value: 'addedWithinDays', kind: 'days' },
  { value: 'playedWithinDays', kind: 'days' },
];
const OPS_BY_KIND: Record<Kind, string[]> = {
  text: ['is', 'isNot', 'contains'],
  number: ['is', 'gte', 'lte'],
  bool: [],
  none: [],
  days: [],
};
const SORTS = ['none', 'random', 'playCount', 'recentlyAdded', 'recentlyPlayed', 'year', 'title', 'bpm'];

const kindOf = (field: string): Kind => FIELDS.find((f) => f.value === field)?.kind ?? 'text';

/** Create or edit a rule-based smart playlist. id === 'new' creates one. */
export default function SmartPlaylistEditor() {
  const t = useT();
  const colors = useColors();
  const { id } = useLocalSearchParams<{ id: string }>();
  const isNew = id === 'new';
  const client = useAuth((s) => s.client);
  const playSongs = usePlayer((s) => s.playSongs);
  const list = useSmartPlaylists();
  const { create, update } = useSmartPlaylistMutations();

  const [name, setName] = useState('');
  const [match, setMatch] = useState<'all' | 'any'>('all');
  const [conditions, setConditions] = useState<SmartCondition[]>([]);
  const [sort, setSort] = useState('none');
  const [order, setOrder] = useState<'asc' | 'desc'>('desc');
  const [limit, setLimit] = useState('100');
  const [preview, setPreview] = useState<Song[] | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [loaded, setLoaded] = useState(false);

  // Hydrate from the saved playlist when editing (once).
  useEffect(() => {
    if (loaded || isNew) return;
    const sp = list.data?.find((p) => p.id === id);
    if (!sp) return;
    setName(sp.name);
    setMatch(sp.rules?.match === 'any' ? 'any' : 'all');
    setConditions(sp.rules?.conditions ?? []);
    setSort(sp.rules?.sort || 'none');
    setOrder(sp.rules?.order === 'asc' ? 'asc' : 'desc');
    setLimit(String(sp.rules?.limit || 100));
    setLoaded(true);
  }, [loaded, isNew, id, list.data]);

  if (!isNew && list.isLoading && !loaded) return <Loading />;

  const buildRules = (): SmartRules => ({
    match,
    conditions: conditions.filter((c) => c.field),
    sort: sort === 'none' ? '' : sort,
    order,
    limit: Number(limit) || 100,
  });

  const setCond = (i: number, patch: Partial<SmartCondition>) =>
    setConditions((cs) => cs.map((c, j) => (j === i ? { ...c, ...patch } : c)));
  const addCond = () => setConditions((cs) => [...cs, { field: 'genre', op: 'is', value: '' }]);
  const removeCond = (i: number) => setConditions((cs) => cs.filter((_, j) => j !== i));
  const onFieldChange = (i: number, field: string) => {
    const kind = kindOf(field);
    setCond(i, { field, op: OPS_BY_KIND[kind][0] ?? '', value: kind === 'bool' ? 'true' : '' });
  };

  const runPreview = async () => {
    if (!client) return;
    setPreviewing(true);
    try {
      setPreview(await client.previewSmartPlaylist(buildRules()));
    } finally {
      setPreviewing(false);
    }
  };

  const onSave = () => {
    const rules = buildRules();
    const finalName = name.trim() || t('smart.untitled');
    const after = () => router.back();
    if (isNew) create.mutate({ name: finalName, rules }, { onSuccess: after });
    else update.mutate({ id: id!, name: finalName, rules }, { onSuccess: after });
  };

  const fieldOpts = FIELDS.map((f) => ({ value: f.value, label: t(`smart.field.${f.value}`) }));

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color={colors.primary} title={isNew ? t('smart.newTitle') : t('smart.editTitle')} subtitle={t('smart.editSubtitle')} />}>
        <Card className="gap-3">
          <Field label={t('smart.name')} placeholder={t('smart.namePlaceholder')} value={name} onChangeText={setName} />
        </Card>

        <Card className="gap-3">
          <CardTitle icon="funnel" color="#3b82f6" title={t('smart.conditions')} />
          <View className="flex-row items-center gap-2">
            <Text className="text-sm text-muted">{t('smart.matchLabel')}</Text>
            <View className="flex-1">
              <Select<'all' | 'any'>
                value={match}
                options={[
                  { value: 'all', label: t('smart.matchAll') },
                  { value: 'any', label: t('smart.matchAny') },
                ]}
                onChange={setMatch}
              />
            </View>
          </View>

          {conditions.map((c, i) => {
            const kind = kindOf(c.field);
            return (
              <View key={i} className="gap-2 rounded-xl bg-surface-alt p-3">
                <View className="flex-row items-center gap-2">
                  <View className="flex-1">
                    <Select<string> value={c.field} options={fieldOpts} onChange={(v) => onFieldChange(i, v)} />
                  </View>
                  <IconButton name="close" color={colors.muted} onPress={() => removeCond(i)} accessibilityLabel={t('smart.removeCondition')} />
                </View>
                {OPS_BY_KIND[kind].length > 0 ? (
                  <Select<string>
                    value={c.op}
                    options={OPS_BY_KIND[kind].map((op) => ({ value: op, label: t(`smart.op.${op}`) }))}
                    onChange={(v) => setCond(i, { op: v })}
                  />
                ) : null}
                {kind === 'text' || kind === 'number' ? (
                  <Field
                    placeholder={t('smart.valuePlaceholder')}
                    keyboardType={kind === 'number' ? 'number-pad' : 'default'}
                    value={c.value}
                    onChangeText={(v) => setCond(i, { value: v })}
                  />
                ) : null}
                {kind === 'days' ? (
                  <Field label={t('smart.days')} keyboardType="number-pad" value={c.value} onChangeText={(v) => setCond(i, { value: v })} />
                ) : null}
                {kind === 'bool' ? (
                  <Select<string>
                    value={c.value || 'true'}
                    options={[
                      { value: 'true', label: t('smart.yes') },
                      { value: 'false', label: t('smart.no') },
                    ]}
                    onChange={(v) => setCond(i, { value: v })}
                  />
                ) : null}
              </View>
            );
          })}
          <Button title={t('smart.addCondition')} variant="secondary" icon="add" onPress={addCond} />
        </Card>

        <Card className="gap-3">
          <CardTitle icon="swap-vertical" color="#a855f7" title={t('smart.sortTitle')} />
          <Select<string> value={sort} options={SORTS.map((s) => ({ value: s, label: t(`smart.sort.${s}`) }))} onChange={setSort} />
          {sort !== 'none' && sort !== 'random' ? (
            <Select<'asc' | 'desc'>
              value={order}
              options={[
                { value: 'desc', label: t('smart.orderDesc') },
                { value: 'asc', label: t('smart.orderAsc') },
              ]}
              onChange={setOrder}
            />
          ) : null}
          <Field label={t('smart.limit')} keyboardType="number-pad" value={limit} onChangeText={setLimit} />
        </Card>

        <Card className="gap-3">
          <View className="flex-row items-center gap-2">
            <Button title={t('smart.preview')} variant="secondary" icon="eye-outline" loading={previewing} onPress={runPreview} />
            {preview ? (
              <>
                <Text className="text-sm text-muted">{t('smart.previewCount', { count: preview.length })}</Text>
                {preview.length ? <Button title={t('smart.play')} size="sm" icon="play" onPress={() => playSongs(preview, 0)} /> : null}
              </>
            ) : null}
          </View>
        </Card>

        <Button title={t('smart.save')} icon="save-outline" loading={create.isPending || update.isPending} onPress={onSave} />
      </AdminScroll>
    </>
  );
}
