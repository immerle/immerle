import { useEffect, useState } from 'react';
import { Modal, Pressable, ScrollView, Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useCleanup,
  useCleanupMutations,
  useSettings,
  useUpdateSettings,
} from '../../src/query/admin';
import { RuntimeSettingsDTO } from '../../src/api/immerleApi';
import { Badge, Button, Card, ErrorState, Field, IconButton, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { useColors } from '../../src/theme/colors';

const num = (s: string) => {
  const n = Number(s);
  return Number.isFinite(n) ? n : 0;
};

const RESTART_LABELS: Record<string, string> = {
  transcode: 'Transcodage',
  'scan.watch': 'Surveillance de la bibliothèque',
  federation: 'Fédération',
};

type SectionKey = 'auth' | 'server' | 'transcode' | 'federation' | 'cleanup';

interface Section {
  key: SectionKey;
  icon: string;
  color: string;
  title: string;
  subtitle: string;
}

const SECTIONS: Section[] = [
  { key: 'auth', icon: 'key', color: '#3b82f6', title: 'Authentification', subtitle: "Durée des tokens d'appareil" },
  { key: 'server', icon: 'server', color: '#0ea5e9', title: 'Serveur', subtitle: 'Origines CORS autorisées' },
  { key: 'transcode', icon: 'film', color: '#a855f7', title: 'Transcodage', subtitle: 'ffmpeg / ffprobe & profils' },
  { key: 'federation', icon: 'git-network', color: '#14b8a6', title: 'Fédération', subtitle: 'Hub & synchronisation' },
  { key: 'cleanup', icon: 'trash-bin', color: '#ef4444', title: 'Nettoyage', subtitle: 'Purge des téléchargements' },
];

interface Form {
  cors: string;
  ttl: string;
  ffmpeg: string;
  ffprobe: string;
  fedEnabled: boolean;
  hubUrl: string;
  publicKey: string;
  privateKey: string;
  syncInterval: string;
  resolveMissing: boolean;
  exportScrobbles: boolean;
}

function toForm(s: RuntimeSettingsDTO): Form {
  return {
    cors: (s.server?.corsAllowedOrigins ?? []).join(', '),
    ttl: String(s.auth?.deviceTokenTtlSeconds ?? 0),
    ffmpeg: s.transcode?.ffmpegPath ?? '',
    ffprobe: s.transcode?.ffprobePath ?? '',
    fedEnabled: s.federation?.enabled ?? false,
    hubUrl: s.federation?.hubUrl ?? '',
    publicKey: s.federation?.publicKey ?? '',
    privateKey: s.federation?.privateKey ?? '',
    syncInterval: String(s.federation?.syncIntervalSeconds ?? 0),
    resolveMissing: s.federation?.resolveMissing ?? false,
    exportScrobbles: s.federation?.exportScrobbles ?? false,
  };
}

/** Runtime settings admin. Each group opens in a bottom sheet; hot-reload fields
 * apply instantly, restart-only changes raise a "restart pending" banner. */
export default function AdminSettings() {
  const colors = useColors();
  const q = useSettings();
  const update = useUpdateSettings();
  const cleanup = useCleanup();
  const cleanupM = useCleanupMutations();
  const [form, setForm] = useState<Form | null>(null);
  const [sheet, setSheet] = useState<SectionKey | null>(null);
  const [removed, setRemoved] = useState<number | null>(null);

  useEffect(() => {
    if (q.data?.settings) setForm(toForm(q.data.settings));
  }, [q.data?.settings]);

  if (q.isLoading || !form) return <Loading />;
  if (q.isError) return <ErrorState message="Impossible de charger les réglages." onRetry={q.refetch} />;

  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const save = (patch: RuntimeSettingsDTO) => update.mutate(patch);

  const pending = q.data?.pendingRestart ?? [];
  const profiles = q.data?.settings.transcode?.profiles ?? [];
  const rows = SECTIONS.filter((s) => s.key !== 'cleanup' || !!cleanup.data);
  const active = SECTIONS.find((s) => s.key === sheet);

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#0ea5e9" title="Réglages" subtitle="Configuration runtime de l'instance" />}
      >
        {/* Restart-pending banner */}
        {pending.length ? (
          <View className="gap-1.5 rounded-2xl border border-danger/40 bg-danger/10 p-4">
            <View className="flex-row items-center gap-2">
              <Ionicon name="warning" size={18} color="#ef4444" />
              <Text className="text-sm font-semibold text-foreground">Redémarrage requis</Text>
            </View>
            <Text className="text-xs text-muted">
              Des changements ne prendront effet qu'après un redémarrage de l'instance :{' '}
              {pending.map((p) => RESTART_LABELS[p] ?? p).join(', ')}.
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
                <IconButton name="close" color={colors.muted} onPress={() => setSheet(null)} accessibilityLabel="Fermer" />
              </View>
            ) : null}

            <ScrollView contentContainerStyle={{ paddingHorizontal: 20, paddingTop: 4, paddingBottom: 8, gap: 12 }} keyboardShouldPersistTaps="handled">
              {sheet === 'auth' ? (
                <>
                  <Field label="Durée des tokens d'appareil (s, 0 = jamais)" keyboardType="number-pad" value={form.ttl} onChangeText={(v) => set('ttl', v)} />
                  <SaveButton loading={update.isPending} onPress={() => save({ auth: { deviceTokenTtlSeconds: num(form.ttl) } })} />
                </>
              ) : null}

              {sheet === 'server' ? (
                <>
                  <Field label="Origines CORS autorisées (séparées par des virgules)" autoCapitalize="none" placeholder="*" value={form.cors} onChangeText={(v) => set('cors', v)} help="* pour tout autoriser." />
                  <SaveButton
                    loading={update.isPending}
                    onPress={() => save({ server: { corsAllowedOrigins: form.cors.split(',').map((o) => o.trim()).filter(Boolean) } })}
                  />
                </>
              ) : null}

              {sheet === 'transcode' ? (
                <>
                  <Field label="Chemin ffmpeg" autoCapitalize="none" value={form.ffmpeg} onChangeText={(v) => set('ffmpeg', v)} />
                  <Field label="Chemin ffprobe" autoCapitalize="none" value={form.ffprobe} onChangeText={(v) => set('ffprobe', v)} />
                  {profiles.length ? (
                    <View className="gap-1.5 rounded-xl bg-surface-alt p-3">
                      <Text className="text-xs font-medium uppercase tracking-wider text-muted">Profils</Text>
                      {profiles.map((p, i) => (
                        <Text key={i} className="text-sm text-foreground">
                          {p.name} · {p.format?.toUpperCase()} · {p.bitRate} kbps
                        </Text>
                      ))}
                    </View>
                  ) : null}
                  <SaveButton loading={update.isPending} hint="Redémarrage" onPress={() => save({ transcode: { ffmpegPath: form.ffmpeg, ffprobePath: form.ffprobe } })} />
                </>
              ) : null}

              {sheet === 'federation' ? (
                <>
                  <ToggleRow label="Fédération activée" value={form.fedEnabled} onChange={(v) => set('fedEnabled', v)} />
                  <Field label="URL du hub" autoCapitalize="none" keyboardType="url" placeholder="https://hub.immerle.fr" value={form.hubUrl} onChangeText={(v) => set('hubUrl', v)} />
                  <Field label="Clé publique" autoCapitalize="none" autoCorrect={false} value={form.publicKey} onChangeText={(v) => set('publicKey', v)} help="Fournie par le hub à l'onboarding (en-tête X-Instance-ID)." />
                  <Field label="Clé privée" autoCapitalize="none" autoCorrect={false} secureTextEntry value={form.privateKey} onChangeText={(v) => set('privateKey', v)} help="Fournie par le hub à l'onboarding (Authorization: Bearer)." />
                  <Field label="Intervalle de synchro (s)" keyboardType="number-pad" value={form.syncInterval} onChangeText={(v) => set('syncInterval', v)} />
                  <ToggleRow label="Résoudre les titres manquants" value={form.resolveMissing} onChange={(v) => set('resolveMissing', v)} />
                  <ToggleRow label="Exporter les écoutes (scrobbles)" value={form.exportScrobbles} onChange={(v) => set('exportScrobbles', v)} />
                  <SaveButton
                    loading={update.isPending}
                    hint="Redémarrage"
                    onPress={() =>
                      save({
                        federation: {
                          enabled: form.fedEnabled,
                          hubUrl: form.hubUrl,
                          publicKey: form.publicKey,
                          privateKey: form.privateKey,
                          syncIntervalSeconds: num(form.syncInterval),
                          resolveMissing: form.resolveMissing,
                          exportScrobbles: form.exportScrobbles,
                        },
                      })
                    }
                  />
                </>
              ) : null}

              {sheet === 'cleanup' && cleanup.data ? (
                <>
                  <Text className="text-xs text-muted">
                    Purge les fichiers téléchargés de plus de {Math.round(cleanup.data.maxAgeSeconds / 86400)} j,
                    toutes les {Math.round(cleanup.data.intervalSeconds / 3600)} h.
                  </Text>
                  <ToggleRow label="Balayage automatique" value={cleanup.data.enabled} onChange={(v) => cleanupM.setEnabled.mutate(v)} />
                  <Button
                    title={removed != null ? `${removed} supprimé${removed > 1 ? 's' : ''}` : 'Lancer le nettoyage maintenant'}
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
  return (
    <View className="flex-row items-center justify-end gap-2 pt-1">
      {hint ? <Badge label={hint} tone="default" /> : null}
      <Button title="Enregistrer" icon="save-outline" loading={loading} onPress={onPress} />
    </View>
  );
}
