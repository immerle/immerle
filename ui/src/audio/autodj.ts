/**
 * The moves (from, to) that turn `ids` into `ordered` from position `start`
 * on, applied one after the other. `ordered` holds the same ids as
 * ids.slice(start), duplicates included; ids missing from it stay after.
 */
export function planMoves(ids: string[], start: number, ordered: string[]): [number, number][] {
  const cur = [...ids];
  const moves: [number, number][] = [];
  ordered.forEach((id, k) => {
    const to = start + k;
    const from = cur.findIndex((x, i) => i >= to && x === id);
    if (from === -1 || from === to) return;
    cur.splice(to, 0, ...cur.splice(from, 1));
    moves.push([from, to]);
  });
  return moves;
}
