import { EngineEvents } from './types';

/** Minimal typed event emitter shared by both engine implementations. */
export class EngineEmitter {
  private handlers: { [K in keyof EngineEvents]: Set<EngineEvents[K]> } = {
    state: new Set(),
    progress: new Set(),
    trackChange: new Set(),
  };

  on<E extends keyof EngineEvents>(event: E, handler: EngineEvents[E]): () => void {
    this.handlers[event].add(handler);
    return () => {
      this.handlers[event].delete(handler);
    };
  }

  emit<E extends keyof EngineEvents>(event: E, ...args: Parameters<EngineEvents[E]>): void {
    for (const handler of this.handlers[event]) {
      (handler as (...a: unknown[]) => void)(...args);
    }
  }
}
