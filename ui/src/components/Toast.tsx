import { Modal, Pressable, Text, View } from 'react-native';
import { useToast, ToastType } from '../stores/toast';
import { Ionicon } from './Ionicon';

/** Per-type colour + icon. green = success, red = error, blue = info, amber = warning. */
const STYLES: Record<ToastType, { bg: string; icon: string }> = {
  success: { bg: '#16a34a', icon: 'checkmark-circle' },
  error: { bg: '#dc2626', icon: 'alert-circle' },
  info: { bg: '#2563eb', icon: 'information-circle' },
  warning: { bg: '#d97706', icon: 'warning' },
};

/** Global toast overlay. Render once at the app root. It lives in its own
 * transparent Modal so it floats above everything — including the app's other
 * Modals (bottom sheets, track menu, …) — and lets touches fall through
 * elsewhere (box-none). Tap a toast to dismiss it. */
export function ToastHost() {
  const toasts = useToast((s) => s.toasts);
  const dismiss = useToast((s) => s.dismiss);
  return (
    <Modal transparent visible={toasts.length > 0} animationType="fade" onRequestClose={() => {}}>
      <View pointerEvents="box-none" className="flex-1 items-center px-6 pt-16">
        {toasts.map((t) => {
          const s = STYLES[t.type];
          return (
            <Pressable key={t.id} onPress={() => dismiss(t.id)} className="mb-2 w-full max-w-[460px]">
              <View
                className="flex-row items-center gap-2 rounded-xl px-4 py-3 shadow-lg"
                style={{ backgroundColor: s.bg }}
              >
                <Ionicon name={s.icon} size={18} color="#fff" />
                <Text className="flex-1 text-sm font-medium text-white">{t.message}</Text>
              </View>
            </Pressable>
          );
        })}
      </View>
    </Modal>
  );
}
