import { useState } from 'react';
import { Modal, Pressable, ScrollView, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useRadioStations, useRadioMutations } from '../../src/query/radio';
import { useAuth } from '../../src/auth/store';
import { RadioStation } from '../../src/api/immerle/types';
import { Button, Card, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { StationCover } from '../../src/components/StationCover';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

interface Draft {
  id?: string;
  name: string;
  streamUrl: string;
  homepageUrl: string;
  coverUrl: string;
}

const EMPTY: Draft = { name: '', streamUrl: '', homepageUrl: '', coverUrl: '' };

/** Admin: manage internet radio stations (add custom, edit any, delete custom). */
export default function AdminRadio() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const q = useRadioStations();
  const { create, update, remove } = useRadioMutations();
  const [draft, setDraft] = useState<Draft | null>(null);

  if (q.isLoading) return <Loading />;
  if (q.isError) return <ErrorState message={t('radio.loadError')} onRetry={q.refetch} />;

  const valid = !!draft && draft.name.trim() !== '' && /^https?:\/\//.test(draft.streamUrl.trim());
  const save = () => {
    if (!draft || !valid) return;
    const body = { name: draft.name.trim(), streamUrl: draft.streamUrl.trim(), homepageUrl: draft.homepageUrl.trim(), coverUrl: draft.coverUrl.trim() };
    const after = () => setDraft(null);
    if (draft.id) update.mutate({ id: draft.id, ...body }, { onSuccess: after });
    else create.mutate(body, { onSuccess: after });
  };

  const custom = (q.data ?? []).filter((s) => !s.builtin);

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll header={<AdminHeader color={colors.primary} title={t('radio.manageTitle')} subtitle={t('radio.manageSubtitle')} />}>
        <Button title={t('radio.add')} icon="add" onPress={() => setDraft({ ...EMPTY })} />
        {/* Built-in stations are server-managed and not editable here. */}
        {custom.length === 0 ? (
          <Text className="px-1 pt-2 text-sm text-muted">{t('radio.noCustom')}</Text>
        ) : (
          custom.map((s: RadioStation) => (
            <Card key={s.id} className="flex-row items-center gap-3">
              <StationCover uri={s.hasCover && client ? client.radioCoverUrl(s.id) : undefined} size={40} rounded={10} />
              <View className="flex-1">
                <Text numberOfLines={1} className="text-base font-semibold text-foreground">{s.name}</Text>
                <Text numberOfLines={1} className="text-xs text-muted">{s.streamUrl}</Text>
              </View>
              <IconButton name="create-outline" color={colors.muted} onPress={() => setDraft({ id: s.id, name: s.name, streamUrl: s.streamUrl, homepageUrl: s.homepageUrl, coverUrl: s.coverUrl ?? '' })} accessibilityLabel={t('radio.edit')} />
              <IconButton name="trash-outline" color={colors.danger} onPress={() => remove.mutate(s.id)} accessibilityLabel={t('radio.delete')} />
            </Card>
          ))
        )}
      </AdminScroll>

      <Modal transparent visible={!!draft} animationType="slide" onRequestClose={() => setDraft(null)}>
        <Pressable className="flex-1 justify-end bg-black/50" onPress={() => setDraft(null)}>
          <Pressable className="max-h-[85%] rounded-t-3xl bg-surface pb-6 pt-2" onPress={(e) => e.stopPropagation()}>
            <View className="mb-1 items-center pt-1">
              <View className="h-1 w-10 rounded-full bg-border" />
            </View>
            <Text className="px-5 pb-2 pt-1 text-lg font-bold text-foreground">{draft?.id ? t('radio.editTitle') : t('radio.addTitle')}</Text>
            <ScrollView contentContainerStyle={{ paddingHorizontal: 20, paddingTop: 4, paddingBottom: 8, gap: 12 }} keyboardShouldPersistTaps="handled">
              <Field label={t('radio.name')} value={draft?.name ?? ''} onChangeText={(v) => setDraft((d) => (d ? { ...d, name: v } : d))} />
              <Field label={t('radio.streamUrl')} autoCapitalize="none" autoCorrect={false} keyboardType="url" placeholder="https://…" value={draft?.streamUrl ?? ''} onChangeText={(v) => setDraft((d) => (d ? { ...d, streamUrl: v } : d))} />
              <Field label={t('radio.homepageUrl')} autoCapitalize="none" autoCorrect={false} keyboardType="url" value={draft?.homepageUrl ?? ''} onChangeText={(v) => setDraft((d) => (d ? { ...d, homepageUrl: v } : d))} />
              <Field
                label={t('radio.logoUrl')}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                placeholder="https://…/logo.png"
                value={draft?.coverUrl ?? ''}
                onChangeText={(v) => setDraft((d) => (d ? { ...d, coverUrl: v } : d))}
                help={t('radio.logoUrlHelp')}
              />
              <Button title={t('radio.save')} icon="save-outline" disabled={!valid} loading={create.isPending || update.isPending} onPress={save} />
            </ScrollView>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}
