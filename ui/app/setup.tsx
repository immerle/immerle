import { useEffect, useState } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Redirect, router } from 'expo-router';
import { getSetupStatus, initSetup, SetupFieldError } from '../src/api/setup';
import { useSelfServer } from '../src/api/selfServer';
import { useAuth } from '../src/auth/store';
import { normalizeServerUrl } from '../src/utils/serverUrl';
import { Button, ErrorState, Field, Loading } from '../src/components/ui';
import { AuthShell } from '../src/components/AuthShell';
import { Ionicon } from '../src/components/Ionicon';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

type Phase = 'url' | 'loading' | 'form' | 'success' | 'done' | 'error';

const USERNAME_RE = /^[a-zA-Z0-9_.\-]{1,64}$/;
const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

type FieldKey = 'setupToken' | 'username' | 'displayName' | 'password' | 'confirm' | 'email';

/**
 * First-run server setup — creating the very first admin account on a Immerle
 * backend. This screen lives in the app (not the server) and talks to the
 * public `/setup/{status,init}` endpoints of whichever server URL the user
 * points at. It implements the spec's state machine, mirrors server-side
 * validation, surfaces field errors, warns on insecure transport, and persists
 * nothing locally.
 */
export default function Setup() {
  const t = useT();
  const colors = useColors();
  const [phase, setPhase] = useState<Phase>('url');
  // Pre-fill with the origin when this app is served by its own Immerle binary.
  const selfUrl = useSelfServer((s) => s.url);
  const [serverUrl, setServerUrl] = useState(() => useSelfServer.getState().url ?? '');
  // Landing straight on /setup can beat the async self-probe; fill it in when it
  // resolves, but never clobber what the user has already typed.
  useEffect(() => {
    if (selfUrl) setServerUrl((cur) => cur || selfUrl);
  }, [selfUrl]);
  const [tokenRequired, setTokenRequired] = useState(false);
  const [reveal, setReveal] = useState(false);

  const [values, setValues] = useState({
    setupToken: '',
    username: '',
    displayName: '',
    password: '',
    confirm: '',
    email: '',
  });
  const [errors, setErrors] = useState<Partial<Record<FieldKey, string>>>({});
  const [globalError, setGlobalError] = useState<string | null>(null);
  const [createdUser, setCreatedUser] = useState('');
  const authed = useAuth((s) => s.status) === 'authenticated';

  // Already signed in: setup is first-run only, send them into the app.
  if (authed) return <Redirect href="/(tabs)" />;

  const set = (key: FieldKey, v: string) => setValues((s) => ({ ...s, [key]: v }));

  const normalizedUrl = serverUrl ? normalizeServerUrl(serverUrl) : '';
  const insecure = isInsecure(normalizedUrl);

  // --- status probe (state machine entry) ---
  const probe = async () => {
    if (!serverUrl.trim()) return;
    setPhase('loading');
    setGlobalError(null);
    try {
      const status = await getSetupStatus(serverUrl);
      if (status.initialized) {
        setPhase('done');
        return;
      }
      setTokenRequired(!!status.setupTokenRequired);
      setPhase('form');
    } catch {
      setPhase('error');
    }
  };

  // --- client validation (mirrors the server) ---
  const validate = (): Partial<Record<FieldKey, string>> => {
    const e: Partial<Record<FieldKey, string>> = {};
    if (tokenRequired && !values.setupToken.trim()) e.setupToken = t('auth.setup.errTokenRequired');
    if (!USERNAME_RE.test(values.username.trim()))
      e.username = t('auth.setup.usernameHelp');
    if (values.password.length < 8) e.password = t('auth.setup.passwordHelp');
    if (values.confirm !== values.password) e.confirm = t('auth.setup.errPasswordMismatch');
    if (values.email.trim() && !EMAIL_RE.test(values.email.trim()))
      e.email = t('auth.setup.errEmailInvalid');
    return e;
  };

  const submit = async () => {
    if (phase === 'loading') return;
    setGlobalError(null);
    const e = validate();
    setErrors(e);
    if (Object.keys(e).length > 0) return;

    setPhase('loading');
    const result = await initSetup(serverUrl, {
      username: values.username.trim(),
      displayName: values.displayName.trim() || undefined,
      password: values.password,
      email: values.email.trim() || undefined,
      setupToken: tokenRequired ? values.setupToken.trim() : undefined,
    }).catch(() => null);

    if (!result) {
      setPhase('form');
      setGlobalError(t('auth.setup.errNetwork'));
      return;
    }
    if (result.ok) {
      setCreatedUser(result.user.username ?? values.username.trim());
      useSelfServer.setState({ needsSetup: false }); // server is configured now
      setPhase('success');
      return;
    }
    // Map server errors back to the form.
    if (result.status === 409) {
      useSelfServer.setState({ needsSetup: false });
      setPhase('done');
      return;
    }
    if (result.status === 401) {
      setErrors({ setupToken: t('errors.invalid_setup_token') });
      setPhase('form');
      return;
    }
    if (result.status === 400 && Array.isArray(result.details)) {
      const mapped: Partial<Record<FieldKey, string>> = {};
      const unmapped: string[] = [];
      result.details.forEach((d: SetupFieldError) => {
        if (d.field && isFieldKey(d.field)) mapped[d.field] = d.message ?? t('auth.setup.errFieldInvalid');
        else unmapped.push(d.message ?? t('auth.setup.errFieldInvalid'));
      });
      setErrors(mapped);
      if (unmapped.length) setGlobalError(unmapped.join(' '));
      setPhase('form');
      return;
    }
    setGlobalError(t(`errors.${result.error}`, { defaultValue: t('auth.setup.errCreateFailed') }));
    setPhase('form');
  };

  const goLogin = () => router.replace('/login');

  const subtitle =
    phase === 'url'
      ? t('auth.setup.subtitleUrl')
      : phase === 'form'
        ? t('auth.setup.subtitleForm')
        : undefined;

  return (
    <AuthShell
      subtitle={subtitle}
      footer={
        phase === 'url' ? (
          <Pressable onPress={goLogin} className="active:opacity-60">
            <Text className="text-sm font-medium text-primary">{t('auth.setup.haveAccount')}</Text>
          </Pressable>
        ) : null
      }
    >
      {phase === 'loading' ? (
            <Loading label={t('auth.setup.contacting')} />
          ) : phase === 'error' ? (
            <View className="gap-3">
              <ErrorState message={t('auth.setup.contactError')} onRetry={probe} />
              <Button title={t('auth.setup.changeAddress')} variant="secondary" onPress={() => setPhase('url')} />
            </View>
          ) : phase === 'success' ? (
            <SuccessScreen username={createdUser} url={normalizedUrl} onLogin={goLogin} />
          ) : phase === 'done' ? (
            <DoneScreen url={normalizedUrl} onLogin={goLogin} />
          ) : phase === 'url' ? (
            <View className="gap-4">
              <Field
                label={t('auth.setup.serverAddressLabel')}
                icon="globe-outline"
                placeholder="https://musique.exemple.fr"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                inputMode="url"
                value={serverUrl}
                onChangeText={setServerUrl}
                onSubmitEditing={probe}
                help={t('auth.setup.serverAddressHelp')}
              />
              <Button title={t('auth.setup.continue')} icon="arrow-forward" disabled={!serverUrl.trim()} onPress={probe} />
            </View>
          ) : (
            // phase === 'form'
            <View className="gap-4">
              {insecure ? (
                <Banner
                  tone="warn"
                  text={t('auth.setup.insecureWarning')}
                />
              ) : null}
              {globalError ? <Banner tone="error" text={globalError} /> : null}

              {tokenRequired ? (
                <Field
                  label={t('auth.setup.tokenLabel')}
                  icon="key-outline"
                  placeholder={t('auth.setup.tokenPlaceholder')}
                  autoCapitalize="none"
                  autoCorrect={false}
                  autoComplete="off"
                  value={values.setupToken}
                  onChangeText={(v) => set('setupToken', v)}
                  error={errors.setupToken}
                  help={t('auth.setup.tokenHelp')}
                />
              ) : null}

              <Field
                label={t('auth.setup.usernameLabel')}
                icon="person-outline"
                placeholder="admin"
                autoCapitalize="none"
                autoCorrect={false}
                autoComplete="username"
                value={values.username}
                onChangeText={(v) => set('username', v)}
                error={errors.username}
                help={t('auth.setup.usernameHelp')}
              />

              <Field
                label={t('auth.setup.displayNameLabel')}
                icon="happy-outline"
                placeholder="Jean Dupont"
                value={values.displayName}
                onChangeText={(v) => set('displayName', v)}
                error={errors.displayName}
              />

              <Field
                label={t('auth.setup.passwordLabel')}
                icon="lock-closed-outline"
                placeholder="••••••••"
                secureTextEntry={!reveal}
                autoComplete="new-password"
                value={values.password}
                onChangeText={(v) => set('password', v)}
                error={errors.password}
                help={t('auth.setup.passwordHelp')}
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

              <Field
                label={t('auth.setup.confirmLabel')}
                icon="lock-closed-outline"
                placeholder="••••••••"
                secureTextEntry={!reveal}
                autoComplete="new-password"
                value={values.confirm}
                onChangeText={(v) => set('confirm', v)}
                error={errors.confirm}
              />

              <Field
                label={t('auth.setup.emailLabel')}
                icon="mail-outline"
                placeholder="admin@exemple.fr"
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
                autoComplete="email"
                value={values.email}
                onChangeText={(v) => set('email', v)}
                error={errors.email}
              />

              <Button title={t('auth.setup.createAdmin')} icon="shield-checkmark" onPress={submit} />
              <Pressable onPress={() => setPhase('url')} className="items-center py-1 active:opacity-60">
                <Text className="text-sm text-muted">{t('auth.setup.changeServerAddress')}</Text>
              </Pressable>
            </View>
          )}
    </AuthShell>
  );
}

