import { useEffect, useState } from 'react';
import { Modal, Platform, Pressable, ScrollView, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import { useAuth } from '../../src/auth/store';
import { useProviderMutations, useProviders, useSettings, useUpdateSettings } from '../../src/query/admin';
import { Provider } from '../../src/api/gossignol/types';
import { Badge, Button, Card, EmptyState, ErrorState, Field, IconButton, Loading, SectionHeader } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';

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
        <Stack.Screen options={{ title: 'Providers' }} />
        <EmptyState
          icon="desktop-outline"
          title="Disponible sur le web"
          subtitle="La gestion des providers se fait depuis le client web."
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
        <ErrorState message="Impossible de charger les providers." onRetry={q.refetch} />
      ) : (
        <AdminScroll
          header={
            <AdminHeader
              color="#8b5cf6"
              title="Providers"
              subtitle="Sources de contenu dynamiques"
              trailing={
                <View className="flex-row items-center gap-2">
                  <Button title="Ajouter" size="sm" icon="add" onPress={() => setEditing('new')} />
                  {hasSettings ? (
                    <Pressable
                      onPress={() => setBehaviourOpen(true)}
                      accessibilityLabel="Comportement"
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
          <SectionHeader title="Configurés" />
          {!ordered.length ? (
            <EmptyState title="Aucun provider" subtitle="Ajoutez-en un pour commencer." />
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
            <Text className="text-lg font-bold tracking-tight text-foreground">Comportement</Text>
            <IconButton name="close" color={colors.muted} onPress={onClose} accessibilityLabel="Fermer" />
          </View>
          <Text className="text-sm text-muted">
            Réglages globaux de recherche et de téléchargement à la demande. Le provider
            primaire est le premier activé dans l'ordre de la liste.
          </Text>
          <Field label="Timeout de recherche (s)" keyboardType="number-pad" value={timeoutS} onChangeText={setTimeoutS} />
          <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
            <Text className="flex-1 pr-2 text-sm text-foreground">Télécharger automatiquement à la lecture</Text>
            <Switch value={auto} onValueChange={setAuto} trackColor={{ true: colors.primary, false: colors.border }} />
          </View>
          <View className="flex-row gap-2 pt-1">
            <View className="flex-1">
              <Button title="Annuler" variant="ghost" onPress={onClose} />
            </View>
            <View className="flex-1">
              <Button title="Enregistrer" icon="save-outline" loading={update.isPending} onPress={save} />
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
            <Badge label={provider.enabled ? 'Activé' : 'Désactivé'} tone={provider.enabled ? 'success' : 'default'} />
            {provider.active ? (
              <Badge label="En ligne" tone="primary" />
            ) : provider.enabled ? (
              <Badge label="Inactif" tone="danger" />
            ) : null}
            {provider.builtin ? <Badge label="Intégré" tone="default" /> : null}
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
            accessibilityLabel="Paramètres"
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
    if (!SLUG_RE.test(name)) return 'Nom invalide (slug : a-z, 0-9, -, _).';
    if (!/^https?:\/\/.+/.test(endpoint)) return 'Endpoint invalide (http(s)://…).';
    const trimmed = config.trim();
    if (trimmed) {
      try {
        JSON.parse(trimmed);
      } catch {
        return 'Config JSON invalide.';
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
      { onSuccess: onClose },
    );
  };

  return (
    <Modal transparent animationType="fade" visible onRequestClose={onClose}>
      <Pressable className="flex-1 items-center justify-center bg-black/60 px-6" onPress={onClose}>
        <Pressable className="w-full max-w-[460px] overflow-hidden rounded-2xl bg-surface" onPress={(e) => e.stopPropagation()}>
          <View className="flex-row items-center justify-between px-5 pb-2 pt-5">
            <Text className="text-lg font-bold tracking-tight text-foreground">
              {isEdit ? `Modifier « ${initial?.name} »` : 'Nouveau provider'}
            </Text>
            <IconButton name="close" color={colors.muted} onPress={onClose} accessibilityLabel="Fermer" />
          </View>

          <ScrollView
            style={{ maxHeight: 460 }}
            contentContainerStyle={{ paddingHorizontal: 20, paddingBottom: 20, gap: 12 }}
            keyboardShouldPersistTaps="handled"
          >
            <Field
              label="Nom (slug)"
              placeholder="mon-service"
              autoCapitalize="none"
              autoCorrect={false}
              editable={!isEdit}
              help={isEdit ? "Le nom n'est pas modifiable." : 'Identifiant unique : a-z, 0-9, -, _.'}
              value={name}
              onChangeText={setName}
            />
            <Field
              label="Endpoint"
              placeholder="https://mon-service.internal"
              autoCapitalize="none"
              autoCorrect={false}
              keyboardType="url"
              value={endpoint}
              onChangeText={setEndpoint}
            />
            <Field
              label="Config (JSON)"
              placeholder='{"headers":{"Authorization":"Bearer …"}}'
              autoCapitalize="none"
              autoCorrect={false}
              multiline
              numberOfLines={5}
              style={{ minHeight: 110, textAlignVertical: 'top' }}
              help="Optionnel : headers, searchPath/resolvePath/downloadPath, quality, timeoutSeconds."
              value={config}
              onChangeText={setConfig}
            />
            <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
              <Text className="text-sm font-medium text-foreground">Activer immédiatement</Text>
              <Switch value={enabled} onValueChange={setEnabled} trackColor={{ true: colors.primary, false: colors.border }} />
            </View>

            {error ? <Text className="text-xs text-danger">{error}</Text> : null}

            <View className="flex-row gap-2">
              <View className="flex-1">
                <Button title="Annuler" variant="ghost" onPress={onClose} />
              </View>
              <View className="flex-1">
                <Button
                  title={isEdit ? 'Enregistrer' : 'Créer'}
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
                  title="Confirmer la suppression"
                  icon="trash"
                  variant="danger"
                  loading={remove.isPending}
                  onPress={() => remove.mutate(initial.name, { onSuccess: onClose })}
                />
              ) : (
                <Button title="Supprimer ce provider" icon="trash-outline" variant="danger" onPress={() => setConfirming(true)} />
              )
            ) : null}
          </ScrollView>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
