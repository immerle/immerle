import { shouldRetry } from './queryClient';
import { ImmerleApiError } from '../api/immerle/types';

describe('shouldRetry', () => {
  it('retries once when the server answered with an error', () => {
    expect(shouldRetry(0, new ImmerleApiError(500, 'boom'))).toBe(true);
    expect(shouldRetry(1, new ImmerleApiError(500, 'boom'))).toBe(false);
  });

  it('never retries a network failure or a timeout', () => {
    expect(shouldRetry(0, new TypeError('Network request failed'))).toBe(false);
    expect(shouldRetry(0, new DOMException('aborted', 'AbortError'))).toBe(false);
  });
});
