import { Pressable, Text, View } from 'react-native';
import { router } from 'expo-router';
import { usePlayer } from '../src/audio/store';
import { useAuth } from '../src/auth/store';
import { usePlaybackTargets } from '../src/query/account';
import { Ionicon } from '../src/components/Ionicon';
import { Loading } from '../src/components/ui';
import { useColors } from '../src/theme/colors';
import { useT } from '../src/i18n/store';

/**
 * Cast-target picker as its own route (pushed by CastButton, modal like /queue) —
 * a custom anchored <Modal> popover proved unreliable on native (see CastButton
 * in src/components/PlayerBar.tsx). "Everywhere" means independent playback per device.
 */
export default function CastTarget() {
  const t = useT();
  const myId = useAuth((s) => s.client?.getSession()?.deviceId);
  const castTargetId = usePlayer((s) => s.castTargetId);
  const setCastTarget = usePlayer((s) => s.setCastTarget);
  const { data: targets, isLoading } = usePlaybackTargets(true);
  const others = (targets ?? []).filter((d) => d.id !== myId);

  const pick = (deviceId: string) => {
    void setCastTarget(deviceId);
    router.back();
  };

  return (
    <View className="flex-1 bg-background pt-2">
      {myId ? (
        <CastRow label={t('components.player.castThisDevice')} selected={castTargetId === myId} onPress={() => pick(myId)} />
      ) : null}
      <CastRow label={t('components.player.castEverywhere')} selected={!castTargetId} onPress={() => pick('')} />
      {isLoading ? (
        <Loading />
      ) : others.length === 0 ? (
        <Text className="px-4 py-3 text-sm text-muted">{t('components.player.castNoOtherDevices')}</Text>
      ) : (
        others.map((d) => <CastRow key={d.id} label={d.name} selected={castTargetId === d.id} onPress={() => pick(d.id)} />)
      )}
    </View>
  );
}

function CastRow({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) {
  const colors = useColors();
  return (
    <Pressable onPress={onPress} className="flex-row items-center gap-3 px-4 py-3.5 active:bg-surface-alt">
      <Text className="flex-1 text-base text-foreground" numberOfLines={1}>
        {label}
      </Text>
      {selected ? <Ionicon name="checkmark" size={18} color={colors.primary} /> : null}
    </Pressable>
  );
}
