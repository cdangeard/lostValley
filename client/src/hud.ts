import { formatTime } from './clock';
import { buildHotbar, buildState, costLabel, itemLabel, type Content, type HotbarSlot } from './content';
import type { ConnectionState } from './net/Connection';
import type { ClockView, ProgressView, StatsView, TurnView } from './net/protocol';
import { levelProgress } from './talents';

const ITEM_ORDER = [
  'bois', 'pierre', 'fibre', 'navet', 'ble', 'feve', 'navet-lune', 'ble-ambre', 'feve-ronce', 'ration',
  'lait-de-chevre', 'dent-de-rongeur', 'plume-noire', 'epine', 'eclat-de-carapace',
];

/** What the build menu offers besides buildings. */
export const DISMANTLE = 'demonter';

/** The HTML overlay: status, clock, energy, hotbar, bag and short messages. */
export class Hud {
  private readonly status = document.getElementById('status')!;
  private readonly clock = document.getElementById('clock')!;
  private readonly energy = document.getElementById('energy')!;
  private readonly inventory = document.getElementById('inventory')!;
  private readonly hotbar = document.getElementById('hotbar')!;
  private readonly toast = document.getElementById('toast')!;
  private readonly turn = document.getElementById('turn')!;
  private readonly buildMenu = document.getElementById('build')!;
  private readonly chestPanel = document.getElementById('chest')!;
  private readonly level = document.getElementById('level')!;
  private built = new Set<string>();
  private buildChoice = '';
  private chestItems: Record<string, number> = {};
  private items: Record<string, number> = {};
  private tools: string[] = [];
  private slots: HotbarSlot[] = [];
  private selectedItem = '';
  private toastTimer = 0;

  constructor(
    private readonly content: Content,
    private readonly onPick: (index: number) => void,
    private readonly onBuildPick: (kind: string) => void,
    private readonly onChest: (op: 'deposer' | 'prendre', item: string) => void,
  ) {
    this.hotbar.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-slot]');
      if (el) this.onPick(Number(el.dataset.slot));
    });
    this.buildMenu.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-kind]');
      if (el && !el.hasAttribute('disabled')) this.onBuildPick(el.dataset.kind!);
    });
    this.chestPanel.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-op]');
      if (el) this.onChest(el.dataset.op as 'deposer' | 'prendre', el.dataset.item!);
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
    const hp = Math.round((100 * s.hp) / Math.max(1, s.maxHp));
    this.energy.innerHTML =
      `<span class="label">Vie ${s.hp}/${s.maxHp}</span><span class="bar hp"><span style="width:${hp}%"></span></span>` +
      `<span class="label">Énergie</span><span class="bar"><span style="width:${pct}%"></span></span>` +
      `<span class="label">Arrosoir ${s.water}/${s.waterMax}</span>`;
    this.energy.classList.toggle('low', pct <= 15);
    this.energy.classList.toggle('hurt', hp <= 30);
  }

  /** Level, experience and talent points waiting to be spent. */
  setProgress(p: ProgressView): void {
    const pct = Math.round(100 * levelProgress(p.xp, p.levelXp, p.nextXp));
    const pts = p.points > 0 ? `<span class="points">${p.points} point${p.points > 1 ? 's' : ''} · T</span>` : '<span class="key">T : talents</span>';
    this.level.innerHTML =
      `<span class="label">Niveau ${p.level}</span><span class="bar xp"><span style="width:${pct}%"></span></span>${pts}`;
    this.level.classList.toggle('ready', p.points > 0);
  }

  /** The combat indicator: hidden outside combat. */
  setTurn(t: TurnView): void {
    this.turn.classList.toggle('show', t.engaged);
    this.turn.classList.toggle('ready', t.engaged && t.ready);
    this.turn.textContent = !t.engaged
      ? ''
      : t.ready
        ? 'Au pas · à toi de jouer (X : attendre)'
        : 'Au pas · les bêtes bougent…';
  }

  // ---------- build menu ----------

  get buildOpen(): boolean {
    return this.buildMenu.classList.contains('show');
  }

  /** The kinds the menu offers, in order: what keys 1–9 pick while it is open. */
  get buildKinds(): string[] {
    return [...Object.keys(this.content.batiments), DISMANTLE];
  }

  setBuilt(kinds: Set<string>): void {
    this.built = kinds;
    if (this.buildOpen) this.renderBuild();
  }

  showBuild(open: boolean, choice = this.buildChoice): void {
    this.buildChoice = choice;
    this.buildMenu.classList.toggle('show', open);
    if (open) this.renderBuild();
  }

  private renderBuild(): void {
    const rows = this.buildKinds.map((kind, i) => {
      const el = document.createElement('button');
      el.type = 'button';
      el.className = 'entry';
      el.dataset.kind = kind;
      if (kind === this.buildChoice) el.classList.add('selected');
      const key = `<span class="key">${i < 9 ? i + 1 : ''}</span>`;
      if (kind === DISMANTLE) {
        el.innerHTML = `${key}<b>Démonter</b><span class="cost">Rend tout le matériel</span>`;
        return el;
      }
      const def = this.content.batiments[kind];
      const state = buildState(def, this.items, this.built);
      el.classList.add(state);
      const why =
        state === 'locked' ? `Il faut d'abord : ${this.content.batiments[def.requiert ?? '']?.nom ?? def.requiert}` : def.description ?? '';
      el.innerHTML =
        `${key}<b>${def.nom}</b> <span class="size">${def.largeur}×${def.hauteur}</span>` +
        `<span class="cost">${costLabel(def.cout, this.content)}</span><span class="why">${why}</span>`;
      if (state === 'locked') el.setAttribute('disabled', '');
      return el;
    });
    const title = document.createElement('div');
    title.className = 'title';
    title.textContent = 'Construire · E pour poser, B ou Échap pour fermer';
    this.buildMenu.replaceChildren(title, ...rows);
  }

  // ---------- house chest ----------

  get chestOpen(): boolean {
    return this.chestPanel.classList.contains('show');
  }

  showChest(open: boolean, items?: Record<string, number>): void {
    if (items) this.chestItems = items;
    this.chestPanel.classList.toggle('show', open);
    if (open) this.renderChest();
  }

  private renderChest(): void {
    const column = (title: string, items: Record<string, number>, op: 'deposer' | 'prendre') => {
      const col = document.createElement('div');
      col.className = 'col';
      const h = document.createElement('div');
      h.className = 'title';
      h.textContent = title;
      col.append(h);
      const keys = Object.keys(items)
        .filter((k) => items[k] > 0)
        .sort((a, b) => rank(a) - rank(b));
      if (keys.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'empty';
        empty.textContent = 'Vide';
        col.append(empty);
      }
      for (const k of keys) {
        const b = document.createElement('button');
        b.type = 'button';
        b.dataset.op = op;
        b.dataset.item = k;
        b.innerHTML = `${itemLabel(k, this.content)} <span class="count">${items[k]}</span>`;
        col.append(b);
      }
      return col;
    };
    const note = document.createElement('div');
    note.className = 'note';
    note.textContent = 'Clique un objet pour le déplacer. Le coffre est à l’abri : si tu tombes, seul le sac est perdu.';
    this.chestPanel.replaceChildren(
      column('Sac', this.items, 'deposer'),
      column('Coffre', this.chestItems, 'prendre'),
      note,
    );
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
    if (this.buildOpen) this.renderBuild();
    if (this.chestOpen) this.renderChest();
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
