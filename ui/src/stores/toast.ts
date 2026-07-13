import { create } from 'zustand';

/** Toast severity — drives the colour/icon (see ToastHost). */
export type ToastType = 'success' | 'error' | 'info' | 'warning';

/** Optional call-to-action button rendered on the toast itself. */
export interface ToastAction {
  label: string;
  onPress: () => void;
}

export interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
  action?: ToastAction;
}

interface ToastState {
  toasts: ToastItem[];
  /** Show a toast (defaults to "info"); auto-dismisses after ~4s. */
  show: (message: string, type?: ToastType, action?: ToastAction) => void;
  success: (message: string, action?: ToastAction) => void;
  error: (message: string, action?: ToastAction) => void;
  info: (message: string, action?: ToastAction) => void;
  warning: (message: string, action?: ToastAction) => void;
  dismiss: (id: number) => void;
}

const DISMISS_MS = 4000;
let seq = 0;

/** Global toast queue. Call from anywhere: `useToast.getState().success('…')`
 * or `const toast = useToast()` then `toast.error('…')`. Render <ToastHost />
 * once at the app root. */
export const useToast = create<ToastState>((set) => {
  const show = (message: string, type: ToastType = 'info', action?: ToastAction) => {
    const id = ++seq;
    set((s) => ({ toasts: [...s.toasts, { id, message, type, action }] }));
    setTimeout(() => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })), DISMISS_MS);
  };
  return {
    toasts: [],
    show,
    success: (m, a) => show(m, 'success', a),
    error: (m, a) => show(m, 'error', a),
    info: (m, a) => show(m, 'info', a),
    warning: (m, a) => show(m, 'warning', a),
    dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  };
});
