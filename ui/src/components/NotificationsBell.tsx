import { useState } from 'react';
import { Modal, Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { Ionicon } from './Ionicon';
import { useAuth } from '../auth/store';
import { useJam } from '../jam/store';
import { useJamMutations, useJamInvites, useJamInviteMutations } from '../query/social';
import { useColors } from '../theme/colors';
import { useT } from '../i18n/store';

/**
 * Generic notification bell: badge count + a list to act on. Jam invites are
 * the only notification kind today, but the bell itself isn't jam-specific —
 * other kinds can land here later without a redesign.
 */
export function NotificationsBell() {
  const t = useT();
  const colors = useColors();
  const client = useAuth((s) => s.client);
  const hasJam = client?.has('jam') ?? false;
  const invites = useJamInvites();
  const jam = useJamMutations();
  const { dismiss } = useJamInviteMutations();
  const [open, setOpen] = useState(false);

  if (!hasJam) return null;

  const count = invites.data?.length ?? 0;

  const acceptInvite = (sessionId: string) => {
    setOpen(false);
    jam.join.mutate(sessionId, {
      onSuccess: () => {
        useJam.getState().start(sessionId, false);
        router.push(`/jam/${sessionId}` as never);
      },
    });
  };

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        accessibilityLabel={t('notifications.title')}
        className="h-9 w-9 items-center justify-center rounded-full bg-surface-alt active:opacity-70"
      >
        <View>
          <Ionicon name="notifications-outline" size={18} color={colors.foreground} />
          {count > 0 ? (
            <View className="absolute -right-1.5 -top-1.5 h-4 w-4 items-center justify-center rounded-full bg-danger">
              <Text className="text-[10px] font-bold text-white">{count}</Text>
            </View>
          ) : null}
        </View>
      </Pressable>

      <Modal transparent visible={open} animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable className="flex-1 items-center justify-center bg-black/50 px-6" onPress={() => setOpen(false)}>
          <Pressable onPress={(e) => e.stopPropagation()} className="w-full max-w-sm gap-2 rounded-2xl bg-surface p-4">
            <Text className="pb-1 text-base font-semibold text-foreground">{t('notifications.title')}</Text>
            {!count ? (
              <Text className="px-1 py-3 text-sm text-muted">{t('notifications.empty')}</Text>
            ) : (
              (invites.data ?? []).map((inv) => (
                <View key={inv.id} className="gap-2 rounded-lg bg-surface-alt p-3">
                  <Text className="text-sm text-foreground">
                    {t('social.jam.invitedBy', { name: inv.inviterDisplayName || inv.inviterUsername, session: inv.sessionName })}
                  </Text>
                  <View className="flex-row gap-2">
                    <Pressable
                      onPress={() => acceptInvite(inv.sessionId)}
                      className="flex-1 items-center rounded-full bg-primary py-2 active:opacity-80"
                    >
                      <Text className="text-sm font-semibold text-primary-foreground">{t('social.jam.join')}</Text>
                    </Pressable>
                    <Pressable
                      onPress={() => dismiss.mutate(inv.id)}
                      className="flex-1 items-center rounded-full border border-border py-2 active:bg-surface"
                    >
                      <Text className="text-sm text-foreground">{t('social.jam.dismiss')}</Text>
                    </Pressable>
                  </View>
                </View>
              ))
            )}
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}
