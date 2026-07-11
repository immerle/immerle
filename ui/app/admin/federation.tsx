import { useEffect, useState } from 'react';
import { Switch, Text, View } from 'react-native';
import { Stack } from 'expo-router';
import {
  useSettings,
  useUpdateSettings,
  useRegisterInstance,
  useUpdateFederationInstance,
  useFederationProfile,
  useUnlinkInstance,
  useFederationSearch,
  useSubscriptions,
  useSubscriptionMutations,
} from '../../src/query/admin';
import { RuntimeSettingsDTO } from '../../src/api/immerleApi';
import { InstanceSummary } from '../../src/api/immerle/types';
import { Button, Card, ErrorState, Field, Loading } from '../../src/components/ui';
import { AdminHeader, AdminScroll } from '../../src/components/AdminUI';
import { useColors } from '../../src/theme/colors';
import { useToast } from '../../src/stores/toast';
import { tError } from '../../src/i18n';
import { useT } from '../../src/i18n/store';

interface Form {
  userId: string;
  name: string;
  sqid: string;
  syncPlaylists: boolean;
  exportScrobbles: boolean;
}

function toForm(s: RuntimeSettingsDTO): Form {
  return {
    userId: s.federation?.userId ?? '',
    name: s.federation?.instanceName ?? '',
    sqid: s.federation?.sqid ?? '',
    syncPlaylists: s.federation?.syncPlaylists ?? false,
    exportScrobbles: s.federation?.exportScrobbles ?? false,
  };
}

/** Dedicated federation admin page: link/unlink the instance to the hub, edit
 * its name/slug, toggle features, and discover + follow other instances. */
export default function AdminFederation() {
  const t = useT();
  const q = useSettings();
  const update = useUpdateSettings();
  const register = useRegisterInstance();
  const updateInstance = useUpdateFederationInstance();
  const unlink = useUnlinkInstance();

  const instanceId = q.data?.settings.federation?.instanceId ?? '';
  const linked = !!instanceId;

  // Once linked, pull the live name/slug from the hub (source of truth).
  useFederationProfile(linked);

  const [form, setForm] = useState<Form | null>(null);
  useEffect(() => {
    if (q.data?.settings) setForm(toForm(q.data.settings));
  }, [q.data?.settings]);

  if (q.isLoading || !form) return <Loading />;
  if (q.isError) return <ErrorState message={t('admin.federation.loadError')} onRetry={q.refetch} />;

  const set = <K extends keyof Form>(k: K, v: Form[K]) => setForm((f) => (f ? { ...f, [k]: v } : f));
  const save = (patch: RuntimeSettingsDTO, onSuccess?: () => void) => update.mutate(patch, { onSuccess });

  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <AdminScroll
        header={<AdminHeader color="#14b8a6" title={t('admin.federation.title')} subtitle={t('admin.federation.subtitle')} />}
      >
        {!linked ? (
          <Card className="gap-3">
            <Text className="text-xs text-muted">{t('admin.federation.description')}</Text>
            <Field
              label={t('admin.federation.hubUserId')}
              autoCapitalize="none"
              autoCorrect={false}
              value={form.userId}
              onChangeText={(v) => set('userId', v)}
              help={t('admin.federation.hubUserIdHelp')}
            />
            <View className="flex-row justify-end">
              <Button
                title={t('admin.federation.link')}
                icon="link-outline"
                loading={register.isPending}
                disabled={!form.userId.trim()}
                onPress={() =>
                  save({ federation: { userId: form.userId.trim() } }, () =>
                    register.mutate(undefined, {
                      onSuccess: () => useToast.getState().success(t('admin.federation.linkSuccess')),
                      onError: (e) => useToast.getState().error(tError(e)),
                    }),
                  )
                }
              />
            </View>
          </Card>
        ) : (
          <>
            {/* Instance identity (pushed to the hub). */}
            <Card className="gap-3">
              <Text className="text-sm font-semibold text-foreground">{t('admin.federation.instanceSection')}</Text>
              <Field label={t('admin.federation.instanceName')} value={form.name} onChangeText={(v) => set('name', v)} />
              <Field
                label={t('admin.federation.instanceSlug')}
                autoCapitalize="none"
                autoCorrect={false}
                value={form.sqid}
                onChangeText={(v) => set('sqid', v)}
                help={t('admin.federation.instanceSlugHelp')}
              />
              <Text className="text-[11px] text-muted">{t('admin.federation.instanceUuid', { id: instanceId })}</Text>
              <View className="flex-row justify-end">
                <Button
                  title={t('admin.federation.save')}
                  icon="save-outline"
                  loading={updateInstance.isPending}
                  disabled={!form.name.trim() || !form.sqid.trim()}
                  onPress={() =>
                    updateInstance.mutate(
                      { name: form.name.trim(), sqid: form.sqid.trim() },
                      {
                        onSuccess: () => useToast.getState().success(t('admin.federation.saved')),
                        onError: (e) => useToast.getState().error(tError(e)),
                      },
                    )
                  }
                />
              </View>
            </Card>

            {/* Feature toggles (local), auto-saved. */}
            <Card className="gap-2">
              <Text className="text-sm font-semibold text-foreground">{t('admin.federation.features')}</Text>
              <ToggleRow
                label={t('admin.federation.syncPlaylists')}
                value={form.syncPlaylists}
                onChange={(v) => {
                  set('syncPlaylists', v);
                  save({ federation: { syncPlaylists: v } });
                }}
              />
              <ToggleRow
                label={t('admin.federation.exportScrobbles')}
                value={form.exportScrobbles}
                onChange={(v) => {
                  set('exportScrobbles', v);
                  save({ federation: { exportScrobbles: v } });
                }}
              />
            </Card>

            <DiscoverSection />
            <SubscriptionsSection enabled={linked} />

            <View className="flex-row justify-end pt-1">
              <Button
                title={t('admin.federation.unlink')}
                icon="trash-outline"
                variant="danger"
                loading={unlink.isPending}
                onPress={() => unlink.mutate()}
              />
            </View>
          </>
        )}
      </AdminScroll>
    </>
  );
}

