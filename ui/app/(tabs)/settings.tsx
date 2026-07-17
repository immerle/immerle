import { useEffect, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useAuth } from '../../src/auth/store';
import { useTheme, ThemePreference } from '../../src/theme/store';
import { useLocale, LocalePref } from '../../src/i18n/store';
import { usePlayer } from '../../src/audio/store';
import { useAccount, useUpdateAccount } from '../../src/query/account';
import { QUALITY_PRESETS } from '../../src/audio/quality';
import { Button, Card, Field, Select } from '../../src/components/ui';
import { AdminHeader, AdminScroll, CardTitle, colorFor } from '../../src/components/AdminUI';
import { Ionicon } from '../../src/components/Ionicon';
import { ACCENT_PRESETS, DEFAULT_ACCENT, normalizeHex } from '../../src/theme/accent';
import { useColors } from '../../src/theme/colors';
import { isSupported as offlineSupported } from '../../src/offline/fs';
import { t } from '../../src/i18n';

const THEME_OPTIONS: { key: ThemePreference; icon: string }[] = [
  { key: 'light', icon: 'sunny' },
  { key: 'dark', icon: 'moon' },
  { key: 'system', icon: 'phone-portrait' },
];

/** User settings: account, appearance, accent colour, playback quality, access. */
export default function Settings() {
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const logout = useAuth((s) => s.logout);
  const themePref = useTheme((s) => s.preference);
  const setTheme = useTheme((s) => s.setPreference);
  const accent = useTheme((s) => s.accent);
  const setAccent = useTheme((s) => s.setAccent);
  const qualityId = usePlayer((s) => s.qualityId);
  const setQuality = usePlayer((s) => s.setQuality);
  const localePref = useLocale((s) => s.preference);
  const setLocale = useLocale((s) => s.setPreference);
  // `system` is translated; the fixed locales keep their endonym.
  const langOptions: { value: LocalePref; label: string }[] = [
    { value: 'system', label: t('settings.system') },
    { value: 'en', label: 'English' },
    { value: 'fr', label: 'Français' },
  ];
  const [customHex, setCustomHex] = useState('');

  const displayNameState = useAuth((s) => s.displayName);
  const account = useAccount();
  const updateAccount = useUpdateAccount();
  const [editName, setEditName] = useState('');
  const [editEmail, setEditEmail] = useState('');
  const [editCity, setEditCity] = useState('');
  useEffect(() => {
    if (account.data) {
      setEditName(account.data.displayName);
      setEditEmail(account.data.email);
      setEditCity(account.data.city);
      // Server is the cross-device source of truth ("" → follow device); AsyncStorage is just the offline fallback.
      setLocale(account.data.language || 'system');
    }
  }, [account.data, setLocale]);

  // Apply the language instantly (local) and persist it server-side.
  const onChangeLocale = (p: LocalePref) => {
    setLocale(p);
    updateAccount.mutate({ language: p === 'system' ? '' : p });
  };

  const currentAccent = accent ?? DEFAULT_ACCENT;
  const username = client?.username ?? '?';
  const displayName = displayNameState ?? username;
  const applyCustom = () => {
    const hex = normalizeHex(customHex);
    if (hex) {
      setAccent(hex);
      setCustomHex('');
    }
  };

  const onLogout = async () => {
    await logout();
    router.replace('/login');
  };

  return (
    <AdminScroll
      header={<AdminHeader color={colors.primary} title={t('settings.title')} subtitle={t('settings.subtitle')} showBack={false} />}
    >
      <Card>
        <View className="flex-row items-center gap-3">
          <View className="h-12 w-12 items-center justify-center rounded-full" style={{ backgroundColor: colorFor(username) }}>
            <Text className="text-lg font-bold text-white">{displayName.charAt(0).toUpperCase()}</Text>
          </View>
          <View className="flex-1">
            <Text className="text-base font-semibold text-foreground">{displayName}</Text>
            <Text className="text-sm text-muted" numberOfLines={1}>
              {displayName !== username ? `@${username} · ` : ''}
              {client?.serverUrl}
            </Text>
          </View>
          {client?.isAdmin ? (
            <View className="rounded-full bg-success/15 px-2.5 py-1">
              <Text className="text-xs font-semibold text-success">{t('settings.admin')}</Text>
            </View>
          ) : null}
        </View>
      </Card>

      {/* Self-service: display name + email only, not admin user management. */}
      <Card className="gap-3">
        <CardTitle icon="person" color="#14b8a6" title={t('settings.profile')} />
        <Field label={t('settings.displayName')} placeholder={username} value={editName} onChangeText={setEditName} />
        <Field
          label={t('settings.email')}
          placeholder={t('settings.emailPlaceholder')}
          keyboardType="email-address"
          autoCapitalize="none"
          autoCorrect={false}
          value={editEmail}
          onChangeText={setEditEmail}
        />
        <Field
          label={t('settings.city')}
          placeholder={t('settings.cityPlaceholder')}
          help={t('settings.cityHelp')}
          value={editCity}
          onChangeText={setEditCity}
        />
        <View className="gap-1.5">
          <Text className="text-sm font-medium text-muted">{t('settings.language')}</Text>
          <Select value={localePref} options={langOptions} onChange={onChangeLocale} />
        </View>
        <View className="flex-row justify-end">
          <Button
            title={t('settings.save')}
            size="sm"
            icon="save-outline"
            loading={updateAccount.isPending}
            onPress={() => updateAccount.mutate({ displayName: editName.trim(), email: editEmail.trim(), city: editCity.trim() })}
          />
        </View>
      </Card>

      <Card className="gap-3">
        <CardTitle icon="contrast" color="#3b82f6" title={t('settings.appearance')} />
        <View className="flex-row gap-2">
          {THEME_OPTIONS.map((opt) => {
            const active = themePref === opt.key;
            return (
              <Pressable
                key={opt.key}
                onPress={() => setTheme(opt.key)}
                className={`flex-1 items-center gap-1 rounded-xl border p-3 ${
                  active ? 'border-primary bg-primary/10' : 'border-border bg-surface-alt'
                }`}
              >
                <Ionicon name={opt.icon} size={22} color={active ? colors.primary : colors.muted} />
                <Text className={`text-sm ${active ? 'font-semibold text-primary' : 'text-muted'}`}>{t(`settings.theme.${opt.key}`)}</Text>
              </Pressable>
            );
          })}
        </View>
      </Card>

      <Card className="gap-3">
        <CardTitle icon="color-palette" color={currentAccent} title={t('settings.accent')} />
        <View className="flex-row flex-wrap gap-3">
          {ACCENT_PRESETS.map((p) => {
            const active = currentAccent.toLowerCase() === p.hex.toLowerCase();
            return (
              <Pressable
                key={p.id}
                onPress={() => setAccent(p.id === 'green' ? null : p.hex)}
                accessibilityLabel={p.label}
                className="h-10 w-10 items-center justify-center rounded-full"
                style={{ backgroundColor: p.hex, borderWidth: active ? 3 : 0, borderColor: colors.foreground }}
              >
                {active ? <Ionicon name="checkmark" size={18} color="#fff" /> : null}
              </Pressable>
            );
          })}
        </View>
        <View className="flex-row items-end gap-2">
          <View className="flex-1">
            <Field
              label={t('settings.customColor')}
              placeholder="#1ed760"
              autoCapitalize="none"
              autoCorrect={false}
              value={customHex}
              onChangeText={setCustomHex}
              onSubmitEditing={applyCustom}
            />
          </View>
          <View className="mb-0.5 h-11 w-11 rounded-xl border border-border" style={{ backgroundColor: normalizeHex(customHex) ?? currentAccent }} />
          <Button title={t('settings.apply')} disabled={!normalizeHex(customHex)} onPress={applyCustom} />
        </View>
        {accent ? <Button title={t('settings.resetAccent')} variant="ghost" onPress={() => setAccent(null)} /> : null}
      </Card>

      <Card className="gap-3">
        <CardTitle icon="musical-notes" color="#ec4899" title={t('settings.quality')} />
        <View className="overflow-hidden rounded-xl border border-border">
          {QUALITY_PRESETS.map((p, i) => {
            const active = p.id === qualityId;
            return (
              <Pressable
                key={p.id}
                onPress={() => setQuality(p.id)}
                className={`flex-row items-center justify-between px-3 py-2.5 ${i > 0 ? 'border-t border-border' : ''} ${active ? 'bg-primary/10' : 'active:bg-surface-alt'}`}
              >
                <Text className={`text-base ${active ? 'font-semibold text-primary' : 'text-foreground'}`}>{p.label}</Text>
                {active ? <Ionicon name="checkmark" size={20} color={colors.primary} /> : null}
              </Pressable>
            );
          })}
        </View>
      </Card>

      <Card className="gap-2">
        <CardTitle icon="key" color="#14b8a6" title={t('settings.access')} />
        {client?.isFeatureEnabled('wrapped') ? (
          <NavRow icon="sparkles-outline" title={t('wrapped.entryTitle')} subtitle={t('wrapped.entrySubtitle')} onPress={() => router.push('/wrapped' as never)} />
        ) : null}
        {client?.has('playlistImport') ? (
          <NavRow icon="cloud-download-outline" title={t('settings.importTitle')} subtitle={t('settings.importSubtitle')} onPress={() => router.push('/import' as never)} />
        ) : null}
        {offlineSupported && client?.isFeatureEnabled('offlineDownloads') ? (
          <NavRow icon="cloud-offline-outline" title={t('offline.title')} subtitle={t('offline.subtitle')} onPress={() => router.push('/offline' as never)} />
        ) : null}
        <NavRow icon="phone-portrait-outline" title={t('settings.devicesTitle')} subtitle={t('settings.devicesSubtitle')} onPress={() => router.push('/devices' as never)} />
        <NavRow icon="key-outline" title={t('settings.apiTitle')} subtitle={t('settings.apiSubtitle')} onPress={() => router.push('/api-tokens' as never)} />
      </Card>

      <View className="pt-2">
        <Button title={t('settings.logout')} variant="danger" icon="log-out-outline" onPress={onLogout} />
      </View>
    </AdminScroll>
  );
}

function NavRow({ icon, title, subtitle, onPress }: { icon: string; title: string; subtitle: string; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-3 rounded-xl bg-surface-alt px-3 py-2.5 active:opacity-70">
      <View className="h-9 w-9 items-center justify-center rounded-lg bg-primary/15">
        <Ionicon name={icon} size={20} color={colors.primary} />
      </View>
      <View className="flex-1">
        <Text className="text-base font-semibold text-foreground">{title}</Text>
        <Text className="text-xs text-muted">{subtitle}</Text>
      </View>
      <Ionicon name="chevron-forward" size={18} color={colors.muted} />
    </Pressable>
  );
}
