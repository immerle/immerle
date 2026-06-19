import { ReactNode, useEffect, useMemo, useRef, useState } from 'react';
import { Animated, Modal, Platform, Pressable, ScrollView, Switch, Text, TextInput, View } from 'react-native';
import { useColorScheme } from 'nativewind';
import { Stack } from 'expo-router';
import { useAuth } from '../../src/auth/store';
import { useProviderLogs, useProviderMutations, useProviders, useSettings, useUpdateSettings } from '../../src/query/admin';
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
  const [editing, setEditing] = useState<Provider | null>(null);
  const [creating, setCreating] = useState(false);
  const [behaviourOpen, setBehaviourOpen] = useState(false);
  const [toast, setToast] = useState<string | null>(null);
  const hasSettings = !!client?.has('runtimeSettings');

  // Auto-dismiss the error toast.
  useEffect(() => {
    if (!toast) return;
    const id = setTimeout(() => setToast(null), 4000);
    return () => clearTimeout(id);
  }, [toast]);

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
                  <Button title={t('admin.providers.add')} size="sm" icon="add" onPress={() => setCreating(true)} />
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
                onToast={setToast}
              />
            ))
          )}

          {editing ? <ProviderModal initial={editing} onClose={() => setEditing(null)} /> : null}
          <CreateProviderModal visible={creating} onClose={() => setCreating(false)} />
          <BehaviourModal visible={behaviourOpen} onClose={() => setBehaviourOpen(false)} />
        </AdminScroll>
      )}
      {toast ? (
        <View pointerEvents="none" className="absolute inset-x-0 bottom-6 items-center px-6">
          <View className="max-w-[460px] flex-row items-center gap-2 rounded-xl bg-danger px-4 py-3 shadow-lg">
            <Ionicon name="alert-circle" size={18} color="#fff" />
            <Text className="flex-1 text-sm font-medium text-white">{toast}</Text>
          </View>
        </View>
      ) : null}
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

/** Create a dynamic provider from just its URL (centered modal). The server
 * probes /capabilities to derive the name and seed the config skeleton; the
 * provider is created disabled, then configured via the settings panel. */
