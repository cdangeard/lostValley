import { costLabel, itemLabel, type Content } from './content';
import type { BondView, Mood, Stage } from './net/protocol';

/** "♥♥♥♡♡♡♡♡" : full hearts, then empty ones up to the current cap. */
export function heartsText(b: Pick<BondView, 'hearts' | 'maxHearts'>): string {
  const full = Math.max(0, Math.min(b.hearts, b.maxHearts));
  return '♥'.repeat(full) + '♡'.repeat(Math.max(0, b.maxHearts - full));
}

export function stageLabel(stage: Stage | undefined, wedding?: number): string {
  switch (stage) {
    case 'couple':
      return 'En couple';
    case 'fiance':
      return wedding ? `Fiançailles · mariage le jour ${wedding}` : 'Fiançailles';
    case 'marie':
      return 'Marié·e';
    default:
      return '';
  }
}

const HELP: Record<string, string> = {
  arroser: 'arrose tes cultures chaque matin',
  reparer: 'répare tes clôtures et bâtiments chaque matin',
  nourrir: 'nourrit tes créatures chaque matin',
};

export function helpLabel(help: string): string {
  return HELP[help] ?? '';
}

/** What the player should do next with this villager. */
export function nextStep(b: BondView, content: Content, partnerElsewhere: boolean): string {
  const r = content.romance;
  if (!r) return '';
  if (b.stage === 'marie') return '';
  if (b.stage === 'fiance') return 'Le mariage approche.';
  if (b.stage === 'couple') {
    return b.hearts >= r.coeursMax
      ? `Offre-lui un ${itemLabel(r.pendentif, content).toLowerCase()} pour te fiancer.`
      : `Encore ${r.coeursMax - b.hearts} cœur(s) avant le ${itemLabel(r.pendentif, content).toLowerCase()}.`;
  }
  if (partnerElsewhere) return '';
  if (b.hearts >= r.coeursRomance) return `Offre-lui un ${itemLabel(r.bouquet, content).toLowerCase()} pour déclarer ta flamme.`;
  return '';
}

export function recipeLabel(item: string, content: Content): string {
  const recipe = content.romance?.recettes[item];
  return recipe ? costLabel(recipe, content) : '';
}

export function canCraft(item: string, bag: Record<string, number>, content: Content): boolean {
  const recipe = content.romance?.recettes[item];
  return !!recipe && Object.entries(recipe).every(([it, n]) => (bag[it] ?? 0) >= n);
}

/** Tastes the player found out by giving gifts, remembered in this browser. */
export class Tastes {
  private known: Record<string, Record<string, Mood>> = {};

  constructor(private readonly key = 'lostvalley.gouts') {
    try {
      this.known = JSON.parse(localStorage.getItem(key) ?? '{}') ?? {};
    } catch {
      this.known = {};
    }
  }

  learn(villager: string, item: string, mood: Mood): void {
    if (!['adore', 'aime', 'neutre', 'deteste'].includes(mood)) return;
    (this.known[villager] ??= {})[item] = mood;
    try {
      localStorage.setItem(this.key, JSON.stringify(this.known));
    } catch {
      /* private mode: forget on reload */
    }
  }

  of(villager: string, item: string): Mood | undefined {
    return this.known[villager]?.[item];
  }
}

export const TASTE_MARK: Partial<Record<Mood, string>> = { adore: '♥♥', aime: '♥', neutre: '·', deteste: '✗' };
