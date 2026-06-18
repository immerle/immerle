import { ImmerleApiError } from '../api/immerle/types';
import { tError } from './index';

describe('tError', () => {
  it('translates an API code and interpolates server params', () => {
    const err = new ImmerleApiError(404, 'user not found', 'user_not_found', { username: 'bob' });
    expect(tError(err)).toContain('bob');
  });

  it('falls back to the server message for an unknown code', () => {
    const err = new ImmerleApiError(500, 'boom', 'totally_unknown_code');
    expect(tError(err)).toBe('boom');
  });

  it('handles plain errors and non-errors', () => {
    expect(tError(new Error('not_found'))).not.toBe('not_found'); // resolved to a known key
    expect(tError(null)).toBeTruthy();
  });
});