function CreateProviderModal({ visible, onClose }: { visible: boolean; onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const { create } = useProviderMutations();
  const [url, setUrl] = useState('');
  const [error, setError] = useState<string | null>(null);

  // Reset on each open.
  useEffect(() => {
    if (visible) {
      setUrl('');
      setError(null);
    }
  }, [visible]);

  const submit = () => {
    if (!/^https?:\/\/.+/.test(url)) {
      setError(t('admin.providers.invalidEndpoint'));
      return;
    }
    setError(null);
    create.mutate(url.trim(), { onSuccess: onClose, onError: (e) => setError(tError(e)) });
  };

  return (
    <Modal transparent animationType="fade" visible={visible} onRequestClose={onClose}>
      <Pressable className="flex-1 items-center justify-center bg-black/60 px-6" onPress={onClose}>
        <Pressable className="w-full max-w-[440px] gap-3 rounded-2xl bg-surface p-5" onPress={(e) => e.stopPropagation()}>
          <View className="flex-row items-center justify-between">
            <Text className="text-lg font-bold tracking-tight text-foreground">{t('admin.providers.newTitle')}</Text>
            <IconButton name="close" color={colors.muted} onPress={onClose} accessibilityLabel={t('admin.providers.close')} />
          </View>
          <Text className="text-sm text-muted">{t('admin.providers.createSubtitle')}</Text>
          <Field
            label={t('admin.providers.endpointLabel')}
            placeholder="https://mon-service.internal"
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            value={url}
            onChangeText={setUrl}
          />
          {error ? <Text className="text-xs text-danger">{error}</Text> : null}
          <Button title={t('admin.providers.create')} icon="add" loading={create.isPending} onPress={submit} />
        </Pressable>
      </Pressable>
    </Modal>
  );
}

/** Per-provider error/warning log, embedded at the bottom of the settings popin
 * (only fetched while the popin is open). */
function ProviderLogsSection({ name }: { name: string }) {
  const t = useT();
  const q = useProviderLogs(name);
  const logs = q.data ?? [];

  return (
    <View className="gap-2 border-t border-border pt-3">
      <Text className="text-sm font-semibold text-foreground">{t('admin.providers.logsTitle')}</Text>
      <Text className="text-xs text-muted">{t('admin.providers.logsSubtitle')}</Text>
      {q.isLoading ? (
        <Loading />
      ) : q.isError ? (
        <ErrorState message={t('admin.providers.logsError')} onRetry={q.refetch} />
      ) : !logs.length ? (
        <Text className="py-2 text-xs text-muted">{t('admin.providers.logsEmpty')}</Text>
      ) : (
        <View className="gap-2">
          {logs.map((l) => (
            <View key={l.id} className="gap-1 rounded-xl bg-surface-alt px-3 py-2">
              <View className="flex-row items-center gap-2">
                <Badge label={l.level} tone={l.level === 'error' ? 'danger' : 'default'} />
                <Text className="text-xs font-medium text-foreground">{l.action}</Text>
                <Text className="flex-1 text-right text-[11px] text-muted">{new Date(l.createdAt).toLocaleString()}</Text>
              </View>
              <Text className="text-xs text-muted" selectable>{l.message}</Text>
            </View>
          ))}
        </View>
      )}
    </View>
  );
}

function ProviderCard({
  provider,
  onEdit,
  disabledEdit,
  onMoveUp,
  onMoveDown,
  reordering,
  onToast,
}: {
  provider: Provider;
  onEdit: () => void;
  disabledEdit: boolean;
  onMoveUp?: () => void;
  onMoveDown?: () => void;
  reordering: boolean;
  onToast: (msg: string) => void;
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
            {/* Live protocol version from the remote's /capabilities. */}
            {provider.version != null ? <Badge label={`v${provider.version}`} tone="default" /> : null}
          </View>
          {/* Built-ins have no endpoint (the "built-in" badge already labels them). */}
          {provider.endpoint ? (
            <Text className="text-xs text-muted" numberOfLines={1}>
              {provider.endpoint}
            </Text>
          ) : null}
        </View>

        <Switch
          value={provider.enabled}
          disabled={setEnabled.isPending}
          // Enabling runs the server capability check; a failure keeps it off and toasts.
          onValueChange={(v) =>
            setEnabled.mutate(
              { name: provider.name, enabled: v },
              { onError: (e) => onToast(tError(e)) },
            )
          }
          trackColor={{ true: colors.primary, false: colors.border }}
        />

        {/* Settings opens the editor + logs popin (built-ins included: config and
            logs are still relevant, only the name/endpoint are locked). */}
        <Pressable
          onPress={onEdit}
          disabled={disabledEdit}
          accessibilityLabel={t('admin.providers.settings')}
          className={`h-9 w-9 items-center justify-center rounded-full bg-surface-alt ${disabledEdit ? 'opacity-40' : 'active:opacity-70'}`}
        >
          <Ionicon name="settings-outline" size={18} color={colors.foreground} />
        </Pressable>
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

// JSON syntax-highlight palette (token → hex), tuned for both themes.
const JSON_SYNTAX = {
  light: { key: '#0b7285', str: '#2b8a3e', num: '#e8590c', lit: '#9c36b5', punct: '#868e96' },
  dark: { key: '#66d9e8', str: '#69db7c', num: '#ffa94d', lit: '#da77f2', punct: '#909296' },
} as const;

// One regex pass: object keys ("…":), strings, true/false/null, numbers, punctuation.
const JSON_TOKEN_RE =
  /("(?:\\.|[^"\\])*"\s*:)|("(?:\\.|[^"\\])*")|(\b(?:true|false|null)\b)|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|([{}[\],:])/g;

/** Tokenize JSON source into colored <Text> spans (best-effort; never throws). */
function highlightJson(src: string, pal: { key: string; str: string; num: string; lit: string; punct: string }, fallback: string) {
  const out: ReactNode[] = [];
  let last = 0;
  let m: RegExpExecArray | null;
  JSON_TOKEN_RE.lastIndex = 0;
  while ((m = JSON_TOKEN_RE.exec(src))) {
    if (m.index > last) out.push(<Text key={last} style={{ color: fallback }}>{src.slice(last, m.index)}</Text>);
    const [full, key, str, lit, num] = m;
    const color = key ? pal.key : str ? pal.str : lit ? pal.lit : num ? pal.num : pal.punct;
    out.push(<Text key={m.index} style={{ color }}>{full}</Text>);
    last = m.index + full.length;
  }
  if (last < src.length) out.push(<Text key="tail" style={{ color: fallback }}>{src.slice(last)}</Text>);
  return out;
}

/** Config editor: monospace JSON with syntax highlighting, live validation and a
 * Format button. The unified schema is { header: {…}, params: {…}, … }. A colored
 * <Text> overlay sits behind a transparent-glyph TextInput (admin is web-only). */
function JsonConfigField({ value, onChangeText }: { value: string; onChangeText: (v: string) => void }) {
  const t = useT();
  const colors = useColors();
  const { colorScheme } = useColorScheme();
  const pal = colorScheme === 'dark' ? JSON_SYNTAX.dark : JSON_SYNTAX.light;
  const placeholder = '{\n  "header": {},\n  "params": {}\n}';

  // Web textarea DOM node, for caret-aware Tab handling.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const inputRef = useRef<any>(null);

  const error = useMemo(() => {
    const s = value.trim();
    if (!s) return null;
    try {
      JSON.parse(s);
      return null;
    } catch (e) {
      return (e as Error).message;
    }
  }, [value]);

  // Tab inserts two spaces at the caret instead of moving focus (web-only).
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const onKeyPress = (e: any) => {
    if (e?.nativeEvent?.key !== 'Tab') return;
    e.preventDefault?.();
    const el = inputRef.current;
    const start = el?.selectionStart ?? value.length;
    const end = el?.selectionEnd ?? start;
    onChangeText(value.slice(0, start) + '  ' + value.slice(end));
    requestAnimationFrame(() => {
      if (el && typeof el.setSelectionRange === 'function') el.setSelectionRange(start + 2, start + 2);
    });
  };

  // Shared text metrics so the overlay lines up exactly with the input.
  const textStyle = {
    fontFamily: Platform.select({ web: 'monospace', default: 'Courier' }),
    fontSize: 13,
    lineHeight: 20,
    padding: 10,
  } as const;

  return (
    <View className="gap-1.5">
      <View className="flex-row items-center justify-between">
        <Text className="text-sm font-medium text-foreground">{t('admin.providers.configLabel')}</Text>
        <Pressable
          onPress={() => onChangeText(JSON.stringify(JSON.parse(value), null, 2))}
          disabled={!!error || !value.trim()}
          className={`rounded-lg px-2 py-1 ${error || !value.trim() ? 'opacity-40' : 'bg-surface-alt active:opacity-70'}`}
        >
          <Text className="text-xs text-foreground">{t('admin.providers.configFormat')}</Text>
        </Pressable>
      </View>
      <View style={{ position: 'relative', minHeight: 140, borderWidth: 1, borderColor: error ? colors.danger : colors.border, borderRadius: 12 }}>
        {/* Colored layer behind the input (or the placeholder when empty). */}
        <View pointerEvents="none" style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }}>
          <Text style={textStyle}>
            {value ? highlightJson(value, pal, colors.foreground) : <Text style={{ color: colors.muted }}>{placeholder}</Text>}
          </Text>
        </View>
        <TextInput
          ref={inputRef}
          value={value}
          onChangeText={onChangeText}
          onKeyPress={onKeyPress}
          multiline
          autoCapitalize="none"
          autoCorrect={false}
          spellCheck={false}
          // Glyphs invisible (the overlay shows them in color) but the caret stays visible.
          style={[
            textStyle,
            { flex: 1, minHeight: 140, textAlignVertical: 'top', color: colors.foreground },
            // Web-only props (transparent glyphs, visible caret) — not in RN's TextStyle.
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            (Platform.OS === 'web'
              ? { WebkitTextFillColor: 'transparent', caretColor: colors.foreground }
              : null) as any,
          ]}
        />
      </View>
      <Text className={`text-xs ${error ? 'text-danger' : 'text-muted'}`}>
        {error ? t('admin.providers.configInvalid') : t('admin.providers.configHelp')}
      </Text>
    </View>
  );
}

/** Settings side panel for an existing provider: edit the config (validated
 * against /capabilities on save for HTTP providers), reorder is on the card. */
function ProviderModal({ initial, onClose }: { initial: Provider; onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const { upsert, remove } = useProviderMutations();
  const isBuiltin = initial.builtin;
  const [endpoint, setEndpoint] = useState(initial.endpoint);
  const [config, setConfig] = useState(initial.config || '{}');
  const [error, setError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);

  // Slide-in-from-the-right animation (RN Modal has no lateral slide).
  const PANEL_W = 540;
  const slide = useRef(new Animated.Value(PANEL_W)).current;
  const fade = useRef(new Animated.Value(0)).current;
  useEffect(() => {
    Animated.parallel([
      Animated.timing(slide, { toValue: 0, duration: 220, useNativeDriver: false }),
      Animated.timing(fade, { toValue: 1, duration: 220, useNativeDriver: false }),
    ]).start();
  }, [slide, fade]);
  // Animate out, then let the parent unmount us.
  const close = () =>
    Animated.parallel([
      Animated.timing(slide, { toValue: PANEL_W, duration: 180, useNativeDriver: false }),
      Animated.timing(fade, { toValue: 0, duration: 180, useNativeDriver: false }),
    ]).start(() => onClose());

  const validate = (): string | null => {
    // Built-ins have no endpoint (the server compiles in how to reach them).
    if (!isBuiltin && !/^https?:\/\/.+/.test(endpoint)) return t('admin.providers.invalidEndpoint');
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
    // Preserve the current enabled state — enabling is done from the card switch.
    upsert.mutate(
      {
        name: initial.name,
        endpoint,
        config: config.trim() || '{}',
        enabled: initial.enabled,
        kind: isBuiltin ? 'builtin' : 'http',
      },
      { onSuccess: close, onError: (e) => setError(tError(e)) },
    );
  };

  return (
    <Modal transparent animationType="none" visible onRequestClose={close}>
      <Pressable className="flex-1 flex-row justify-end" onPress={close}>
        {/* Dimmed backdrop fades in/out with the panel. */}
        <Animated.View pointerEvents="none" style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.6)', opacity: fade }} />
        {/* Side panel slides in from the right — room for the config editor + logs. */}
        <Animated.View style={{ height: '100%', width: '100%', maxWidth: PANEL_W, transform: [{ translateX: slide }] }}>
          <Pressable className="h-full bg-surface" onPress={(e) => e.stopPropagation()}>
          <View className="flex-row items-center justify-between border-b border-border px-5 py-4">
            <Text className="text-lg font-bold tracking-tight text-foreground">
              {t('admin.providers.editTitle', { name: initial.name })}
            </Text>
            <IconButton name="close" color={colors.muted} onPress={close} accessibilityLabel={t('admin.providers.close')} />
          </View>

          <ScrollView
            style={{ flex: 1 }}
            contentContainerStyle={{ paddingHorizontal: 20, paddingVertical: 16, gap: 12 }}
            keyboardShouldPersistTaps="handled"
          >
            <Field
              label={t('admin.providers.nameLabel')}
              editable={false}
              help={t('admin.providers.nameHelpLocked')}
              value={initial.name}
            />
            {!isBuiltin ? (
              <Field
                label={t('admin.providers.endpointLabel')}
                placeholder="https://mon-service.internal"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                value={endpoint}
                onChangeText={setEndpoint}
              />
            ) : null}
            <JsonConfigField value={config} onChangeText={setConfig} />

            {error ? <Text className="text-xs text-danger">{error}</Text> : null}

            <Button
              title={t('admin.providers.save')}
              icon="save-outline"
              loading={upsert.isPending}
              onPress={submit}
            />

            {/* Delete lives here (built-ins are never deletable). */}
            {initial.deletable ? (
              confirming ? (
                <Button
                  title={t('admin.providers.confirmDelete')}
                  icon="trash"
                  variant="danger"
                  loading={remove.isPending}
                  onPress={() => remove.mutate(initial.name, { onSuccess: close })}
                />
              ) : (
                <Button title={t('admin.providers.delete')} icon="trash-outline" variant="danger" onPress={() => setConfirming(true)} />
              )
            ) : null}

            <ProviderLogsSection name={initial.name} />
          </ScrollView>
          </Pressable>
        </Animated.View>
      </Pressable>
    </Modal>
  );
}