/** Search the hub for other instances and follow them. */
function DiscoverSection() {
  const t = useT();
  const [query, setQuery] = useState('');
  const search = useFederationSearch(query);
  const subs = useSubscriptions(true);
  const { subscribe } = useSubscriptionMutations();
  const followed = new Set((subs.data ?? []).map((s) => s.id));

  return (
    <Card className="gap-3">
      <Text className="text-sm font-semibold text-foreground">{t('admin.federation.discoverTitle')}</Text>
      <Field
        label={t('admin.federation.discoverSearch')}
        autoCapitalize="none"
        autoCorrect={false}
        placeholder={t('admin.federation.discoverPlaceholder')}
        value={query}
        onChangeText={setQuery}
      />
      {query.trim().length > 0 ? (
        search.isLoading ? (
          <Loading />
        ) : (search.data ?? []).length === 0 ? (
          <Text className="text-xs text-muted">{t('admin.federation.discoverEmpty')}</Text>
        ) : (
          (search.data ?? []).map((inst) => (
            <InstanceRow
              key={inst.id}
              inst={inst}
              actionLabel={followed.has(inst.id) ? t('admin.federation.following') : t('admin.federation.follow')}
              actionIcon={followed.has(inst.id) ? 'checkmark' : 'add'}
              disabled={followed.has(inst.id) || subscribe.isPending}
              onAction={() =>
                subscribe.mutate(
                  { instanceId: inst.id },
                  {
                    onSuccess: () => useToast.getState().success(t('admin.federation.followed', { name: inst.name || inst.sqid })),
                    onError: (e) => useToast.getState().error(tError(e)),
                  },
                )
              }
            />
          ))
        )
      ) : null}
    </Card>
  );
}

/** The instances this one follows, with unfollow. */
function SubscriptionsSection({ enabled }: { enabled: boolean }) {
  const t = useT();
  const subs = useSubscriptions(enabled);
  const { unsubscribe } = useSubscriptionMutations();

  return (
    <Card className="gap-3">
      <Text className="text-sm font-semibold text-foreground">{t('admin.federation.subscriptionsTitle')}</Text>
      {subs.isLoading ? (
        <Loading />
      ) : (subs.data ?? []).length === 0 ? (
        <Text className="text-xs text-muted">{t('admin.federation.subscriptionsEmpty')}</Text>
      ) : (
        (subs.data ?? []).map((inst) => (
          <InstanceRow
            key={inst.id}
            inst={inst}
            actionLabel={t('admin.federation.unfollow')}
            actionIcon="remove"
            danger
            disabled={unsubscribe.isPending}
            onAction={() =>
              unsubscribe.mutate(inst.id, {
                onError: (e) => useToast.getState().error(tError(e)),
              })
            }
          />
        ))
      )}
    </Card>
  );
}

function InstanceRow({
  inst,
  actionLabel,
  actionIcon,
  onAction,
  disabled,
  danger,
}: {
  inst: InstanceSummary;
  actionLabel: string;
  actionIcon: string;
  onAction: () => void;
  disabled?: boolean;
  danger?: boolean;
}) {
  return (
    <View className="flex-row items-center gap-3 rounded-xl bg-surface-alt px-3 py-2">
      <View className="flex-1">
        <Text className="text-sm font-medium text-foreground">{inst.name || inst.sqid}</Text>
        <Text className="text-xs text-muted">
          {inst.sqid}
          {inst.region ? ` · ${inst.region}` : ''}
        </Text>
      </View>
      <Button title={actionLabel} icon={actionIcon} size="sm" variant={danger ? 'danger' : 'secondary'} disabled={disabled} onPress={onAction} />
    </View>
  );
}

function ToggleRow({ label, value, onChange }: { label: string; value: boolean; onChange: (v: boolean) => void }) {
  const colors = useColors();
  return (
    <View className="flex-row items-center justify-between rounded-xl bg-surface-alt px-3 py-2">
      <Text className="flex-1 pr-2 text-sm text-foreground">{label}</Text>
      <Switch value={value} onValueChange={onChange} trackColor={{ true: colors.primary, false: colors.border }} />
    </View>
  );
}
