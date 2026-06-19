import { useEffect, useRef } from 'react';
import { Pressable, Text, View } from 'react-native';
import { Ionicon } from './Ionicon';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

const AUDIO_EXT = /\.(mp3|flac|m4a|aac|ogg|opus|wav|wma|aiff?|alac)$/i;

function audioFiles(list: FileList | File[] | null | undefined): File[] {
  return Array.from(list ?? []).filter((f) => f.type.startsWith('audio/') || AUDIO_EXT.test(f.name));
}

/**
 * Web drag-and-drop (and click-to-pick) zone for uploading audio files. DOM drag
 * events are wired on the underlying div via a ref; click opens a transient file
 * input. Audio files are filtered client-side (the server validates again).
 */
export function UploadDropzone({ onFiles, busy }: { onFiles: (files: File[]) => void; busy?: boolean }) {
  const t = useT();
  const colors = useColors();
  const ref = useRef<View>(null);

  useEffect(() => {
    // On react-native-web a View's ref is the underlying DOM node.
    const node = ref.current as unknown as HTMLElement | null;
    if (!node) return;
    const prevent = (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
    };
    const onDrop = (e: DragEvent) => {
      prevent(e);
      const files = audioFiles(e.dataTransfer?.files);
      if (files.length) onFiles(files);
    };
    node.addEventListener('dragover', prevent);
    node.addEventListener('drop', onDrop);
    return () => {
      node.removeEventListener('dragover', prevent);
      node.removeEventListener('drop', onDrop);
    };
  }, [onFiles]);

  const pick = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'audio/*';
    input.multiple = true;
    input.onchange = () => {
      const files = audioFiles(input.files);
      if (files.length) onFiles(files);
    };
    input.click();
  };

  return (
    <Pressable ref={ref} onPress={pick} disabled={busy} accessibilityRole="button">
      <View className="items-center gap-2 rounded-2xl border-2 border-dashed border-border bg-surface-alt px-4 py-8">
        <Ionicon name={busy ? 'hourglass-outline' : 'cloud-upload-outline'} size={32} color={colors.muted} />
        <Text className="text-sm font-medium text-foreground">
          {busy ? t('media.local.uploading') : t('media.local.dropHere')}
        </Text>
        <Text className="text-xs text-muted">{t('media.local.dropHint')}</Text>
      </View>
    </Pressable>
  );
}
