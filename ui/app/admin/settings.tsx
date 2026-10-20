import { ReactNode, useEffect, useState } from 'react';
import { Pressable, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useChartsSyncMutation,
  useCleanup,
  useCleanupMutations,
  useSettings,
  useSonicStatus,
  useUpdateSettings,
} from '../../src/query/admin';
import { useSmartPlaylistsAdmin, useSetSmartPlaylists } from '../../src/query/smartPlaylists';
import { useRadioAdmin, useSetRadio } from '../../src/query/radio';
import { useWrappedAdmin, useSetWrapped } from '../../src/query/wrapped';
import { useOfflineAdmin, useSetOffline } from '../../src/query/offline';
import { useHallOfFameAdmin, useSetHallOfFame } from '../../src/query/hallOfFame';
import { useConcertsAdmin, useUpdateConcertsConfig, useConcertsSyncMutation } from '../../src/query/concerts';
import { useLastFmAdmin, useUpdateLastFmAdminConfig } from '../../src/query/lastfm';
import { useAuth } from '../../src/auth/store';
import { RuntimeSettingsDTO } from '../../src/api/immerleApi';
import { Badge, Button, Card, ErrorState, Field, Loading, Select } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';
import { useT } from '../../src/i18n/store';
import { COUNTRIES } from '../../src/utils/countries';
import { concertProviderNames, concertProviderApplies } from '../../src/utils/concertProviders';

const num = (s: string) => {
  const n = Number(s);
  return Number.isFinite(n) ? n : 0;
};

const RESTART_LABEL_KEYS: Record<string, string> = {
  transcode: 'admin.settings.restartTranscode',
  'scan.watch': 'admin.settings.restartScanWatch',
};

type SectionKey = 'auth' | 'ldap' | 'server' | 'transcode' | 'cleanup' | 'charts' | 'concerts' | 'lastfm' | 'logs' | 'features' | 'sonic';

/** Sections grouped by theme, in display order. */
const GROUPS: { titleKey: string; keys: SectionKey[] }[] = [
  { titleKey: 'admin.settings.groupAccess', keys: ['auth', 'ldap', 'server'] },
  { titleKey: 'admin.settings.groupPlayback', keys: ['transcode'] },
  { titleKey: 'admin.settings.groupContent', keys: ['features', 'sonic', 'charts', 'concerts'] },
  { titleKey: 'admin.settings.groupIntegrations', keys: ['lastfm'] },
  { titleKey: 'admin.settings.groupMaintenance', keys: ['cleanup', 'logs'] },
];

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
  sonicWorkers: string;
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
    sonicWorkers: String(s.sonic?.workers ?? 1),
  };
}

/** Runtime settings admin, grouped by theme. Each section expands in place;
 * hot-reload fields apply instantly, restart-only changes raise a "restart
 * pending" banner. */
