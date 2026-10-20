import { planMoves } from './autodj';

const apply = (ids: string[], moves: [number, number][]) => {
  const out = [...ids];
  for (const [from, to] of moves) out.splice(to, 0, ...out.splice(from, 1));
  return out;
};

describe('planMoves', () => {
  it('reorders the queue after the current track', () => {
    const ids = ['done', 'now', 'c', 'a', 'b'];
    const moves = planMoves(ids, 1, ['now', 'a', 'b', 'c']);
    expect(apply(ids, moves)).toEqual(['done', 'now', 'a', 'b', 'c']);
    expect(moves.every(([from, to]) => from > 1 && to > 1)).toBe(true); // played and current tracks never move
  });

  it('keeps duplicates', () => {
    const ids = ['now', 'x', 'y', 'x'];
    expect(apply(ids, planMoves(ids, 0, ['now', 'x', 'x', 'y']))).toEqual(['now', 'x', 'x', 'y']);
  });

  it('does nothing for an order already in place', () => {
    expect(planMoves(['a', 'b'], 0, ['a', 'b'])).toEqual([]);
  });
});
