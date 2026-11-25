import { describe, expect, it } from 'vitest';
import { footprint, footprintTiles } from './placement';

describe('footprint', () => {
  it('covers the faced tile and never the player', () => {
    for (const facing of ['up', 'down', 'left', 'right'] as const) {
      for (const [w, h] of [[1, 1], [2, 1], [2, 2], [3, 2]]) {
        const tl = footprint(10, 10, facing, w, h);
        const tiles = footprintTiles(tl.x, tl.y, w, h).map(([x, y]) => `${x},${y}`);
        const front = { up: '10,9', down: '10,11', left: '9,10', right: '11,10' }[facing];
        expect(tiles).toContain(front);
        expect(tiles).not.toContain('10,10');
      }
    }
  });

  it('grows away from the player', () => {
    expect(footprint(5, 5, 'up', 2, 2)).toEqual({ x: 5, y: 3 });
    expect(footprint(5, 5, 'right', 2, 1)).toEqual({ x: 6, y: 5 });
  });
});
