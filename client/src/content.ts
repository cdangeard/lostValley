/** The part of assets/data/contenu.json the client uses: names, labels and costs. */
export interface Content {
  outils: Record<string, { nom: string; energie: number; combat?: { motif: string; degats: number; ticks: number } }>;
  cultures: Record<string, { nom: string; graine: string; jours: number; sauvage?: boolean }>;
  objets: Record<string, string>;
  nourriture: Record<string, number>;
  creatures: Record<string, CreatureDef>;
  batiments: Record<string, BuildingDef>;
}

export interface CreatureDef {
  nom: string;
  pv: number;
  faiblesse?: string;
  comportement?: string;
  nocturne?: boolean;
  apprivoisement?: { role: string; habitat: string; regime: Record<string, number>; produit?: string; soumission?: number };
}

export interface BuildingDef {
  nom: string;
  largeur: number;
  hauteur: number;
  cout: Record<string, number>;
  requiert?: string;
  description?: string;
}

export interface HotbarSlot {
  item: string; // what Use sends
  label: string;
  count?: number; // seeds and food
  kind: 'tool' | 'seed' | 'food' | 'treat';
}

/** Tools first (in the order the player carries them), then seeds, then food, then what only creatures eat. */
export function buildHotbar(tools: string[], bag: Record<string, number>, content: Content): HotbarSlot[] {
  const slots: HotbarSlot[] = tools.map((t) => ({ item: t, label: content.outils[t]?.nom ?? t, kind: 'tool' }));
  const seeds = Object.values(content.cultures).map((c) => c.graine);
  for (const seed of seeds) {
    const n = bag[seed] ?? 0;
    if (n > 0) slots.push({ item: seed, label: itemLabel(seed, content), count: n, kind: 'seed' });
  }
  for (const food of Object.keys(content.nourriture ?? {})) {
    const n = bag[food] ?? 0;
    if (n > 0) slots.push({ item: food, label: itemLabel(food, content), count: n, kind: 'food' });
  }
  const listed = new Set(slots.map((s) => s.item));
  for (const c of Object.values(content.creatures ?? {})) {
    for (const item of Object.keys(c.apprivoisement?.regime ?? {})) {
      const n = bag[item] ?? 0;
      if (n > 0 && !listed.has(item)) {
        listed.add(item);
        slots.push({ item, label: itemLabel(item, content), count: n, kind: 'treat' });
      }
    }
  }
  return slots;
}

export function itemLabel(item: string, content: Content): string {
  return content.objets[item] ?? item;
}

/** "20 Bois, 10 Pierre" */
export function costLabel(cost: Record<string, number>, content: Content): string {
  return Object.entries(cost)
    .map(([item, n]) => `${n} ${itemLabel(item, content)}`)
    .join(', ');
}

export type BuildState = 'ok' | 'locked' | 'poor';

/** Whether a building can be afforded and is unlocked, given what stands in the domain. */
export function buildState(def: BuildingDef, bag: Record<string, number>, built: Set<string>): BuildState {
  if (def.requiert && !built.has(def.requiert)) return 'locked';
  return Object.entries(def.cout).every(([item, n]) => (bag[item] ?? 0) >= n) ? 'ok' : 'poor';
}
