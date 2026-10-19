import { effectiveQualityId } from './quality';

describe('effectiveQualityId', () => {
  it('caps the user pick to the network when adaptive', () => {
    expect(effectiveQualityId('original', true, 'fast')).toBe('original');
    expect(effectiveQualityId('original', true, 'cellular')).toBe('medium');
    expect(effectiveQualityId('high', true, 'slow')).toBe('low');
  });

  it('never raises a lower pick, and is a no-op when disabled', () => {
    expect(effectiveQualityId('low', true, 'fast')).toBe('low');
    expect(effectiveQualityId('original', false, 'slow')).toBe('original');
  });
});
