import { fetchWithTimeout } from './immerleApi';

describe('fetchWithTimeout', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('resolves through the underlying fetch on success', async () => {
    const response = new Response('ok');
    jest.spyOn(globalThis, 'fetch').mockResolvedValue(response);
    await expect(fetchWithTimeout(new Request('http://example.invalid'))).resolves.toBe(response);
  });

  it('forwards an already-aborted caller signal to the underlying request', async () => {
    // Regression: an unreachable server must resolve to an error within a
    // bounded time instead of hanging a component's query forever -- the
    // combined signal (caller's + the internal timeout) is what makes that
    // possible, so it must actually reach the real fetch call.
    const controller = new AbortController();
    controller.abort();
    let seenAborted = false;
    jest.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      seenAborted = (input as Request).signal.aborted;
      return new Response('ok');
    });
    await fetchWithTimeout(new Request('http://example.invalid', { signal: controller.signal }));
    expect(seenAborted).toBe(true);
  });
});
