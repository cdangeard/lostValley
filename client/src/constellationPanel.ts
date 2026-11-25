import type { ProgressView } from './net/protocol';
import { canRefund, canTake, graph, modLabel, reachable, type Constellation, type Graph, type TalentNode } from './talents';

const SVG = 'http://www.w3.org/2000/svg';
const RADIUS: Record<string, number> = { coeur: 13, petit: 6, notable: 10, angulaire: 14 };
const TYPE_LABEL: Record<string, string> = {
  coeur: 'Cœur',
  petit: 'Petit nœud',
  notable: 'Nœud notable',
  angulaire: 'Pierre angulaire',
};
const REGIONS: Record<string, { nom: string; x: number; y: number }> = {
  terre: { nom: 'Terre', x: 0, y: -440 },
  sylve: { nom: 'Sylve', x: 440, y: 0 },
  lame: { nom: 'Lame', x: 0, y: 450 },
  forge: { nom: 'Forge', x: -440, y: 0 },
};
const MIN_ZOOM = 0.6;
const MAX_ZOOM = 3.5;

/**
 * The talent screen: the whole constellation in SVG, zoomed with the wheel or
 * the buttons and moved by dragging. A click takes a node, a right click (or
 * shift + click) gives it back.
 */
export class ConstellationPanel {
  private readonly root = document.getElementById('talents')!;
  private readonly g: Graph;
  private readonly svg: SVGSVGElement;
  private readonly world: SVGGElement;
  private readonly header: HTMLElement;
  private readonly tip: HTMLElement;
  private progress: ProgressView = { level: 1, xp: 0, levelXp: 0, nextXp: 0, points: 0, talents: [] };
  private taken = new Set<string>();
  private zoom = 1;
  private pan = { x: 0, y: 0 };
  private drag: { x: number; y: number; px: number; py: number; moved: boolean } | null = null;
  private hover: string | null = null;

  constructor(
    data: Constellation,
    private readonly onTake: (id: string) => void,
    private readonly onRefund: (id: string) => void,
    private readonly onClose: () => void,
  ) {
    this.g = graph(data);
    this.header = document.createElement('div');
    this.header.className = 'head';
    const tools = document.createElement('div');
    tools.className = 'tools';
    for (const [label, title, fn] of [
      ['+', 'Zoomer', () => this.zoomBy(1.25)],
      ['−', 'Dézoomer', () => this.zoomBy(0.8)],
      ['⌂', 'Recentrer', () => this.resetView()],
      ['×', 'Fermer (T ou Échap)', () => this.onClose()],
    ] as const) {
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = label;
      b.title = title;
      b.addEventListener('click', fn);
      tools.append(b);
    }
    this.svg = document.createElementNS(SVG, 'svg');
    this.svg.setAttribute('viewBox', '-480 -480 960 960');
    this.svg.setAttribute('role', 'img');
    this.svg.setAttribute('aria-label', 'Constellation des Racines');
    this.world = document.createElementNS(SVG, 'g');
    this.svg.append(this.world);
    this.tip = document.createElement('div');
    this.tip.className = 'tip';
    this.root.replaceChildren(this.header, tools, this.svg, this.tip);
    this.listen();
  }

  get open(): boolean {
    return this.root.classList.contains('show');
  }

  show(open: boolean): void {
    this.root.classList.toggle('show', open);
    if (open) this.render();
    else this.tip.classList.remove('show');
  }

  setProgress(p: ProgressView): void {
    this.progress = p;
    this.taken = new Set(p.talents);
    if (this.open) this.render();
  }

  // ---------- drawing ----------

  private render(): void {
    const p = this.progress;
    const pts = p.points;
    this.header.innerHTML =
      `<b>Constellation des Racines</b> · Niveau ${p.level} · ` +
      `<span class="${pts > 0 ? 'points' : ''}">${pts} point${pts > 1 ? 's' : ''} à placer</span>` +
      `<span class="hint">Clic : prendre · Clic droit : retirer (1 graine d'oubli) · Molette : zoom · Glisser : déplacer</span>`;
    const parts: SVGElement[] = [];
    for (const r of Object.values(REGIONS)) {
      const t = document.createElementNS(SVG, 'text');
      t.setAttribute('x', String(r.x));
      t.setAttribute('y', String(r.y));
      t.setAttribute('class', 'region');
      t.textContent = r.nom;
      parts.push(t);
    }
    for (const [a, b] of this.linkPairs()) {
      const na = this.g.nodes.get(a)!;
      const nb = this.g.nodes.get(b)!;
      const line = document.createElementNS(SVG, 'line');
      line.setAttribute('x1', String(na.x));
      line.setAttribute('y1', String(na.y));
      line.setAttribute('x2', String(nb.x));
      line.setAttribute('y2', String(nb.y));
      const lit = this.isTaken(a) && this.isTaken(b);
      const open = (this.isTaken(a) && reachable(this.g, this.taken, b)) || (this.isTaken(b) && reachable(this.g, this.taken, a));
      line.setAttribute('class', lit ? 'link lit' : open ? 'link open' : 'link');
      parts.push(line);
    }
    for (const n of this.g.nodes.values()) parts.push(this.nodeEl(n, pts));
    // Names of the notable nodes and keystones, under them.
    for (const n of this.g.nodes.values()) {
      if (n.type !== 'notable' && n.type !== 'angulaire') continue;
      const t = document.createElementNS(SVG, 'text');
      t.setAttribute('x', String(n.x));
      t.setAttribute('y', String(n.y + RADIUS[n.type] + 13));
      t.setAttribute('class', `label ${n.type}`);
      t.textContent = n.nom;
      parts.push(t);
    }
    this.world.replaceChildren(...parts);
    this.applyView();
    if (this.hover) this.showTip(this.hover);
  }

