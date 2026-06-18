import { useEffect, useState } from 'react';
import { Modal, Platform, Pressable, ScrollView, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useAuth } from '../../src/auth/store';
import { useProviderMutations, useProviders, useSettings, useUpdateSettings } from '../../src/query/admin';
import { Provider } from '../../src/api/immerle/types';
import { Badge, Button, Card, EmptyState, ErrorState, Field, IconButton, Loading, SectionHeader } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';
import { tError } from '../../src/i18n';
import { useT } from '../../src/i18n/store';

const SLUG_RE = /^[a-z0-9][a-z0-9_-]*$/;

const byOrder = (a: Provider, b: Provider) =>
  a.sortOrder - b.sortOrder || a.name.localeCompare(b.name);

/**
 * Dynamic providers admin (web only). A provider is content-neutral: a name, an
 * HTTP endpoint and an opaque JSON config the server calls for search/resolve/
 * download. Admins create, edit, toggle, reorder and delete them; changes apply
 * live. Built-in providers (from server config) can be toggled and reordered
 * but never deleted or redefined. Order = priority (drives search fallback).
 */
export default function AdminProviders() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const q = useProviders();
  const { reorder } = useProviderMutations();
  const [editing, setEditing] = useState<Provider | 'new' | null>(null);
  const [behaviourOpen, setBehaviourOpen] = useState(false);
  const hasSettings = !!client?.has('runtimeSettings');

  // Admin surfaces are web-only — skip the heavy editor on native.
  if (Platform.OS !== 'web') {
    return (
      <>
        <Stack.Screen options={{ title: t('admin.providers.title') }} />
        <EmptyState
          icon="desktop-outline"
          title={t('admin.providers.webOnlyTitle')}
          subtitle={t('admin.providers.webOnlySubtitle')}
        />
      </>
    );
  }

  const ordered = [...(q.data ?? [])].sort(byOrder);

  // Swap a provider with its neighbour and persist the full new order.
  const move = (index: number, dir: -1 | 1) => {
    const names = ordered.map((p) => p.name);
    const j = index + dir;
    if (j < 0 || j >= names.length) return;
    [names[index], names[j]] = [names[j], names[index]];
    reorder.mutate(names);
  };

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message={t('admin.providers.loadError')} onRetry={q.refetch} />
      ) : (
        <AdminScroll
          header={
            <AdminHeader
              color="#8b5cf6"
              title={t('admin.providers.title')}
              subtitle={t('admin.providers.headerSubtitle')}
              trailing={
                <View className="flex-row items-center gap-2">
                  <Button title={t('admin.providers.add')} size="sm" icon="add" onPress={() => setEditing('new')} />
                  {hasSettings ? (
                    <Pressable
                      onPress={() => setBehaviourOpen(true)}
                      accessibilityLabel={t('admin.providers.behaviour')}
                      className="h-9 w-9 items-center justify-center rounded-full bg-surface-alt active:opacity-70"
                    >
                      <Ionicon name="settings-outline" size={18} color={colors.foreground} />
                    </Pressable>
                  ) : null}
                </View>
              }
            />
          }
        >
          <SectionHeader title={t('admin.providers.configured')} />
          {!ordered.length ? (
            <EmptyState title={t('admin.providers.emptyTitle')} subtitle={t('admin.providers.emptySubtitle')} />
          ) : (
            ordered.map((p, i) => (
              <ProviderCard
                key={p.name}
                provider={p}
                onEdit={() => setEditing(p)}
                disabledEdit={!!editing}
                onMoveUp={i > 0 ? () => move(i, -1) : undefined}
                onMoveDown={i < ordered.length - 1 ? () => move(i, 1) : undefined}
                reordering={reorder.isPending}
              />
            ))
          )}

          {editing ? (
            <ProviderModal initial={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />
          ) : null}
          <BehaviourModal visible={behaviourOpen} onClose={() => setBehaviourOpen(false)} />
        </AdminScroll>
      )}
    </>
  );
}

/** Global provider behaviour (runtime settings), in a popin opened from the
 * header gear. All fields hot-reload on the server. */
