import { Text, View } from 'react-native';
import { Link, Stack } from 'expo-router';
import { useT } from '../src/i18n/store';

export default function NotFound() {
  const t = useT();
  return (
    <>
      <Stack.Screen options={{ title: t('auth.notFound.title') }} />
      <View className="flex-1 items-center justify-center gap-3 bg-background p-6">
        <Text className="text-lg font-semibold text-foreground">{t('auth.notFound.message')}</Text>
        <Link href="/(tabs)" className="text-primary">
          {t('auth.notFound.backHome')}
        </Link>
      </View>
    </>
  );
}
