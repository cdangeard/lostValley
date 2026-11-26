import { describe, expect, it } from 'vitest';
import { canCraft, heartsText, nextStep, stageLabel } from './villagers';
import type { Content } from './content';

const content = {
  objets: { 'bouquet-de-lierre': 'Bouquet de lierre-cœur', 'pendentif-des-racines': 'Pendentif des racines', fibre: 'Fibre', feve: 'Fève' },
  romance: {
    coeursMax: 10,
    coeursRomance: 8,
    joursAvantMariage: 3,
    bouquet: 'bouquet-de-lierre',
    pendentif: 'pendentif-des-racines',
    recettes: { 'bouquet-de-lierre': { fibre: 10, feve: 5 } },
  },
} as unknown as Content;

describe('villagers', () => {
  it('draws hearts up to the cap', () => {
    expect(heartsText({ hearts: 3, maxHearts: 8 })).toBe('♥♥♥♡♡♡♡♡');
    expect(heartsText({ hearts: 12, maxHearts: 10 })).toBe('♥'.repeat(10));
  });

  it('names the stages', () => {
    expect(stageLabel('fiance', 12)).toContain('jour 12');
    expect(stageLabel(undefined)).toBe('');
  });

  it('says what comes next', () => {
    const b = { villager: 'bram', points: 800, hearts: 8, maxHearts: 8 };
    expect(nextStep(b, content, false)).toContain('bouquet');
    expect(nextStep(b, content, true)).toBe('');
    expect(nextStep({ ...b, stage: 'couple', hearts: 9, maxHearts: 10 }, content, false)).toContain('1 cœur');
  });

  it('checks recipes against the bag', () => {
    expect(canCraft('bouquet-de-lierre', { fibre: 10, feve: 5 }, content)).toBe(true);
    expect(canCraft('bouquet-de-lierre', { fibre: 9, feve: 5 }, content)).toBe(false);
    expect(canCraft('inconnu', {}, content)).toBe(false);
  });
});
