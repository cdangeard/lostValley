import { itemLabel, type Content } from './content';
import type { BondView, DialogueMsg } from './net/protocol';
import { canCraft, heartsText, helpLabel, nextStep, recipeLabel, stageLabel, TASTE_MARK, Tastes } from './villagers';

const MOOD_NOTE: Record<string, string> = {
  adore: 'adore ce cadeau',
  aime: 'aime ce cadeau',
  neutre: 'reçoit le cadeau poliment',
  deteste: 'déteste ce cadeau',
};

/**
 * Talking with villagers: the dialogue box (lines one after the other, then
 * a gift from the bag) and the relations panel (R) with hearts, romance and
 * the romance tokens to make at the workshop.
 */
export class VillagePanel {
  private readonly box = document.getElementById('dialogue')!;
  private readonly panel = document.getElementById('village')!;
  readonly tastes = new Tastes();
  private lines: string[] = [];
  private current: DialogueMsg | null = null;
  private bonds: BondView[] = [];
  private bag: Record<string, number> = {};
  private tools = new Set<string>();
  private workshop = false;
  private giving = false;

  constructor(
    private readonly content: Content,
    private readonly onGift: (villager: string, item: string) => void,
    private readonly onCraft: (item: string) => void,
    private readonly onClose: () => void,
  ) {
    this.box.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-act]');
      if (!el) return;
      const act = el.dataset.act!;
      if (act === 'next') this.advance();
      else if (act === 'gift') this.showGifts(true);
      else if (act === 'back') this.showGifts(false);
      else if (act === 'close') this.close();
      else if (act === 'give' && this.current) this.onGift(this.current.villager, el.dataset.item!);
    });
    this.panel.addEventListener('click', (ev) => {
      const el = (ev.target as HTMLElement).closest<HTMLElement>('[data-craft]');
      if (el && !el.hasAttribute('disabled')) this.onCraft(el.dataset.craft!);
    });
  }

  get talking(): boolean {
    return this.current !== null;
  }

  get open(): boolean {
    return this.panel.classList.contains('show');
  }

  private name(id: string): string {
    return this.content.villageois?.[id]?.nom ?? id;
  }

  private bond(id: string): BondView {
    return this.bonds.find((b) => b.villager === id) ?? { villager: id, points: 0, hearts: 0, maxHearts: 8 };
  }

  setBonds(bonds: BondView[]): void {
    this.bonds = bonds;
    if (this.current) this.renderBox();
    if (this.open) this.renderPanel();
  }

  setBag(bag: Record<string, number>, tools: string[]): void {
    this.bag = bag;
    this.tools = new Set(tools);
    if (this.giving) this.renderBox();
    if (this.open) this.renderPanel();
  }

  setWorkshop(built: boolean): void {
    this.workshop = built;
    if (this.open) this.renderPanel();
  }

  /** A villager speaks: a new dialogue, or the answer to a gift. */
  say(msg: DialogueMsg, gift?: string): void {
    if (gift) this.tastes.learn(msg.villager, gift, msg.mood);
    this.current = msg;
    this.lines = [...msg.lines];
    this.giving = false;
    this.renderBox();
  }

  /** Next line, or close when the last one was read. Returns false once closed. */
  advance(): boolean {
    if (!this.current) return false;
    if (this.giving) {
      this.showGifts(false);
      return true;
    }
    if (this.lines.length > 1) {
      this.lines.shift();
      this.renderBox();
      return true;
    }
    this.close();
    return false;
  }

  close(): void {
    if (!this.current) return;
    this.current = null;
    this.giving = false;
    this.box.classList.remove('show');
    this.onClose();
  }

  private showGifts(on: boolean): void {
    this.giving = on;
    this.renderBox();
  }

  private renderBox(): void {
    const msg = this.current;
    if (!msg) return;
    const def = this.content.villageois?.[msg.villager];
    const b = this.bond(msg.villager);
    const head =
      `<div class="who"><b>${esc(this.name(msg.villager))}</b> <span class="title">${esc(def?.titre ?? '')}</span>` +
      `<span class="hearts" title="${b.points} points">${heartsText(b)}</span></div>`;
    let body: string;
    if (this.giving) {
      const items = Object.entries(this.bag)
        .filter(([item, n]) => n > 0 && !this.tools.has(item))
        .sort(([a], [z]) => itemLabel(a, this.content).localeCompare(itemLabel(z, this.content), 'fr'));
      body = items.length
        ? `<div class="gifts">${items
            .map(([item, n]) => {
              const mark = TASTE_MARK[this.tastes.of(msg.villager, item) ?? 'parle'] ?? '';
              return `<button type="button" data-act="give" data-item="${esc(item)}">${esc(itemLabel(item, this.content))}` +
                ` <span class="count">${n}</span>${mark ? ` <span class="taste">${mark}</span>` : ''}</button>`;
            })
            .join('')}</div>`
        : '<div class="line">Ton sac est vide.</div>';
      body += `<div class="actions"><button type="button" data-act="back">Retour</button></div>`;
    } else {
      const note = MOOD_NOTE[msg.mood] ? `<div class="note">${esc(this.name(msg.villager))} ${MOOD_NOTE[msg.mood]}` +
        `${msg.delta ? ` (${msg.delta > 0 ? '+' : ''}${msg.delta})` : ''}.</div>` : '';
      const more = this.lines.length > 1;
      body =
        `<div class="line${msg.mood === 'scene' ? ' scene' : ''}">${esc(this.lines[0] ?? '')}</div>${more ? '' : note}` +
        `<div class="actions">` +
        (more
          ? `<button type="button" data-act="next">Suite (E)</button>`
          : `<button type="button" data-act="gift"${b.gifted ? ' title="Déjà un cadeau aujourd\'hui"' : ''}>Offrir…</button>` +
            `<button type="button" data-act="close">Au revoir (E)</button>`) +
        `</div>`;
    }
    this.box.innerHTML = head + body;
    this.box.classList.add('show');
  }

  toggle(): boolean {
    const show = !this.open;
    this.panel.classList.toggle('show', show);
    if (show) this.renderPanel();
    return show;
  }

  hide(): void {
    this.panel.classList.remove('show');
  }

  private renderPanel(): void {
    const partner = this.bonds.find((b) => b.stage);
    const rows = this.bonds.map((b) => {
      const def = this.content.villageois?.[b.villager];
      const today = [b.talked ? 'parlé aujourd’hui' : 'pas encore parlé aujourd’hui', b.gifted ? 'cadeau offert' : '']
        .filter(Boolean)
        .join(' · ');
      const stage = stageLabel(b.stage, b.wedding);
      const next = nextStep(b, this.content, !!partner && partner.villager !== b.villager);
      const help = b.stage === 'marie' && def ? `${def.nom} ${helpLabel(def.aide)}.` : '';
      return (
        `<div class="villager"><div><b>${esc(def?.nom ?? b.villager)}</b> <span class="title">${esc(def?.titre ?? '')}</span></div>` +
        `<div class="hearts">${heartsText(b)}${stage ? ` <span class="stage">${esc(stage)}</span>` : ''}</div>` +
        `<div class="today">${today}</div>` +
        (next ? `<div class="next">${esc(next)}</div>` : '') +
        (help ? `<div class="next">${esc(help)}</div>` : '') +
        `</div>`
      );
    });
    const r = this.content.romance;
    const crafts = r
      ? [r.bouquet, r.pendentif]
          .map((item) => {
            const ok = this.workshop && canCraft(item, this.bag, this.content);
            return (
              `<button type="button" class="craft" data-craft="${esc(item)}"${ok ? '' : ' disabled'}>` +
              `<b>${esc(itemLabel(item, this.content))}</b> <span class="count">${this.bag[item] ?? 0}</span>` +
              `<span class="cost">${esc(recipeLabel(item, this.content))}</span></button>`
            );
          })
          .join('')
      : '';
    this.panel.innerHTML =
      `<div class="title">Villageois (R pour fermer)</div>` +
      (rows.length
        ? rows.join('')
        : '<div class="empty">Personne ne s’est encore installé. Fais grandir ton domaine : un atelier, une créature apprivoisée, une forêt qui recule…</div>') +
      `<div class="title">À fabriquer${this.workshop ? '' : ' (il faut un atelier)'}</div>${crafts}`;
  }
}

function esc(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
}
