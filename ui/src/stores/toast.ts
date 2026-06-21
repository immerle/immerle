import { create } from 'zustand';

/** Toast severity — drives the colour/icon (see ToastHost). */
export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface ToastItem {
  id: number;
  message: string;
  type: ToastType;
}

interface ToastState {
  toasts: ToastItem[];
  /** Show a toast (defaults to "info"); auto-dismisses after ~4s. */
  show: (message: string, type?: ToastType) => void;
  success: (message: string) => void;
  error: (message: string) => void;
  info: (message: string) => void;
  warning: (message: string) => void;
  dismiss: (id: number) => void;
}

const DISMISS_MS = 4000;
let seq = 0;

/** Global toast queue. Call from anywhere: `useToast.getState().success('…')`
 * or `const toast = useToast()` then `toast.error('…')`. Render <ToastHost />
 * once at the app root. */
export const useToast = create<ToastState>((set) => {
  const show = (message: string, type: ToastType = 'info') => {
    const id = ++seq;
    set((s) => ({ toasts: [...s.toasts, { id, message, type }] }));
    setTimeout(() => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })), DISMISS_MS);
  };
  return {
    toasts: [],
    show,
    success: (m) => show(m, 'success'),
    error: (m) => show(m, 'error'),
    info: (m) => show(m, 'info'),
    warning: (m) => show(m, 'warning'),
    dismiss: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
  };
});
