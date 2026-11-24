import { formatTime } from './clock';
import { buildHotbar, itemLabel, type Content, type HotbarSlot } from './content';
import type { ConnectionState } from './net/Connection';
import type { ClockView, StatsView } from './net/protocol';

const ITEM_ORDER = ['bois', 'pierre', 'fibre', 'navet', 'ble', 'feve', 'navet-lune', 'ble-ambre', 'feve-ronce'];

/** The HTML overlay: status, clock, energy, hotbar, bag and short messages. */
export class Hud {
  private readonly status = document.getElementById('status')!;
  private readonly clock = document.getElementById('clock')!;
  private readonly energy = document.getElementById('energy')!;
  private readonly inventory = document.getElementById('inventory')!;
  private readonly hotbar = document.getElementById('hotbar')!;
  private readonly toast = document.getElementById('toast')!;
  private items: Record<string, number> = {};
  private tools: string[] = [];
  private slots: HotbarSlot[] = [];
  private selectedItem = '';
  private toastTimer = 0;

  constructor(
    private readonly content: Content,
    private readonly onPick: (index: number) => void,
  ) {
    this.hotbar.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-slot]');
      if (el) this.onPick(Number(el.dataset.slot));
    });
  }

  setConnection(state: ConnectionState, players = 0): void {
    this.status.classList.toggle('online', state === 'online');
    this.status.textContent =
      state === 'online'
        ? `En ligne · ${players} ${players > 1 ? 'personnes' : 'personne'} dans la vallée`
        : state === 'connecting'
          ? 'Connexion…'
          : 'Hors ligne, nouvelle tentative…';
  }

  setClock(view: ClockView, minute: number): void {
    const weather = view.weather === 'pluie' ? 'Pluie' : 'Soleil';
    const text = `${view.season}, jour ${view.seasonDay} · ${formatTime(minute)} · ${weather}`;
    if (this.clock.textContent !== text) this.clock.textContent = text;
  }

  setStats(s: StatsView): void {
    const pct = Math.round((100 * s.energy) / Math.max(1, s.maxEnergy));
    this.energy.innerHTML =
      `<span class="label">Énergie</span><span class="bar"><span style="width:${pct}%"></span></span>` +
      `<span class="label">Arrosoir ${s.water}/${s.waterMax}</span>`;
    this.energy.classList.toggle('low', pct <= 15);
  }

  setTools(tools: string[]): void {
    this.tools = tools;
    this.renderHotbar();
  }

  setInventory(items: Record<string, number>): void {
    const before = this.items;
    this.items = { ...items };
    const keys = Object.keys(items)
      .filter((k) => items[k] > 0 && !k.startsWith('graine-'))
      .sort((a, b) => rank(a) - rank(b));
    this.inventory.replaceChildren(
      ...keys.map((k) => {
        const el = document.createElement('div');
        el.className = 'item';
        const count = document.createElement('span');
        count.className = 'count';
        count.textContent = String(items[k]);
        if ((before[k] ?? 0) < items[k]) count.classList.add('bump');
        el.append(itemLabel(k, this.content), count);
        return el;
      }),
    );
    this.renderHotbar();
  }

  /** The slots of the hotbar, for the scene to pick from. */
  get hotbarSlots(): HotbarSlot[] {
    return this.slots;
  }

  select(item: string): void {
    this.selectedItem = item;
    this.renderHotbar();
  }

  say(text: string, ms = 2600): void {
    this.toast.textContent = text;
    this.toast.classList.add('show');
    window.clearTimeout(this.toastTimer);
    this.toastTimer = window.setTimeout(() => this.toast.classList.remove('show'), ms);
  }

  private renderHotbar(): void {
    this.slots = buildHotbar(this.tools, this.items, this.content);
    this.hotbar.replaceChildren(
      ...this.slots.map((s, i) => {
        const el = document.createElement('button');
        el.type = 'button';
        el.className = `slot ${s.kind}`;
        el.dataset.slot = String(i);
        if (s.item === this.selectedItem) el.classList.add('selected');
        const key = document.createElement('span');
        key.className = 'key';
        key.textContent = i < 9 ? String(i + 1) : '';
        el.append(key, s.label);
        if (s.count !== undefined) {
          const c = document.createElement('span');
          c.className = 'count';
          c.textContent = String(s.count);
          el.append(c);
        }
        return el;
      }),
    );
  }
}

function rank(item: string): number {
  const i = ITEM_ORDER.indexOf(item);
  return i === -1 ? ITEM_ORDER.length : i;
}
