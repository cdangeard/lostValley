import type { ConnectionState } from './net/Connection';

const ITEM_LABELS: Record<string, string> = { bois: 'Bois', pierre: 'Pierre', fibre: 'Fibre' };
const ITEM_ORDER = ['bois', 'pierre', 'fibre'];

/** The HTML overlay: connection status and the bag. Heavier menus will live here too. */
export class Hud {
  private readonly status = document.getElementById('status')!;
  private readonly inventory = document.getElementById('inventory')!;
  private items: Record<string, number> = {};

  setConnection(state: ConnectionState, players = 0): void {
    this.status.classList.toggle('online', state === 'online');
    this.status.textContent =
      state === 'online'
        ? `En ligne · ${players} ${players > 1 ? 'personnes' : 'personne'} dans la vallée`
        : state === 'connecting'
          ? 'Connexion…'
          : 'Hors ligne, nouvelle tentative…';
  }

  setInventory(items: Record<string, number>): void {
    const before = this.items;
    this.items = { ...items };
    const keys = Object.keys(items)
      .filter((k) => items[k] > 0)
      .sort((a, b) => rank(a) - rank(b));
    this.inventory.replaceChildren(
      ...keys.map((k) => {
        const el = document.createElement('div');
        el.className = 'item';
        const count = document.createElement('span');
        count.className = 'count';
        count.textContent = String(items[k]);
        if ((before[k] ?? 0) < items[k]) count.classList.add('bump');
        el.append(ITEM_LABELS[k] ?? k, count);
        return el;
      }),
    );
  }
}

function rank(item: string): number {
  const i = ITEM_ORDER.indexOf(item);
  return i === -1 ? ITEM_ORDER.length : i;
}
