import { Modal, Platform, Pressable, Text, View } from 'react-native';
import { useToast, ToastType } from '../stores/toast';
import { Ionicon } from './Ionicon';

/** Per-type colour + icon. green = success, red = error, blue = info, amber = warning. */
const STYLES: Record<ToastType, { bg: string; icon: string }> = {
  success: { bg: '#16a34a', icon: 'checkmark-circle' },
  error: { bg: '#dc2626', icon: 'alert-circle' },
  info: { bg: '#2563eb', icon: 'information-circle' },
  warning: { bg: '#d97706', icon: 'warning' },
};

/** Global toast overlay. Render once at the app root, above PlayerBar/TrackMenu/
 * SearchOverlay/MobileDrawer so it paints over them. Touches should fall
 * through everywhere except the toast itself (box-none) — a plain Modal does
 * that correctly on native, but react-native-web's Modal wrapper always has
 * `pointer-events: auto` and swallows taps regardless, so on web we skip
 * Modal and use a plain absolutely-positioned View instead. Tap to dismiss. */
export function ToastHost() {
  const toasts = useToast((s) => s.toasts);
  const dismiss = useToast((s) => s.dismiss);
  if (toasts.length === 0) return null;

  const list = toasts.map((t) => {
    const s = STYLES[t.type];
    return (
      <Pressable key={t.id} onPress={() => dismiss(t.id)} className="mb-2 w-full max-w-[460px]">
        <View
          className="flex-row items-center gap-2 rounded-xl px-4 py-3 shadow-lg"
          style={{ backgroundColor: s.bg }}
        >
          <Ionicon name={s.icon} size={18} color="#fff" />
          <Text className="flex-1 text-sm font-medium text-white">{t.message}</Text>
          {t.action ? (
            <Pressable
              onPress={() => {
                dismiss(t.id);
                t.action?.onPress();
              }}
              hitSlop={8}
            >
              <Text className="text-sm font-bold text-white underline">{t.action.label}</Text>
            </Pressable>
          ) : null}
        </View>
      </Pressable>
    );
  });

  if (Platform.OS === 'web') {
    return (
      <View
        pointerEvents="box-none"
        style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0 }}
        className="items-center px-6 pt-16"
      >
        {list}
      </View>
    );
  }

  return (
    <Modal transparent visible animationType="fade" onRequestClose={() => {}}>
      <View pointerEvents="box-none" className="flex-1 items-center px-6 pt-16">
        {list}
      </View>
    </Modal>
  );
}
