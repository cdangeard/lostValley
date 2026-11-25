/** The Constellation des Racines as assets/data/constellation.json describes it. The server decides; these mirror its rules for the screen. */

export type NodeType = 'coeur' | 'petit' | 'notable' | 'angulaire';

export interface TalentNode {
  id: string;
  nom: string;
  type: NodeType;
  region: string;
  x: number;
  y: number;
  mods: Record<string, number>;
  description?: string;
}

export interface Constellation {
  noeuds: TalentNode[];
  liens: [string, string][];
}

export interface Graph {
  nodes: Map<string, TalentNode>;
  adj: Map<string, string[]>;
  heart: string;
}

export function graph(c: Constellation): Graph {
  const nodes = new Map(c.noeuds.map((n) => [n.id, n]));
  const adj = new Map<string, string[]>();
  for (const [a, b] of c.liens) {
    adj.set(a, [...(adj.get(a) ?? []), b]);
    adj.set(b, [...(adj.get(b) ?? []), a]);
  }
  const heart = c.noeuds.find((n) => n.type === 'coeur')?.id ?? '';
  return { nodes, adj, heart };
}

/** A node can be taken when it is linked to the heart or to a node already taken, and a point is left. */
export function canTake(g: Graph, taken: Set<string>, id: string, points: number): boolean {
  if (points <= 0 || taken.has(id) || id === g.heart || !g.nodes.has(id)) return false;
  return (g.adj.get(id) ?? []).some((n) => n === g.heart || taken.has(n));
}

/** Whether a node is next to what was taken: drawn as reachable even with no point left. */
export function reachable(g: Graph, taken: Set<string>, id: string): boolean {
  return canTake(g, taken, id, 1);
}

/** A taken node can be given back when every other taken node stays linked to the heart. */
export function canRefund(g: Graph, taken: Set<string>, id: string): boolean {
  if (!taken.has(id)) return false;
  const seen = new Set([g.heart]);
  const queue = [g.heart];
  while (queue.length) {
    const n = queue.shift()!;
    for (const m of g.adj.get(n) ?? []) {
      if (m === id || seen.has(m) || !taken.has(m)) continue;
      seen.add(m);
      queue.push(m);
    }
  }
  return seen.size === taken.size; // the heart, plus every taken node but this one
}

/** Sums the modifiers of the taken nodes. */
export function sumMods(g: Graph, taken: Iterable<string>): Record<string, number> {
  const out: Record<string, number> = {};
  for (const id of taken) {
    for (const [k, v] of Object.entries(g.nodes.get(id)?.mods ?? {})) out[k] = (out[k] ?? 0) + v;
  }
  return out;
}

const LABELS: Record<string, (v: number) => string> = {
  rendement: (v) => `+${v} % de chances de récolter une part en plus`,
  graineRetour: (v) => `+${v} % de chances de récupérer une graine`,
  mutation: (v) => `+${v} % de chances de mutation de tes semis`,
  arrosoir: (v) => `+${v} charges d'arrosoir`,
  rosee: (v) => `${v} % de chances que la rosée arrose tes semis secs`,
  economieEnergie: (v) => `${v} % de chances qu'un geste ne coûte pas d'énergie`,
  confiance: (v) => `+${v} % de confiance par repas`,
  production: (v) => `+${v} % de chances que tes bêtes produisent le double`,
  vieCompagnons: (v) => `+${v} PV pour tes créatures`,
  degatsCompagnons: (v) => `+${v} dégâts pour tes créatures`,
  compagnons: (v) => `+${v} créature${v > 1 ? 's' : ''} peut te suivre`,
  materiaux: (v) => `+${v} % de chances de bois ou pierre en plus`,
  energieMax: (v) => `+${v} énergie`,
  economieConstruction: (v) => `-${v} % de matériaux pour construire`,
  cloture: (v) => `+${v} PV pour tes clôtures`,
  vieMax: (v) => `+${v} PV`,
  esquive: (v) => `${v} % de chances d'esquiver un coup`,
  nuit: (v) => `+${v} dégâts la nuit`,
  degatsHoue: (v) => `+${v} dégâts à la houe`,
  degatsFaux: (v) => `+${v} dégâts à la faux`,
  degatsHache: (v) => `+${v} dégâts à la hache`,
  degatsPioche: (v) => `+${v} dégâts à la pioche`,
  mainVerte: () => 'Main verte',
  meneurDeMeute: () => 'Meneur de meute',
};

/** "+5 % de chances de récolter une part en plus" */
export function modLabel(key: string, value: number): string {
  return LABELS[key]?.(value) ?? `${key} ${value}`;
}

/** Where the experience bar stands, 0 to 1. */
export function levelProgress(xp: number, levelXp: number, nextXp: number): number {
  if (nextXp <= levelXp) return 1;
  return Math.min(1, Math.max(0, (xp - levelXp) / (nextXp - levelXp)));
}
