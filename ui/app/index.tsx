import { Redirect } from 'expo-router';
import { useAuth } from '../src/auth/store';
import { Loading } from '../src/components/ui';
import { View } from 'react-native';

/**
 * Auth gate. Sends the user to the tabs when a session is restored, otherwise
 * to login. Shows a spinner while the persisted session is being checked.
 */
export default function Index() {
  const status = useAuth((s) => s.status);

  if (status === 'idle' || status === 'restoring') {
    return (
      <View className="flex-1 bg-background">
        <Loading label="Chargement…" />
      </View>
    );
  }

  if (status === 'authenticated') return <Redirect href="/(tabs)" />;
  return <Redirect href="/login" />;
}