function BehaviourModal({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const q = useSettings();
  const update = useUpdateSettings();
  const [timeoutS, setTimeoutS] = useState('');
  const [auto, setAuto] = useState(false);

  // Re-sync from the server each time the popin opens.
  useEffect(() => {
    const p = q.data?.settings.providers;
    if (!p || !visible) return;
    setTimeoutS(String(p.searchTimeoutSeconds ?? 0));
    setAuto(p.autoDownloadOnPlay ?? false);
  }, [q.data?.settings.providers, visible]);

  const save = () =>
    update.mutate(
      {
        providers: {
          searchTimeoutSeconds: Number(timeoutS) || 0,
          autoDownloadOnPlay: auto,
        },
      },
      { onSuccess: onClose },
    );

  return (
    <Modal transparent animationType="fade" visible={visible} onRequestClose={onClose}>
      <Pressable className="flex-1 items-center justify-center bg-black/60 px-6" onPress={onClose}>
        <Pressable className="w-full max-w-[440px] gap-3 rounded-2xl bg-surface p-5" onPress={(e) => e.stopPropagation()}>
          <View className="flex-row items-center justify-between">
            <Text className="text-lg font-bold tracking-tight text-foreground">{t('admin.providers.behaviour')}</Text>
            <IconButton name="close" color={colors.muted} onPress={onClose} accessibilityLabel={t('admin.providers.close')} />
          </View>
          <Text className="text-sm text-muted">
            {t('admin.providers.behaviourDescription')}
          </Text>
          <Field label={t('admin.providers.searchTimeout')} keyboardType="number-pad" value={timeoutS} onChangeText={setTimeoutS} />
          <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
            <Text className="flex-1 pr-2 text-sm text-foreground">{t('admin.providers.autoDownload')}</Text>
            <Switch value={auto} onValueChange={setAuto} trackColor={{ true: colors.primary, false: colors.border }} />
          </View>
          <View className="flex-row gap-2 pt-1">
            <View className="flex-1">
              <Button title={t('admin.providers.cancel')} variant="ghost" onPress={onClose} />
            </View>
            <View className="flex-1">
              <Button title={t('admin.providers.save')} icon="save-outline" loading={update.isPending} onPress={save} />
            </View>
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

function ProviderCard({
  provider,
  onEdit,
  disabledEdit,
  onMoveUp,
  onMoveDown,
  reordering,
}: {
  provider: Provider;
  onEdit: () => void;
  disabledEdit: boolean;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  reordering: boolean;
}) {
  const t = useT();
  const colors = useColors();
  const { setEnabled } = useProviderMutations();

  return (
    <Card>
      <View className="flex-row items-center gap-3">
        <View className="gap-0.5">
          <ArrowButton icon="chevron-up" onPress={onMoveUp} disabled={!onMoveUp || reordering} />
          <ArrowButton icon="chevron-down" onPress={onMoveDown} disabled={!onMoveDown || reordering} />
        </View>
        <View className="h-10 w-10 items-center justify-center rounded-xl bg-primary/15">
          <Ionicon name={provider.builtin ? 'cube' : 'cube-outline'} size={20} color={colors.primary} />
        </View>

        <View className="flex-1 gap-1.5">
          <Text className="text-base font-semibold text-foreground">{provider.name}</Text>
          {/* Status pills, right under the title */}
          <View className="flex-row flex-wrap items-center gap-1.5">
            <Badge label={provider.enabled ? t('admin.providers.enabled') : t('admin.providers.disabled')} tone={provider.enabled ? 'success' : 'default'} />
            {provider.active ? (
              <Badge label={t('admin.providers.online')} tone="primary" />
            ) : provider.enabled ? (
              <Badge label={t('admin.providers.inactive')} tone="danger" />
            ) : null}
            {provider.builtin ? <Badge label={t('admin.providers.builtin')} tone="default" /> : null}
          </View>
          <Text className="text-xs text-muted" numberOfLines={1}>
            {provider.kind} · {provider.endpoint || '—'}
          </Text>
        </View>

        <Switch
          value={provider.enabled}
          disabled={setEnabled.isPending}
          onValueChange={(v) => setEnabled.mutate({ name: provider.name, enabled: v })}
          trackColor={{ true: colors.primary, false: colors.border }}
        />

        {/* Settings opens the editor — built-ins can't be edited. */}
        {!provider.builtin ? (
          <Pressable
            onPress={onEdit}
            disabled={disabledEdit}
            accessibilityLabel={t('admin.providers.settings')}
            className={`h-9 w-9 items-center justify-center rounded-full bg-surface-alt ${disabledEdit ? 'opacity-40' : 'active:opacity-70'}`}
          >
            <Ionicon name="settings-outline" size={18} color={colors.foreground} />
          </Pressable>
        ) : null}
      </View>
    </Card>
  );
}

function ArrowButton({ icon, onPress, disabled }: { icon: string; onPress?: () => void; disabled: boolean }) {
  const colors = useColors();
  return (
    <Pressable
      onPress={onPress}
      disabled={disabled}
      className={`h-7 w-7 items-center justify-center rounded-lg border border-border ${disabled ? 'opacity-30' : 'active:bg-surface-alt'}`}
    >
      <Ionicon name={icon} size={16} color={colors.foreground} />
    </Pressable>
  );
}

function ProviderModal({ initial, onClose }: { initial: Provider | null; onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const { upsert, remove } = useProviderMutations();
  const isEdit = !!initial;
  const [name, setName] = useState(initial?.name ?? '');
  const [endpoint, setEndpoint] = useState(initial?.endpoint ?? '');
  const [config, setConfig] = useState(initial?.config ?? '{}');
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  const validate = (): string | null => {
    if (!SLUG_RE.test(name)) return t('admin.providers.invalidName');
    if (!/^https?:\/\/.+/.test(endpoint)) return t('admin.providers.invalidEndpoint');
    const trimmed = config.trim();
    if (trimmed) {
      try {
        JSON.parse(trimmed);
      } catch {
        return t('admin.providers.invalidConfig');
      }
    }
    return null;
  };

  const submit = () => {
    const err = validate();
    if (err) {
      setError(err);
      return;
    }
    setError(null);
    upsert.mutate(
      { name, endpoint, config: config.trim() || '{}', enabled, kind: 'http' },
      { onSuccess: onClose, onError: (e) => setError(tError(e)) },
    );
  };

  return (
    <Modal transparent animationType="fade" visible onRequestClose={onClose}>
      <Pressable className="flex-1 items-center justify-center bg-black/60 px-6" onPress={onClose}>
        <Pressable className="w-full max-w-[460px] overflow-hidden rounded-2xl bg-surface" onPress={(e) => e.stopPropagation()}>
          <View className="flex-row items-center justify-between px-5 pb-2 pt-5">
            <Text className="text-lg font-bold tracking-tight text-foreground">
              {isEdit ? t('admin.providers.editTitle', { name: initial?.name }) : t('admin.providers.newTitle')}
            </Text>
            <IconButton name="close" color={colors.muted} onPress={onClose} accessibilityLabel={t('admin.providers.close')} />
          </View>

          <ScrollView
            style={{ maxHeight: 460 }}
            contentContainerStyle={{ paddingHorizontal: 20, paddingBottom: 20, gap: 12 }}
            keyboardShouldPersistTaps="handled"
          >
            <Field
              label={t('admin.providers.nameLabel')}
              placeholder="mon-service"
              autoCapitalize="none"
              autoCorrect={false}
              editable={!isEdit}
              help={isEdit ? t('admin.providers.nameHelpLocked') : t('admin.providers.nameHelp')}
              value={name}
              onChangeText={setName}
            />
            <Field
              label={t('admin.providers.endpointLabel')}
              placeholder="https://mon-service.internal"
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              value={endpoint}
              onChangeText={setEndpoint}
            />
            <Field
              label={t('admin.providers.configLabel')}
              placeholder='{"headers":{"Authorization":"Bearer …"}}'
              autoCapitalize="none"
              autoCorrect={false}
              multiline
              numberOfLines={5}
              style={{ minHeight: 110, textAlignVertical: 'top' }}
              help={t('admin.providers.configHelp')}
              value={config}
              onChangeText={setConfig}
            />
            <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
              <Text className="text-sm font-medium text-foreground">{t('admin.providers.enableNow')}</Text>
              <Switch value={enabled} onValueChange={setEnabled} trackColor={{ true: colors.primary, false: colors.border }} />
            </View>

            {error ? <Text className="text-xs text-danger">{error}</Text> : null}

            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title={t('admin.providers.cancel')} variant="ghost" onPress={onClose} />
              </View>
              <View className="flex-1">
                <Button
                  title={isEdit ? t('admin.providers.save') : t('admin.providers.create')}
                  icon="save-outline"
                  loading={upsert.isPending}
                  onPress={submit}
                />
              </View>
            </View>

            {/* Delete lives here (built-ins are never deletable). */}
            {isEdit && initial?.deletable ? (
              confirming ? (
                <Button
                  title={t('admin.providers.confirmDelete')}
                  icon="trash"
                  variant="danger"
                  loading={remove.isPending}
                  onPress={() => remove.mutate(initial.name, { onSuccess: onClose })}
                />
              ) : (
                <Button title={t('admin.providers.delete')} icon="trash-outline" variant="danger" onPress={() => setConfirming(true)} />
              )
            ) : null}
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
