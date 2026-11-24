/** The part of assets/data/contenu.json the client uses: names and labels. */
export interface Content {
  outils: Record<string, { nom: string; energie: number }>;
  cultures: Record<string, { nom: string; graine: string; jours: number; sauvage?: boolean }>;
  objets: Record<string, string>;
}

export interface HotbarSlot {
  item: string; // what Use sends
  label: string;
  count?: number; // seeds only
  kind: 'tool' | 'seed';
}

/** Tools first (in the order the player carries them), then the seeds in the bag. */
export function buildHotbar(tools: string[], bag: Record<string, number>, content: Content): HotbarSlot[] {
  const slots: HotbarSlot[] = tools.map((t) => ({ item: t, label: content.outils[t]?.nom ?? t, kind: 'tool' }));
  const seeds = Object.values(content.cultures).map((c) => c.graine);
  for (const seed of seeds) {
    const n = bag[seed] ?? 0;
    if (n > 0) slots.push({ item: seed, label: itemLabel(seed, content), count: n, kind: 'seed' });
  }
  return slots;
}

export function itemLabel(item: string, content: Content): string {
  return content.objets[item] ?? item;
}
