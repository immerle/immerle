import { useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useAuth } from '../src/auth/store';
import { useSelfServer } from '../src/api/selfServer';
import { Button, Field } from '../src/components/ui';
import { AuthShell } from '../src/components/AuthShell';
import { Ionicon } from '../src/components/Ionicon';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Subsonic / Immerle login. Verifies the credentials against the live
 * instance (ping) before persisting them to secure storage, then enters the
 * app. The raw password never leaves this screen — only the derived salted
 * token is stored.
 */
export default function Login() {
  const t = useT();
  const colors = useColors();
  const login = useAuth((s) => s.login);
  const error = useAuth((s) => s.error);

  // Pre-fill with the origin when this app is served by its own Immerle binary.
  const [serverUrl, setServerUrl] = useState(() => useSelfServer.getState().url ?? '');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [reveal, setReveal] = useState(false);
  const [busy, setBusy] = useState(false);

  const canSubmit = !!(serverUrl.trim() && username.trim() && password.length > 0);

  const onSubmit = async () => {
    if (!canSubmit) return;
    setBusy(true);
    try {
      await login({ serverUrl, username, password });
      router.replace('/(tabs)');
    } catch {
      // Error surfaced via the auth store.
    } finally {
      setBusy(false);
    }
  };

  return (
    <AuthShell
      subtitle={t('auth.login.subtitle')}
      footer={
        <>
          <Pressable onPress={() => router.push('/setup' as never)} className="active:opacity-60">
            <Text className="text-sm font-medium text-primary">{t('auth.login.setupLink')}</Text>
          </Pressable>
          <Text className="max-w-[320px] text-center text-xs text-muted">
            {t('auth.login.compatNote')}
          </Text>
        </>
      }
    >
      <Field
        label={t('auth.login.serverLabel')}
        icon="globe-outline"
        placeholder="https://musique.exemple.fr"
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        inputMode="url"
        value={serverUrl}
        onChangeText={setServerUrl}
      />
      <Field
        label={t('auth.login.usernameLabel')}
        icon="person-outline"
        placeholder={t('auth.login.usernamePlaceholder')}
        autoCapitalize="none"
        autoCorrect={false}
        autoComplete="username"
        value={username}
        onChangeText={setUsername}
      />
      <Field
        label={t('auth.login.passwordLabel')}
        icon="lock-closed-outline"
        placeholder="••••••••"
        secureTextEntry={!reveal}
        autoComplete="current-password"
        value={password}
        onChangeText={setPassword}
        onSubmitEditing={onSubmit}
        trailing={
          <Pressable
            onPress={() => setReveal((r) => !r)}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityLabel={reveal ? t('auth.passwordHide') : t('auth.passwordShow')}
          >
            <Ionicon name={reveal ? 'eye-off-outline' : 'eye-outline'} size={20} color={colors.muted} />
          </Pressable>
        }
      />

      {error ? (
        <View className="flex-row items-center gap-2 rounded-xl bg-danger/10 p-3" accessibilityLiveRegion="polite" role="alert">
          <Ionicon name="alert-circle" size={18} color={colors.danger} />
          <Text className="flex-1 text-sm text-danger">{error}</Text>
        </View>
      ) : null}

      <Button title={t('auth.login.submit')} icon="log-in-outline" loading={busy} disabled={!canSubmit} onPress={onSubmit} />
    </AuthShell>
  );
}