function Banner({ tone, text }: { tone: 'warn' | 'error'; text: string }) {
  const cls = tone === 'warn' ? 'bg-accent/10' : 'bg-danger/10';
  const txt = tone === 'warn' ? 'text-foreground' : 'text-danger';
  return (
    <View className={`rounded-xl p-3 ${cls}`} accessibilityLiveRegion="polite" role="alert">
      <Text className={`text-sm ${txt}`}>{text}</Text>
    </View>
  );
}

function SuccessScreen({ username, url, onLogin }: { username: string; url: string; onLogin: () => void }) {
  const t = useT();
  const colors = useColors();
  return (
    <View className="items-center gap-3">
      <View className="h-16 w-16 items-center justify-center rounded-full bg-success/15">
        <Ionicon name="checkmark" size={36} color={colors.success} />
      </View>
      <Text className="text-xl font-bold text-foreground">{t('auth.setup.adminCreated')}</Text>
      <Text className="text-center text-base text-muted">
        {t('auth.setup.accountPrefix')} <Text className="font-semibold text-foreground">{username}</Text> {t('auth.setup.accountSuffix')}
      </Text>
      <Text className="text-center text-sm text-muted">
        {t('auth.setup.connectClient')}
      </Text>
      <Text selectable className="rounded-lg bg-surface-alt px-3 py-2 text-sm text-foreground">{url}</Text>
      <View className="w-full pt-2">
        <Button title={t('auth.setup.loginNow')} icon="log-in-outline" onPress={onLogin} />
      </View>
    </View>
  );
}

