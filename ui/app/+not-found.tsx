import { Text, View } from 'react-native';
import { Link, Stack } from 'expo-router';

export default function NotFound() {
  return (
    <>
      <Stack.Screen options={{ title: 'Introuvable' }} />
      <View className="flex-1 items-center justify-center gap-3 bg-background p-6">
        <Text className="text-lg font-semibold text-foreground">Cette page n'existe pas.</Text>
        <Link href="/(tabs)" className="text-primary">
          Retour à l'accueil
        </Link>
      </View>
    </>
  );
}
