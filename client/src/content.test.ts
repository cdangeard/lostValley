import { describe, expect, it } from 'vitest';
import raw from '../../assets/data/contenu.json';
import { buildHotbar, type Content } from './content';

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
});
