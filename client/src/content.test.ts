import { describe, expect, it } from 'vitest';
import raw from '../../assets/data/contenu.json';
import { buildHotbar, buildState, costLabel, type Content } from './content';

const content = raw as Content;

describe('buildHotbar', () => {
  it('lists tools, then seeds the bag holds', () => {
    const bar = buildHotbar(['houe', 'arrosoir'], { 'graine-ble': 2, 'graine-navet': 0, bois: 4 }, content);
    expect(bar.map((s) => s.item)).toEqual(['houe', 'arrosoir', 'graine-ble']);
    expect(bar[0].label).toBe('Houe');
    expect(bar[2]).toMatchObject({ kind: 'seed', count: 2, label: 'Graines de blé' });
  });

  it('knows every seed and harvest by name', () => {
    for (const crop of Object.values(content.cultures)) {
      expect(content.objets[crop.graine]).toBeTruthy();
    }
  });

  it('adds food after seeds', () => {
    const bar = buildHotbar(['houe'], { navet: 2, 'graine-ble': 1, bois: 3 }, content);
    expect(bar.map((s) => s.kind)).toEqual(['tool', 'seed', 'food']);
  });
});

describe('buildings', () => {
  it('needs the workshop first, then materials', () => {
    const well = content.batiments.puits;
    expect(buildState(well, { pierre: 99, bois: 99 }, new Set())).toBe('locked');
    expect(buildState(well, {}, new Set(['atelier']))).toBe('poor');
    expect(buildState(well, { pierre: 12, bois: 4 }, new Set(['atelier']))).toBe('ok');
    expect(costLabel(well.cout, content)).toBe('12 Pierre, 4 Bois');
  });

  it('names every creature drop', () => {
    for (const kind of Object.keys(content.creatures)) expect(content.creatures[kind].nom).toBeTruthy();
    for (const item of ['ration', 'plume-noire', 'epine']) expect(content.objets[item]).toBeTruthy();
  });
});
