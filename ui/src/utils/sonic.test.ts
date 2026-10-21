import { sonicSummary } from './sonic';

const t = (key: string, params?: Record<string, unknown>) => `${key}:${params?.percent}`;

describe('sonicSummary', () => {
  it('rounds the tempo, names the key and gives the energy as a percentage', () => {
    expect(sonicSummary(t, { bpm: 127.6, key: 'A', mode: 'minor', camelot: '8A', energy: 0.823, danceability: 0.5 })).toBe(
      '128 BPM · Am (8A) · components.trackMenu.energy:82',
    );
  });

  it('leaves out what the analysis could not find', () => {
    expect(sonicSummary(t, { bpm: 0, key: '', mode: '', camelot: '', energy: 0.4, danceability: 0 })).toBe('components.trackMenu.energy:40');
  });
});
