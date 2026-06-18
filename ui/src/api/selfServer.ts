import { create } from 'zustand';
import { createImmerleApi } from './immerleApi';

type SelfServer = {
  /** Origin serving this web app when it's an Immerle binary; null on native or a non-Immerle host. */
  url: string | null;
  /** True when that server still needs first-run setup (capabilities.setup.needed). */
  needsSetup: boolean;
  /** Whether the one-shot probe has run (gate/screens wait on this). */
  checked: boolean;
  detect: () => Promise<void>;
};

/**
 * When the Immerle binary serves this web app, the server lives at the page's
 * own origin. We probe it once at startup so login/setup can pre-fill the
 * server address and the gate can jump straight to first-run setup. No-op on
 * native (no window) and on a non-Immerle host (the probe fails → checked, no url).
 */
export const useSelfServer = create<SelfServer>((set, get) => ({
  url: null,
  needsSetup: false,
  checked: false,
  detect: async () => {
    if (get().checked) return;
    const origin = typeof window !== 'undefined' ? window.location?.origin : undefined;
    if (!origin) {
      set({ checked: true });
      return;
    }
    try {
      const { data } = await createImmerleApi(origin).GET('/capabilities');
      // Only trust it as our server when it actually returned a capabilities
      // object (a dev server / static host answering HTML 200 won't).
      const caps = (data as { capabilities?: Record<string, { needed?: boolean }> } | undefined)
        ?.capabilities;
      if (caps) {
        set({ url: origin, needsSetup: !!caps.setup?.needed, checked: true });
        return;
      }
    } catch {
      // Non-Immerle host or network error: fall through to checked-with-no-url.
    }
    set({ checked: true });
  },
}));