  private linkPairs(): [string, string][] {
    const out: [string, string][] = [];
    for (const [a, list] of this.g.adj) for (const b of list) if (a < b) out.push([a, b]);
    return out;
  }

  private isTaken(id: string): boolean {
    return id === this.g.heart || this.taken.has(id);
  }

  private nodeEl(n: TalentNode, points: number): SVGElement {
    const el = document.createElementNS(SVG, 'circle');
    el.setAttribute('cx', String(n.x));
    el.setAttribute('cy', String(n.y));
    el.setAttribute('r', String(RADIUS[n.type] ?? 6));
    const state = this.isTaken(n.id)
      ? 'taken'
      : canTake(this.g, this.taken, n.id, points)
        ? 'can'
        : reachable(this.g, this.taken, n.id)
          ? 'next'
          : 'far';
    el.setAttribute('class', `node ${n.type} ${n.region} ${state}`);
    el.dataset.id = n.id;
    return el;
  }

  // ---------- tooltip ----------

  private showTip(id: string): void {
    const n = this.g.nodes.get(id);
    if (!n) return;
    const mods = Object.entries(n.mods)
      .filter(([k]) => k !== 'mainVerte' && k !== 'meneurDeMeute')
      .map(([k, v]) => `<li>${modLabel(k, v)}</li>`)
      .join('');
    let status = '';
    if (n.type === 'coeur') status = 'Toujours acquis.';
    else if (this.taken.has(id))
      status = canRefund(this.g, this.taken, id)
        ? 'Acquis. Clic droit pour le retirer (1 graine d’oubli).'
        : 'Acquis. D’autres nœuds en dépendent.';
    else if (canTake(this.g, this.taken, id, this.progress.points)) status = 'Clic pour le prendre.';
    else if (reachable(this.g, this.taken, id)) status = 'À portée : il faut un point de talent (un niveau de plus).';
    else status = 'Trop loin : prends d’abord un nœud voisin.';
    this.tip.innerHTML =
      `<div class="name">${n.nom}</div><div class="type ${n.region}">${TYPE_LABEL[n.type] ?? n.type}</div>` +
      (mods ? `<ul>${mods}</ul>` : '') +
      (n.description ? `<p>${n.description}</p>` : '') +
      `<div class="status">${status}</div>`;
    this.tip.classList.add('show');
  }

  // ---------- pan and zoom ----------

  private applyView(): void {
    this.world.setAttribute('transform', `translate(${this.pan.x} ${this.pan.y}) scale(${this.zoom})`);
  }

  private resetView(): void {
    this.zoom = 1;
    this.pan = { x: 0, y: 0 };
    this.applyView();
  }

  /** Zoom by factor around a point in viewBox units (the centre by default). */
  private zoomBy(factor: number, at = { x: 0, y: 0 }): void {
    const z = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, this.zoom * factor));
    const k = z / this.zoom;
    this.pan = { x: at.x - (at.x - this.pan.x) * k, y: at.y - (at.y - this.pan.y) * k };
    this.zoom = z;
    this.applyView();
  }

  /** A screen point in viewBox units. */
  private toView(clientX: number, clientY: number): { x: number; y: number } {
    const box = this.svg.getBoundingClientRect();
    const size = Math.min(box.width, box.height);
    return {
      x: ((clientX - box.left - box.width / 2) / size) * 960,
      y: ((clientY - box.top - box.height / 2) / size) * 960,
    };
  }

  private listen(): void {
    this.svg.addEventListener(
      'wheel',
      (ev) => {
        ev.preventDefault();
        this.zoomBy(ev.deltaY < 0 ? 1.15 : 1 / 1.15, this.toView(ev.clientX, ev.clientY));
      },
      { passive: false },
    );
    this.svg.addEventListener('pointerdown', (ev) => {
      if (ev.button !== 0) return;
      this.drag = { x: ev.clientX, y: ev.clientY, px: this.pan.x, py: this.pan.y, moved: false };
    });
    window.addEventListener('pointermove', (ev) => {
      if (this.drag) {
        const a = this.toView(this.drag.x, this.drag.y);
        const b = this.toView(ev.clientX, ev.clientY);
        if (Math.abs(ev.clientX - this.drag.x) + Math.abs(ev.clientY - this.drag.y) > 4) this.drag.moved = true;
        this.pan = { x: this.drag.px + b.x - a.x, y: this.drag.py + b.y - a.y };
        this.applyView();
      }
    });
    window.addEventListener('pointerup', () => {
      setTimeout(() => (this.drag = null));
    });
    this.svg.addEventListener('click', (ev) => {
      if (this.drag?.moved) return;
      const id = (ev.target as SVGElement).dataset?.id;
      if (!id) return;
      if (ev.shiftKey) this.refund(id);
      else if (canTake(this.g, this.taken, id, this.progress.points)) this.onTake(id);
    });
    this.svg.addEventListener('contextmenu', (ev) => {
      ev.preventDefault();
      const id = (ev.target as SVGElement).dataset?.id;
      if (id) this.refund(id);
    });
    this.svg.addEventListener('pointerover', (ev) => {
      const id = (ev.target as SVGElement).dataset?.id;
      if (id) {
        this.hover = id;
        this.showTip(id);
      }
    });
    this.svg.addEventListener('pointerout', (ev) => {
      if ((ev.target as SVGElement).dataset?.id) {
        this.hover = null;
        this.tip.classList.remove('show');
      }
    });
  }

  private refund(id: string): void {
    if (canRefund(this.g, this.taken, id)) this.onRefund(id);
  }
}