export default function AdminSettings() {
  const t = useT();
  const COUNTRY_OPTIONS = [
    { value: '', label: t('admin.settings.countryNotSet') },
    ...COUNTRIES.map((c) => ({ value: c.code, label: c.name })),
  ];
  const client = useAuth((s) => s.client);
  const q = useSettings();
  const update = useUpdateSettings();
  const cleanup = useCleanup();
  const cleanupM = useCleanupMutations();
  const chartsSync = useChartsSyncMutation();
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
  const concerts = useConcertsAdmin();
  const updateConcerts = useUpdateConcertsConfig();
  const concertsSync = useConcertsSyncMutation();
  const lastFm = useLastFmAdmin();
  const updateLastFm = useUpdateLastFmAdminConfig();
  const [form, setForm] = useState<Form | null>(null);
  const [open, setOpen] = useState<Set<SectionKey>>(new Set());
  const sonic = useSonicStatus(open.has('sonic'));
  const [removed, setRemoved] = useState<number | null>(null);
  const [synced, setSynced] = useState<number | null>(null);
  const [concertsSynced, setConcertsSynced] = useState<number | null>(null);
  const [concertsEnabled, setConcertsEnabled] = useState(false);
  const [concertsCountry, setConcertsCountry] = useState('');
  const [tmKey, setTmKey] = useState('');
  const [skKey, setSkKey] = useState('');
  const [lastFmEnabled, setLastFmEnabled] = useState(false);
  const [lastFmKey, setLastFmKey] = useState('');
  const [lastFmSecret, setLastFmSecret] = useState('');

  // The concerts subtitle depends on the selected country (Ticketmaster and
  // Skiddle aren't available everywhere — see CONCERT_PROVIDERS), so this
  // list is built here, not as a static top-level const.
  const SECTIONS: Section[] = [
    { key: 'auth', icon: 'key', color: '#3b82f6', title: t('admin.settings.authTitle'), subtitle: t('admin.settings.authSubtitle') },
    { key: 'ldap', icon: 'people-circle', color: '#22c55e', title: t('admin.settings.ldapTitle'), subtitle: t('admin.settings.ldapSubtitle') },
    { key: 'server', icon: 'server', color: '#0ea5e9', title: t('admin.settings.serverTitle'), subtitle: t('admin.settings.serverSubtitle') },
    { key: 'transcode', icon: 'film', color: '#a855f7', title: t('admin.settings.transcodeTitle'), subtitle: t('admin.settings.transcodeSubtitle') },
    { key: 'cleanup', icon: 'trash-bin', color: '#ef4444', title: t('admin.settings.cleanupTitle'), subtitle: t('admin.settings.cleanupSubtitle') },
    { key: 'charts', icon: 'trending-up', color: '#1db954', title: t('admin.settings.chartsTitle'), subtitle: t('admin.settings.chartsSubtitle') },
    { key: 'concerts', icon: 'megaphone', color: '#ec4899', title: t('admin.settings.concertsTitle'), subtitle: concertProviderNames(concertsCountry) || t('admin.settings.countryNotSet') },
    { key: 'lastfm', icon: 'disc', color: '#eab308', title: t('admin.settings.lastFmTitle'), subtitle: t('admin.settings.lastFmSubtitle') },
    { key: 'logs', icon: 'document-text', color: '#f59e0b', title: t('admin.settings.logsTitle'), subtitle: t('admin.settings.logsSubtitle') },
    { key: 'sonic', icon: 'pulse', color: '#14b8a6', title: t('admin.settings.sonicTitle'), subtitle: t('admin.settings.sonicSubtitle') },
    { key: 'features', icon: 'sparkles', color: '#8b5cf6', title: t('admin.settings.featuresTitle'), subtitle: t('admin.settings.featuresSubtitle') },
  ];

  useEffect(() => {
    if (q.data?.settings) setForm(toForm(q.data.settings));
  }, [q.data?.settings]);

  useEffect(() => {
    if (concerts.data) {
      setConcertsEnabled(concerts.data.enabled);
      setConcertsCountry(concerts.data.country);
    }
  }, [concerts.data]);

  useEffect(() => {
    if (lastFm.data) setLastFmEnabled(lastFm.data.enabled);
  }, [lastFm.data]);

  if (q.isLoading || !form) return <Loading />;
  if (q.isError) return <ErrorState message={t('admin.settings.loadError')} onRetry={q.refetch} />;

  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const save = (patch: RuntimeSettingsDTO, onSuccess?: () => void) => update.mutate(patch, { onSuccess });

  const pending = q.data?.pendingRestart ?? [];
  const profiles = q.data?.settings.transcode?.profiles ?? [];
  const visible = (key: SectionKey) => {
    if (key === 'cleanup') return !!cleanup.data;
    if (key === 'concerts') return !!client?.has('concertDiscovery');
    if (key === 'lastfm') return !!client?.has('lastFm');
    return true;
  };
  const toggle = (key: SectionKey) =>
    setOpen((cur) => {
      const next = new Set(cur);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#0ea5e9" title={t('admin.settings.title')} subtitle={t('admin.settings.headerSubtitle')} />}
      >
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

        {GROUPS.map((g) => {
          const sections = g.keys.filter(visible).map((k) => SECTIONS.find((sec) => sec.key === k)!);
          if (!sections.length) return null;
          return (
            <View key={g.titleKey} className="gap-2.5">
              <Text className="px-1 pt-2 text-xs font-semibold uppercase tracking-wider text-muted">{t(g.titleKey)}</Text>
              {sections.map((sec) => (
                <SettingSection key={sec.key} section={sec} open={open.has(sec.key)} onToggle={() => toggle(sec.key)}>
                  {renderBody(sec.key)}
                </SettingSection>
              ))}
            </View>
          );
        })}
      </AdminScroll>
    </>
  );

  function renderBody(sheet: SectionKey) {
    if (!form) return null;
    return (
      <>
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
            <ToggleRow
              label={t('admin.settings.spotifySearchEnabled')}
              value={q.data?.settings.spotifySearch?.enabled ?? false}
              onChange={(v) => save({ spotifySearch: { enabled: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.spotifySearchHelp')}</Text>
          </>
        ) : null}

        {sheet === 'sonic' ? (
          <>
            <Text className="text-xs text-muted">{t('admin.settings.sonicDescription')}</Text>
            {sonic.data && !sonic.data.available ? (
              <Text className="text-xs text-danger">{t('admin.settings.sonicUnavailable')}</Text>
            ) : null}
            <ToggleRow
              label={t('admin.settings.sonicEnabled')}
              value={q.data?.settings.sonic?.enabled ?? false}
              onChange={(v) => save({ sonic: { enabled: v } }, () => sonic.refetch())}
            />
            <Field
              label={t('admin.settings.sonicWorkers')}
              keyboardType="number-pad"
              value={form.sonicWorkers}
              onChangeText={(v) => set('sonicWorkers', v)}
              help={t('admin.settings.sonicWorkersHelp')}
            />
            <SaveButton loading={update.isPending} onPress={() => save({ sonic: { workers: num(form.sonicWorkers) } })} />
            <Text className="pt-2 text-xs font-semibold uppercase tracking-wider text-muted">{t('admin.settings.sonicFeatures')}</Text>
            <ToggleRow
              label={t('admin.settings.sonicRadio')}
              hint={q.data?.settings.sonic?.enabled ? undefined : t('admin.settings.sonicNeedsAnalysis')}
              value={q.data?.settings.sonic?.radio ?? false}
              onChange={(v) => save({ sonic: { radio: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.sonicRadioHelp')}</Text>
            <ToggleRow
              label={t('admin.settings.sonicAutoplay')}
              hint={q.data?.settings.sonic?.enabled ? undefined : t('admin.settings.sonicNeedsAnalysis')}
              value={q.data?.settings.sonic?.autoplay ?? false}
              onChange={(v) => save({ sonic: { autoplay: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.sonicAutoplayHelp')}</Text>
            <ToggleRow
              label={t('admin.settings.sonicMoods')}
              hint={q.data?.settings.sonic?.enabled ? undefined : t('admin.settings.sonicNeedsAnalysis')}
              value={q.data?.settings.sonic?.moods ?? false}
              onChange={(v) => save({ sonic: { moods: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.sonicMoodsHelp')}</Text>
            <ToggleRow
              label={t('admin.settings.sonicAutoDj')}
              hint={q.data?.settings.sonic?.enabled ? undefined : t('admin.settings.sonicNeedsAnalysis')}
              value={q.data?.settings.sonic?.autoDj ?? false}
              onChange={(v) => save({ sonic: { autoDj: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.sonicAutoDjHelp')}</Text>
            <ToggleRow
              label={t('admin.settings.sonicDuplicates')}
              hint={q.data?.settings.sonic?.enabled ? undefined : t('admin.settings.sonicNeedsAnalysis')}
              value={q.data?.settings.sonic?.duplicates ?? false}
              onChange={(v) => save({ sonic: { duplicates: v } })}
            />
            <Text className="text-xs text-muted">{t('admin.settings.sonicDuplicatesHelp')}</Text>
            {sonic.data?.available && sonic.data.total ? (
              <View className="gap-0.5 rounded-xl bg-surface-alt p-3">
                <Text className="text-sm text-foreground">
                  {t('admin.settings.sonicProgress', { analyzed: sonic.data.analyzed ?? 0, total: sonic.data.total })}
                </Text>
                {sonic.data.failed ? (
                  <Text className="text-xs text-muted">{t('admin.settings.sonicFailed', { count: sonic.data.failed })}</Text>
                ) : null}
              </View>
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
        {sheet === 'charts' ? (
          <>
            <Text className="text-xs text-muted">{t('admin.settings.chartsDescription')}</Text>
            <Button
              title={synced != null ? t('admin.settings.chartsSyncedCount', { count: synced }) : t('admin.settings.runChartsSync')}
              icon="trending-up"
              variant="secondary"
              loading={chartsSync.isPending}
              onPress={() => chartsSync.mutate(undefined, { onSuccess: (n) => setSynced(n) })}
            />
          </>
        ) : null}

        {sheet === 'concerts' ? (
          <>
            <Text className="text-xs text-muted">{t('admin.settings.concertsDescription')}</Text>
            <ToggleRow label={t('admin.settings.concertsEnabled')} value={concertsEnabled} onChange={setConcertsEnabled} />
            <View className="gap-1.5">
              <Text className="text-sm font-medium text-muted">{t('admin.settings.concertsCountry')}</Text>
              <Select value={concertsCountry} options={COUNTRY_OPTIONS} onChange={setConcertsCountry} />
              <Text className="text-xs text-muted">
                {t('admin.settings.concertsSources', { list: concertProviderNames(concertsCountry) })}
              </Text>
            </View>
            {concertProviderApplies('Ticketmaster', concertsCountry) ? (
              <Field
                label={t('admin.settings.ticketmasterKey')}
                autoCapitalize="none"
                autoCorrect={false}
                placeholder={concerts.data?.ticketmasterConfigured ? t('admin.settings.keyConfigured') : t('admin.settings.keyNotConfigured')}
                value={tmKey}
                onChangeText={setTmKey}
                help={!concertProviderApplies('Skiddle', concertsCountry) ? t('admin.settings.concertsKeysHelp') : undefined}
              />
            ) : null}
            {concertProviderApplies('Skiddle', concertsCountry) ? (
              <Field
                label={t('admin.settings.skiddleKey')}
                autoCapitalize="none"
                autoCorrect={false}
                placeholder={concerts.data?.skiddleConfigured ? t('admin.settings.keyConfigured') : t('admin.settings.keyNotConfigured')}
                value={skKey}
                onChangeText={setSkKey}
                help={t('admin.settings.concertsKeysHelp')}
              />
            ) : null}
            <SaveButton
              loading={updateConcerts.isPending}
              onPress={() =>
                updateConcerts.mutate(
                  {
                    enabled: concertsEnabled,
                    country: concertsCountry,
                    ...(tmKey ? { ticketmasterApiKey: tmKey } : {}),
                    ...(skKey ? { skiddleApiKey: skKey } : {}),
                  },
                  {
                    onSuccess: () => {
                      setTmKey('');
                      setSkKey('');
                    },
                  },
                )
              }
            />
            <Button
              title={concertsSynced != null ? t('admin.settings.concertsSyncedCount', { count: concertsSynced }) : t('admin.settings.runConcertsSync')}
              icon="megaphone-outline"
              variant="secondary"
              loading={concertsSync.isPending}
              onPress={() => concertsSync.mutate(undefined, { onSuccess: (n) => setConcertsSynced(n) })}
            />
          </>
        ) : null}

        {sheet === 'lastfm' ? (
          <>
            <Text className="text-xs text-muted">{t('admin.settings.lastFmDescription')}</Text>
            <ToggleRow label={t('admin.settings.lastFmEnabled')} value={lastFmEnabled} onChange={setLastFmEnabled} />
            <Field
              label={t('admin.settings.lastFmApiKey')}
              autoCapitalize="none"
              autoCorrect={false}
              placeholder={lastFm.data?.configured ? t('admin.settings.keyConfigured') : t('admin.settings.keyNotConfigured')}
              value={lastFmKey}
              onChangeText={setLastFmKey}
            />
            <Field
              label={t('admin.settings.lastFmApiSecret')}
              autoCapitalize="none"
              autoCorrect={false}
              placeholder={lastFm.data?.configured ? t('admin.settings.keyConfigured') : t('admin.settings.keyNotConfigured')}
              value={lastFmSecret}
              onChangeText={setLastFmSecret}
              help={t('admin.settings.lastFmKeysHelp')}
            />
            <SaveButton
              loading={updateLastFm.isPending}
              onPress={() =>
                updateLastFm.mutate(
                  {
                    enabled: lastFmEnabled,
                    ...(lastFmKey ? { apiKey: lastFmKey } : {}),
                    ...(lastFmSecret ? { apiSecret: lastFmSecret } : {}),
                  },
                  {
                    onSuccess: () => {
                      setLastFmKey('');
                      setLastFmSecret('');
                    },
                  },
                )
              }
            />
          </>
        ) : null}
      </>
    );
  }
}

function SettingSection({
  section,
  open,
  onToggle,
  children,
}: {
  section: Section;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  const colors = useColors();
  return (
    <Card className="gap-3">
      <Pressable
        onPress={onToggle}
        accessibilityRole="button"
        accessibilityState={{ expanded: open }}
        className="flex-row items-center gap-3 active:opacity-70"
      >
        <View className="h-10 w-10 items-center justify-center rounded-xl" style={{ backgroundColor: section.color + '22' }}>
          <Ionicon name={section.icon} size={20} color={section.color} />
        </View>
        <View className="flex-1">
          <Text className="text-base font-semibold text-foreground">{section.title}</Text>
          <Text className="text-xs text-muted">{section.subtitle}</Text>
        </View>
        <Ionicon name={open ? 'chevron-up' : 'chevron-down'} size={18} color={colors.muted} />
      </Pressable>
      {open ? <View className="gap-3 border-t border-border pt-3">{children}</View> : null}
    </Card>
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