function DoneScreen({ url, onLogin }: { url: string; onLogin: () => void }) {
  const t = useT();
  const colors = useColors();
  return (
    <View className="items-center gap-3">
      <View className="h-16 w-16 items-center justify-center rounded-full bg-surface-alt">
        <Ionicon name="checkmark-done" size={32} color={colors.muted} />
      </View>
      <Text className="text-xl font-bold text-foreground">{t('auth.setup.alreadyConfigured')}</Text>
      <Text className="text-center text-sm text-muted">{t('auth.setup.alreadyHasAdmin')}</Text>
      <Text selectable className="rounded-lg bg-surface-alt px-3 py-2 text-sm text-foreground">{url}</Text>
      <View className="w-full pt-2">
        <Button title={t('auth.setup.login')} icon="log-in-outline" onPress={onLogin} />
      </View>
    </View>
  );
}

function isFieldKey(field: string): field is FieldKey {
  return ['setupToken', 'username', 'password', 'confirm', 'email'].includes(field);
}

/** http:// on a non-local host means the password would travel in clear text. */
function isInsecure(url: string): boolean {
  if (!url.startsWith('http://')) return false;
  const host = url.replace(/^http:\/\//, '').split(/[/:]/)[0];
  return host !== 'localhost' && host !== '127.0.0.1' && host !== '::1' && host !== '[::1]';
}
