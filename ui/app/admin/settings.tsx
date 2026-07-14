import { useEffect, useState } from 'react';
import { Modal, Pressable, ScrollView, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useCleanup,
  useCleanupMutations,
  useSettings,
  useUpdateSettings,
} from '../../src/query/admin';
import { useSmartPlaylistsAdmin, useSetSmartPlaylists } from '../../src/query/smartPlaylists';
import { useRadioAdmin, useSetRadio } from '../../src/query/radio';
import { useWrappedAdmin, useSetWrapped } from '../../src/query/wrapped';
import { useOfflineAdmin, useSetOffline } from '../../src/query/offline';
import { useHallOfFameAdmin, useSetHallOfFame } from '../../src/query/hallOfFame';
import { useAuth } from '../../src/auth/store';
import { RuntimeSettingsDTO } from '../../src/api/immerleApi';
import { Badge, Button, Card, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';

const num = (s: string) => {
  const n = Number(s);
  return Number.isFinite(n) ? n : 0;
};

const RESTART_LABEL_KEYS: Record<string, string> = {
  transcode: 'admin.settings.restartTranscode',
  'scan.watch': 'admin.settings.restartScanWatch',
};

type SectionKey = 'auth' | 'ldap' | 'server' | 'transcode' | 'cleanup' | 'logs' | 'features';

interface Section {
  key: SectionKey;
  icon: string;
  color: string;
  title: string;
  subtitle: string;
}

interface Form {
  cors: string;
  ttl: string;
  ldapEnabled: boolean;
  ldapUrl: string;
  ldapBindDn: string;
  ffmpeg: string;
  ffprobe: string;
  logRetention: string;
}

function toForm(s: RuntimeSettingsDTO): Form {
  return {
    cors: (s.server?.corsAllowedOrigins ?? []).join(', '),
    ttl: String(s.auth?.deviceTokenTtlSeconds ?? 0),
    ldapEnabled: s.ldap?.enabled ?? false,
    ldapUrl: s.ldap?.url ?? '',
    ldapBindDn: s.ldap?.bindDnTemplate ?? '',
    ffmpeg: s.transcode?.ffmpegPath ?? '',
    ffprobe: s.transcode?.ffprobePath ?? '',
    logRetention: String(s.logs?.retentionDays ?? 30),
  };
}

/** Runtime settings admin. Each group opens in a bottom sheet; hot-reload fields
 * apply instantly, restart-only changes raise a "restart pending" banner. */
export default function AdminSettings() {
  const t = useT();
  const colors = useColors();
  const SECTIONS: Section[] = [
    { key: 'auth', icon: 'key', color: '#3b82f6', title: t('admin.settings.authTitle'), subtitle: t('admin.settings.authSubtitle') },
    { key: 'ldap', icon: 'people-circle', color: '#22c55e', title: t('admin.settings.ldapTitle'), subtitle: t('admin.settings.ldapSubtitle') },
    { key: 'server', icon: 'server', color: '#0ea5e9', title: t('admin.settings.serverTitle'), subtitle: t('admin.settings.serverSubtitle') },
    { key: 'transcode', icon: 'film', color: '#a855f7', title: t('admin.settings.transcodeTitle'), subtitle: t('admin.settings.transcodeSubtitle') },
    { key: 'cleanup', icon: 'trash-bin', color: '#ef4444', title: t('admin.settings.cleanupTitle'), subtitle: t('admin.settings.cleanupSubtitle') },
    { key: 'logs', icon: 'document-text', color: '#f59e0b', title: t('admin.settings.logsTitle'), subtitle: t('admin.settings.logsSubtitle') },
    { key: 'features', icon: 'sparkles', color: '#8b5cf6', title: t('admin.settings.featuresTitle'), subtitle: t('admin.settings.featuresSubtitle') },
  ];
  const client = useAuth((s) => s.client);
  const q = useSettings();
  const update = useUpdateSettings();
  const cleanup = useCleanup();
  const cleanupM = useCleanupMutations();
  const smart = useSmartPlaylistsAdmin();
  const setSmart = useSetSmartPlaylists();
  const radio = useRadioAdmin();
  const setRadio = useSetRadio();
  const wrapped = useWrappedAdmin();
  const setWrapped = useSetWrapped();
  const offline = useOfflineAdmin();
  const setOffline = useSetOffline();
  const hallOfFame = useHallOfFameAdmin();
  const setHallOfFame = useSetHallOfFame();
  const [form, setForm] = useState<Form | null>(null);
  const [sheet, setSheet] = useState<SectionKey | null>(null);
  const [removed, setRemoved] = useState<number | null>(null);

  useEffect(() => {
    if (q.data?.settings) setForm(toForm(q.data.settings));
  }, [q.data?.settings]);

  if (q.isLoading || !form) return <Loading />;
  if (q.isError) return <ErrorState message={t('admin.settings.loadError')} onRetry={q.refetch} />;

  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const save = (patch: RuntimeSettingsDTO, onSuccess?: () => void) => update.mutate(patch, { onSuccess });

  const pending = q.data?.pendingRestart ?? [];
  const profiles = q.data?.settings.transcode?.profiles ?? [];
  const rows = SECTIONS.filter((s) => {
    if (s.key === 'cleanup') return !!cleanup.data;
    if (s.key === 'features')
      return (
        !!client?.has('smartPlaylists') ||
        !!client?.has('internetRadio') ||
        !!client?.has('wrapped') ||
        !!client?.has('offlineDownloads') ||
        !!client?.has('hallOfFame')
      );
    return true;
  });
  const active = SECTIONS.find((s) => s.key === sheet);

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#0ea5e9" title={t('admin.settings.title')} subtitle={t('admin.settings.headerSubtitle')} />}
      >
        {/* Restart-pending banner */}
        {pending.length ? (
          <View className="gap-1.5 rounded-2xl border border-danger/40 bg-danger/10 p-4">
            <View className="flex-row items-center gap-2">
              <Ionicon name="warning" size={18} color="#ef4444" />
              <Text className="text-sm font-semibold text-foreground">{t('admin.settings.restartRequired')}</Text>
            </View>
            <Text className="text-xs text-muted">
              {t('admin.settings.restartPending')}{' '}
              {pending.map((p) => (RESTART_LABEL_KEYS[p] ? t(RESTART_LABEL_KEYS[p]) : p)).join(', ')}.
            </Text>
          </View>
        ) : null}

        {rows.map((s) => (
          <SettingRow key={s.key} section={s} onPress={() => setSheet(s.key)} />
        ))}
      </AdminScroll>

      {/* Bottom sheet for the active section */}
      <Modal transparent visible={!!sheet} animationType="slide" onRequestClose={() => setSheet(null)}>
        <Pressable className="flex-1 justify-end bg-black/50" onPress={() => setSheet(null)}>
          <Pressable className="max-h-[85%] rounded-t-3xl bg-surface pb-6 pt-2" onPress={(e) => e.stopPropagation()}>
            <View className="mb-1 items-center pt-1">
              <View className="h-1 w-10 rounded-full bg-border" />
            </View>
            {active ? (
              <View className="flex-row items-center gap-2.5 px-5 pb-2 pt-1">
                <View className="h-9 w-9 items-center justify-center rounded-xl" style={{ backgroundColor: active.color + '22' }}>
                  <Ionicon name={active.icon} size={18} color={active.color} />
                </View>
                <Text className="flex-1 text-lg font-bold tracking-tight text-foreground">{active.title}</Text>
                <IconButton name="close" color={colors.muted} onPress={() => setSheet(null)} accessibilityLabel={t('admin.settings.close')} />
              </View>
            ) : null}

            <ScrollView contentContainerStyle={{ paddingHorizontal: 20, paddingTop: 4, paddingBottom: 8, gap: 12 }} keyboardShouldPersistTaps="handled">
              {sheet === 'auth' ? (
                <>
                  <Field label={t('admin.settings.deviceTokenTtl')} keyboardType="number-pad" value={form.ttl} onChangeText={(v) => set('ttl', v)} />
                  <SaveButton loading={update.isPending} onPress={() => save({ auth: { deviceTokenTtlSeconds: num(form.ttl) } })} />
                </>
              ) : null}

              {sheet === 'ldap' ? (
                <>
                  <Text className="text-xs text-muted">{t('admin.settings.ldapDescription')}</Text>
                  <ToggleRow label={t('admin.settings.ldapEnabled')} value={form.ldapEnabled} onChange={(v) => set('ldapEnabled', v)} />
                  <Field label={t('admin.settings.ldapUrl')} autoCapitalize="none" autoCorrect={false} keyboardType="url" placeholder="ldaps://ldap.example.com:636" value={form.ldapUrl} onChangeText={(v) => set('ldapUrl', v)} />
                  <Field label={t('admin.settings.ldapBindDn')} autoCapitalize="none" autoCorrect={false} placeholder="uid=%s,ou=people,dc=example,dc=com" value={form.ldapBindDn} onChangeText={(v) => set('ldapBindDn', v)} help={t('admin.settings.ldapBindDnHelp')} />
                  <SaveButton
                    loading={update.isPending}
                    onPress={() => save({ ldap: { enabled: form.ldapEnabled, url: form.ldapUrl, bindDnTemplate: form.ldapBindDn } })}
                  />
                </>
              ) : null}

              {sheet === 'server' ? (
                <>
                  <Field label={t('admin.settings.corsLabel')} autoCapitalize="none" placeholder="*" value={form.cors} onChangeText={(v) => set('cors', v)} help={t('admin.settings.corsHelp')} />
                  <SaveButton
                    loading={update.isPending}
                    onPress={() => save({ server: { corsAllowedOrigins: form.cors.split(',').map((o) => o.trim()).filter(Boolean) } })}
                  />
                </>
              ) : null}

              {sheet === 'transcode' ? (
                <>
                  <Field label={t('admin.settings.ffmpegPath')} autoCapitalize="none" value={form.ffmpeg} onChangeText={(v) => set('ffmpeg', v)} />
                  <Field label={t('admin.settings.ffprobePath')} autoCapitalize="none" value={form.ffprobe} onChangeText={(v) => set('ffprobe', v)} />
                  {profiles.length ? (
                    <View className="gap-1.5 rounded-xl bg-surface-alt p-3">
                      <Text className="text-xs font-medium uppercase tracking-wider text-muted">{t('admin.settings.profiles')}</Text>
                      {profiles.map((p, i) => (
                        <Text key={i} className="text-sm text-foreground">
                          {p.name} · {p.format?.toUpperCase()} · {p.bitRate} kbps
                        </Text>
                      ))}
                    </View>
                  ) : null}
                  <SaveButton loading={update.isPending} hint={t('admin.settings.restartHint')} onPress={() => save({ transcode: { ffmpegPath: form.ffmpeg, ffprobePath: form.ffprobe } })} />
                </>
              ) : null}

              {sheet === 'logs' ? (
                <>
                  <Text className="text-xs text-muted">{t('admin.settings.logsDescription')}</Text>
                  <Field
                    label={t('admin.settings.logRetention')}
                    keyboardType="number-pad"
                    value={form.logRetention}
                    onChangeText={(v) => set('logRetention', v)}
                    help={t('admin.settings.logRetentionHelp')}
                  />
                  <SaveButton loading={update.isPending} onPress={() => save({ logs: { retentionDays: num(form.logRetention) } })} />
                </>
              ) : null}

              {sheet === 'features' ? (
                <>
                  <Text className="text-xs text-muted">{t('admin.settings.featuresDescription')}</Text>
                  {client?.has('smartPlaylists') ? (
                    <ToggleRow
                      label={t('admin.settings.smartPlaylistsEnabled')}
                      value={smart.data ?? false}
                      onChange={(v) => setSmart.mutate(v)}
                    />
                  ) : null}
                  {client?.has('internetRadio') ? (
                    <ToggleRow
                      label={t('admin.settings.radioEnabled')}
                      value={radio.data ?? false}
                      onChange={(v) => setRadio.mutate(v)}
                    />
                  ) : null}
                  {client?.has('wrapped') ? (
                    <ToggleRow
                      label={t('admin.settings.wrappedEnabled')}
                      value={wrapped.data ?? false}
                      onChange={(v) => setWrapped.mutate(v)}
                    />
                  ) : null}
                  {client?.has('offlineDownloads') ? (
                    <ToggleRow
                      label={t('admin.settings.offlineEnabled')}
                      value={offline.data ?? false}
                      onChange={(v) => setOffline.mutate(v)}
                    />
                  ) : null}
                  {client?.has('hallOfFame') ? (
                    <ToggleRow
                      label={t('admin.settings.hallOfFameEnabled')}
                      value={hallOfFame.data ?? false}
                      onChange={(v) => setHallOfFame.mutate(v)}
                    />
                  ) : null}
                </>
              ) : null}

              {sheet === 'cleanup' && cleanup.data ? (
                <>
                  <Text className="text-xs text-muted">
                    {t('admin.settings.cleanupDescription', {
                      days: Math.round(cleanup.data.maxAgeSeconds / 86400),
                      hours: Math.round(cleanup.data.intervalSeconds / 3600),
                    })}
                  </Text>
                  <ToggleRow label={t('admin.settings.autoSweep')} value={cleanup.data.enabled} onChange={(v) => cleanupM.setEnabled.mutate(v)} />
                  <Button
                    title={removed != null ? t('admin.settings.removedCount', { count: removed }) : t('admin.settings.runCleanup')}
                    icon="trash-outline"
                    variant="secondary"
                    loading={cleanupM.run.isPending}
                    onPress={() => cleanupM.run.mutate(undefined, { onSuccess: (n) => setRemoved(n) })}
                  />
                </>
              ) : null}
            </ScrollView>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

function SettingRow({ section, onPress }: { section: Section; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="active:opacity-70">
      <Card className="flex-row items-center gap-3">
        <View className="h-10 w-10 items-center justify-center rounded-xl" style={{ backgroundColor: section.color + '22' }}>
          <Ionicon name={section.icon} size={20} color={section.color} />
        </View>
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground">{section.title}</Text>
          <Text className="text-xs text-muted">{section.subtitle}</Text>
        </View>
        <Ionicon name="chevron-forward" size={18} color={colors.muted} />
      </Card>
    </Pressable>
  );
}

function ToggleRow({ label, hint, value, onChange }: { label: string; hint?: string; value: boolean; onChange: (v: boolean) => void }) {
  const colors = useColors();
  return (
    <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
      <View className="flex-1 flex-row items-center gap-2 pr-2">
        <Text className="text-sm text-foreground">{label}</Text>
        {hint ? <Badge label={hint} tone="default" /> : null}
      </View>
      <Switch value={value} onValueChange={onChange} trackColor={{ true: colors.primary, false: colors.border }} />
    </View>
  );
}

function SaveButton({ loading, hint, onPress }: { loading: boolean; hint?: string; onPress: () => void }) {
  const t = useT();
  return (
    <View className="flex-row items-center justify-end gap-2 pt-1">
      {hint ? <Badge label={hint} tone="default" /> : null}
      <Button title={t('admin.settings.save')} icon="save-outline" loading={loading} onPress={onPress} />
    </View>
  );
}
