import { Text, View } from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/**
 * Native fallback for the upload zone. Drag-and-drop file uploads are a web
 * affordance; on mobile we show an informational hint instead. The web variant
 * lives in `UploadDropzone.web.tsx`.
 */
// eslint-disable-next-line @typescript-eslint/no-unused-vars
export function UploadDropzone({ onFiles, busy }: { onFiles: (files: File[]) => void; busy?: boolean }) {
  const t = useT();
  const colors = useColors();
  return (
    <View className="flex-row items-center gap-2 rounded-2xl border border-border bg-surface-alt px-4 py-4">
      <Ionicon name="information-circle-outline" size={20} color={colors.muted} />
      <Text className="flex-1 text-xs text-muted">{t('media.local.webOnly')}</Text>
    </View>
  );
}
