import { useEffect, useState } from 'react';
import { Modal, Platform, Pressable, Text, View } from 'react-native';
import { Song } from '../api/subsonic/types';
import { CoverArt } from './CoverArt';
import { Ionicon } from './Ionicon';
import { Field } from './ui';
import { useRenameTrack, useSetTrackCover } from '../query/local';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/** Pick a single image File via a transient input (web only). */
function pickImage(): Promise<File | null> {
  return new Promise((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.onchange = () => resolve(input.files?.[0] ?? null);
    input.click();
  });
}

/**
 * Edit modal for a user-uploaded ("local") track: rename and replace the cover.
 * Owner-only edits are enforced server-side. Cover replacement is a web
 * affordance (uses a file input); rename works everywhere.
 */
export function LocalTrackEditModal({ song, onClose }: { song: Song | null; onClose: () => void }) {
  const t = useT();
  const colors = useColors();
  const rename = useRenameTrack();
  const setCover = useSetTrackCover();
  const [title, setTitle] = useState('');
  const [coverArt, setCoverArt] = useState<string | undefined>(undefined);

  useEffect(() => {
    setTitle(song?.title ?? '');
    setCoverArt(song?.coverArt);
  }, [song]);

  if (!song) return null;

  const save = () => {
    const trimmed = title.trim();
    if (!trimmed) return;
    rename.mutate({ id: song.id, title: trimmed }, { onSuccess: onClose });
  };

  const changeCover = async () => {
    const image = await pickImage();
    if (!image) return;
    setCover.mutate(
      { id: song.id, image },
      { onSuccess: (updated) => setCoverArt(updated.coverArt) },
    );
  };

  return (
    <Modal transparent animationType="slide" visible onRequestClose={onClose}>
      <Pressable className="flex-1 justify-end bg-black/50" onPress={onClose}>
        <Pressable className="rounded-t-3xl bg-surface px-5 pb-8 pt-2" onPress={(e) => e.stopPropagation()}>
          <View className="mb-3 items-center pt-1">
            <View className="h-1 w-10 rounded-full bg-border" />
          </View>

          <Text className="pb-4 text-lg font-bold text-foreground">{t('media.local.editTitle')}</Text>

          <View className="flex-row items-center gap-4 pb-4">
            <CoverArt coverArt={coverArt} size={72} rounded="rounded-lg" />
            {Platform.OS === 'web' ? (
              <Pressable
                onPress={changeCover}
                disabled={setCover.isPending}
                className="flex-row items-center gap-2 rounded-xl border border-border px-4 py-3 active:opacity-70"
              >
                <Ionicon name="image-outline" size={18} color={colors.foreground} />
                <Text className="text-sm font-medium text-foreground">
                  {setCover.isPending ? t('media.local.uploading') : t('media.local.changeCover')}
                </Text>
              </Pressable>
            ) : null}
          </View>

          <Field
            label={t('media.local.name')}
            value={title}
            onChangeText={setTitle}
            onSubmitEditing={save}
            placeholder={song.title}
          />

          <View className="mt-4 flex-row justify-end gap-2">
            <Pressable onPress={onClose} className="rounded-xl px-4 py-3 active:opacity-70">
              <Text className="font-medium text-muted">{t('media.local.cancel')}</Text>
            </Pressable>
            <Pressable
              onPress={save}
              disabled={rename.isPending || !title.trim()}
              className="rounded-xl bg-primary px-4 py-3 active:opacity-80"
            >
              <Text className="font-semibold text-primary-foreground">{t('media.local.save')}</Text>
            </Pressable>
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}
