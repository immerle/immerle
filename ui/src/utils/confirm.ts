import { Alert, Platform } from 'react-native';

/** Cross-platform yes/no confirm (window.confirm on web, Alert on native). */
export function confirm(message: string, onYes: () => void, labels?: { cancel: string; ok: string }) {
  if (Platform.OS === 'web') {
    if (window.confirm(message)) onYes();
    return;
  }
  Alert.alert(message, undefined, [
    { text: labels?.cancel ?? 'Cancel', style: 'cancel' },
    { text: labels?.ok ?? 'OK', style: 'destructive', onPress: onYes },
  ]);
}
