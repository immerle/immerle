import { Redirect } from 'expo-router';
import { useAuth } from '../src/auth/store';
import { useSelfServer } from '../src/api/selfServer';
import { Loading } from '../src/components/ui';
import { View } from 'react-native';
import { useT } from '../src/i18n/store';

/**
 * Auth gate. Sends the user to the tabs when a session is restored, otherwise
 * to login — or straight to setup when the server hosting this app still needs
 * first-run configuration. Shows a spinner while the session and the self-probe
 * are resolving.
 */
export default function Index() {
  const t = useT();
  const status = useAuth((s) => s.status);
  const checked = useSelfServer((s) => s.checked);
  const needsSetup = useSelfServer((s) => s.needsSetup);

  if (status === 'idle' || status === 'restoring' || !checked) {
    return (
      <View className="flex-1 bg-background">
        <Loading label={t('auth.index.loading')} />
      </View>
    );
  }

  if (status === 'authenticated') return <Redirect href="/(tabs)" />;
  if (needsSetup) return <Redirect href="/setup" />;
  return <Redirect href="/login" />;
}
