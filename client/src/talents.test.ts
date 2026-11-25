import { describe, expect, it } from 'vitest';
import raw from '../../assets/data/constellation.json';
import { canRefund, canTake, graph, levelProgress, modLabel, sumMods, type Constellation } from './talents';

const g = graph(raw as unknown as Constellation);

describe('constellation', () => {
  it('starts from the heart', () => {
    expect(g.heart).toBe('coeur');
    expect(canTake(g, new Set(), 'terre-t1', 1)).toBe(true);
    expect(canTake(g, new Set(), 'terre-t2', 1)).toBe(false);
    expect(canTake(g, new Set(), 'terre-t1', 0)).toBe(false);
    expect(canTake(g, new Set(['terre-t1']), 'terre-t2', 1)).toBe(true);
  });

  it('gives back only what keeps the rest linked', () => {
    const taken = new Set(['terre-t1', 'terre-t2']);
    expect(canRefund(g, taken, 'terre-t1')).toBe(false);
    expect(canRefund(g, taken, 'terre-t2')).toBe(true);
    expect(canRefund(g, taken, 'sylve-t1')).toBe(false);
  });

  it('labels every modifier it uses', () => {
    for (const n of g.nodes.values()) {
      for (const [k, v] of Object.entries(n.mods)) expect(modLabel(k, v)).not.toBe(`${k} ${v}`);
    }
    expect(sumMods(g, ['lame-t1', 'lame-t3'])).toEqual({ vieMax: 4 });
  });

  it('fills the experience bar', () => {
    expect(levelProgress(25, 20, 50)).toBeCloseTo(1 / 6);
    expect(levelProgress(10, 10, 0)).toBe(1);
  });
});
