import type { Dir } from './net/protocol';

const STEP: Record<Dir, [number, number]> = { up: [0, -1], down: [0, 1], left: [-1, 0], right: [1, 0] };

/**
 * Where a w × h building goes when placed by a player at (x, y) facing dir:
 * its footprint covers the faced tile, extends away from the player and is
 * centred across. Returns the top-left tile. The server checks the same rule.
 */
export function footprint(x: number, y: number, facing: Dir, w: number, h: number): { x: number; y: number } {
  const [dx, dy] = STEP[facing];
  const fx = x + dx;
  const fy = y + dy;
  switch (facing) {
    case 'up':
      return { x: fx - Math.floor((w - 1) / 2), y: fy - (h - 1) };
    case 'down':
      return { x: fx - Math.floor((w - 1) / 2), y: fy };
    case 'left':
      return { x: fx - (w - 1), y: fy - Math.floor((h - 1) / 2) };
    case 'right':
      return { x: fx, y: fy - Math.floor((h - 1) / 2) };
  }
}

/** Tiles of a footprint, row by row. */
export function footprintTiles(x: number, y: number, w: number, h: number): Array<[number, number]> {
  const out: Array<[number, number]> = [];
  for (let j = 0; j < h; j++) for (let i = 0; i < w; i++) out.push([x + i, y + j]);
  return out;
}
