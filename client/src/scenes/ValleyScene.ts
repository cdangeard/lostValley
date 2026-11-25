import Phaser from 'phaser';
import { ActionPacer, STEP_MS, type Action } from '../ActionPacer';
import { LocalClock, cropStage, darkness } from '../clock';
import type { Content } from '../content';
import { DISMANTLE, Hud } from '../hud';
import { Connection, playerId, serverUrl } from '../net/Connection';
import type {
  BuildingView,
  CreatureView,
  Dir,
  HitMsg,
  PlayerView,
  PlotView,
  ResourceView,
  ServerMessage,
  TurnView,
} from '../net/protocol';
import { footprint, footprintTiles } from '../placement';

const TILE = 16;
// Must match the names in the Tiled map (see server/internal/world/tiled.go).
const LAYER_GROUND = 'sol';
const LAYER_MARKERS = 'reperes';
const MARKER_BED = 'lit';
const MARKER_CHEST = 'coffre';

// Frames of sprites/entities.png (see server/cmd/genassets).
const RESOURCE_FRAMES: Record<string, [alive: number, depleted: number]> = {
  arbre: [0, 1],
  rocher: [2, 3],
  herbes: [4, 5],
};
const PLAYER_FRAMES: Record<Dir, number> = { down: 6, left: 7, right: 8, up: 9 };
const F_BED = 10;
const F_SOIL_DRY = 11;
const F_SOIL_WET = 12;
const STAGE_FRAMES = { seed: 13, sprout: 14, young: 15 };
const RIPE_FRAMES: Record<string, number> = {
  navet: 16,
  ble: 17,
  feve: 18,
  'navet-lune': 19,
  'ble-ambre': 20,
  'feve-ronce': 21,
};
const F_WITHERED = 22;
const F_WEEDS = 23;
const CREATURE_FRAMES: Record<string, [up: number, burrowed: number]> = {
  'rongeur-racine': [24, 25],
  'corbeau-voleur': [26, 26],
  ronceau: [27, 27],
  'scarabee-galet': [28, 28],
};
/** One frame per tile of the footprint, row by row. */
const BUILDING_FRAMES: Record<string, number[]> = {
  atelier: [29, 30, 31, 32],
  puits: [33],
  sechoir: [34, 35],
  epouvantail: [36],
  cloture: [37],
};
const F_CHEST = 38;

const DIR_STEP: Record<Dir, [number, number]> = { up: [0, -1], down: [0, 1], left: [-1, 0], right: [1, 0] };

// What a refused action means to the player. Silent ones are left out.
const REASONS: Record<string, string> = {
  wrong_tool: "Ce n'est pas le bon outil.",
  exhausted: 'Trop fatigué… Va dormir.',
  empty_can: "L'arrosoir est vide : remplis-le à la rivière.",
  no_item: "Tu n'as plus cet objet.",
  cannot_build: 'On ne peut pas construire ici : il faut un terrain libre, dans le domaine.',
  needs_plan: "Il faut d'abord un atelier (ou d'autres bâtiments en dépendent).",
  missing_materials: 'Il manque des matériaux dans le sac.',
  full_health: 'Tu es déjà en pleine forme.',
  not_now: 'Pas question de dormir avec une bête dans les parages !',
};
const NOTICES: Record<string, string> = {
  morning: 'Une nouvelle journée commence. Partie sauvegardée.',
  fainted: "Tu t'es effondré de fatigue… Réveil à la maison, avec moins d'énergie.",
  waiting: 'Au lit. La nuit passera quand tout le monde dormira.',
  ko: 'Tu es tombé… Réveil à la maison. Le sac et tout son contenu sont perdus.',
  wave: 'Des nuisibles sortent de la forêt et foncent sur les champs !',
  wave_over: "L'aube chasse les nuisibles vers la forêt.",
  uprooted: 'Souche arrachée : le domaine s’agrandit.',
  dried: 'Rations séchées récupérées.',
  drying: 'Récoltes suspendues : elles seront sèches demain matin.',
};

interface PlayerSprite {
  view: PlayerView;
  sprite: Phaser.GameObjects.Sprite;
  label: Phaser.GameObjects.Text;
}

interface CreatureSprite {
  view: CreatureView;
  sprite: Phaser.GameObjects.Sprite;
  bar: Phaser.GameObjects.Graphics;
  warn: Phaser.GameObjects.Rectangle[];
}

interface BuildingSprite {
  view: BuildingView;
  parts: Phaser.GameObjects.Image[];
  label: Phaser.GameObjects.Text;
}

interface PlotSprite {
  view: PlotView;
  soil: Phaser.GameObjects.Image;
  plant: Phaser.GameObjects.Image;
}

/** The valley: draws what the server says, and sends what the player wants. */
export class ValleyScene extends Phaser.Scene {
  private conn!: Connection;
  private hud!: Hud;
  private me = '';
  private players = new Map<string, PlayerSprite>();
  private resources = new Map<number, { view: ResourceView; sprite: Phaser.GameObjects.Sprite }>();
  private plots = new Map<string, PlotSprite>();
  private creatures = new Map<number, CreatureSprite>();
  private buildings = new Map<number, BuildingSprite>();
  private claim = '';
  private claimLayer!: Phaser.GameObjects.Graphics;
  private ghost!: Phaser.GameObjects.Graphics;
  private building = ''; // what the build menu has chosen: a kind, DISMANTLE or ''
  private turn: TurnView = { engaged: false, ready: false, tick: 0 };
  private chestTile: [number, number] | null = null;
  private bedTile: [number, number] | null = null;
  private groundLayer!: Phaser.Tilemaps.TilemapLayer;
  private pacer = new ActionPacer();
  private clock = new LocalClock();
  private selected = '';
  private facingMarker!: Phaser.GameObjects.Rectangle;
  private night!: Phaser.GameObjects.Rectangle;
  private rain!: Phaser.GameObjects.Particles.ParticleEmitter;
  private keys!: Record<'up' | 'down' | 'left' | 'right' | 'use' | 'wait', Phaser.Input.Keyboard.Key[]>;

  constructor() {
    super('valley');
  }

  create(): void {
    const map = this.make.tilemap({ key: 'valley' });
    const tiles = map.addTilesetImage('terrain', 'terrain');
    if (!tiles) throw new Error('tileset "terrain" missing from the map');
    this.groundLayer = map.createLayer(LAYER_GROUND, tiles, 0, 0)!;

    const markers = map.getObjectLayer(LAYER_MARKERS)?.objects ?? [];
    for (const [name, frame] of [[MARKER_BED, F_BED], [MARKER_CHEST, F_CHEST]] as const) {
      const o = markers.find((m) => m.name === name);
      if (!o) continue;
      const tx = Math.floor((o.x ?? 0) / TILE);
      const ty = Math.floor((o.y ?? 0) / TILE);
      if (name === MARKER_BED) this.bedTile = [tx, ty];
      else this.chestTile = [tx, ty];
      this.add.image(tx * TILE + TILE / 2, ty * TILE + TILE / 2, 'entities', frame).setDepth(ty * TILE + TILE / 2 - 1);
    }
    this.claimLayer = this.add.graphics().setDepth(0.6).setVisible(false);
    this.ghost = this.add.graphics().setDepth(1e6 + 1);

    this.cameras.main.setBounds(0, 0, map.widthInPixels, map.heightInPixels);
    this.fitZoom();
    this.scale.on('resize', () => this.fitZoom());

    this.facingMarker = this.add
      .rectangle(0, 0, TILE, TILE)
      .setStrokeStyle(1, 0xf3ead2, 0.55)
      .setDepth(1e6)
      .setVisible(false);

    // Night: one dark veil over the whole map, thicker as the hours pass.
    this.night = this.add
      .rectangle(0, 0, map.widthInPixels, map.heightInPixels, 0x0b1640)
      .setOrigin(0, 0)
      .setDepth(2e6)
      .setAlpha(0);

    const g = this.add.graphics();
    g.fillStyle(0xb9d4f0, 0.8).fillRect(0, 0, 1, 4);
    g.generateTexture('drop', 1, 4);
    g.destroy();
    this.rain = this.add.particles(0, 0, 'drop', {
      x: { min: 0, max: 1 },
      speedY: { min: 220, max: 280 },
      speedX: -30,
      lifespan: 900,
      quantity: 3,
      frequency: 30,
      alpha: { start: 0.7, end: 0.2 },
      emitting: false,
    });
    this.rain.setDepth(2e6 + 1);

    const kb = this.input.keyboard!;
    const k = (code: number) => kb.addKey(code, true);
    const K = Phaser.Input.Keyboard.KeyCodes;
    // Arrows, ZQSD (AZERTY) and WASD (QWERTY) all work.
    this.keys = {
      up: [k(K.UP), k(K.Z), k(K.W)],
      down: [k(K.DOWN), k(K.S)],
      left: [k(K.LEFT), k(K.Q), k(K.A)],
      right: [k(K.RIGHT), k(K.D)],
      use: [k(K.SPACE), k(K.E)],
      wait: [k(K.X)],
    };
    k(K.B).on('down', () => this.toggleBuild());
    k(K.ESC).on('down', () => this.closePanels());
    // 1–9 pick a hotbar slot; AZERTY rows send the same key codes.
    const digits = [K.ONE, K.TWO, K.THREE, K.FOUR, K.FIVE, K.SIX, K.SEVEN, K.EIGHT, K.NINE];
    const numpad = [K.NUMPAD_ONE, K.NUMPAD_TWO, K.NUMPAD_THREE, K.NUMPAD_FOUR, K.NUMPAD_FIVE, K.NUMPAD_SIX,
      K.NUMPAD_SEVEN, K.NUMPAD_EIGHT, K.NUMPAD_NINE];
    [digits, numpad].forEach((codes) =>
      codes.forEach((code, i) =>
        k(code).on('down', () => {
          if (this.hud.buildOpen) this.pickBuilding(this.hud.buildKinds[i]);
          else this.pick(i);
        }),
      ),
    );
    this.input.on('wheel', (_p: unknown, _o: unknown, _dx: number, dy: number) => this.cycle(dy > 0 ? 1 : -1));

    this.hud = new Hud(
      this.cache.json.get('contenu') as Content,
      (i) => this.pick(i),
      (kind) => this.pickBuilding(kind),
      (op, item) => this.conn.send({ t: 'chest_move', seq: 0, op, item, count: 0 }),
    );
    this.conn = new Connection({
      url: serverUrl(),
      playerId: playerId(),
      onMessage: (msg) => this.onMessage(msg),
      onState: (state) => {
        if (state !== 'online') this.pacer.reset();
        this.hud.setConnection(state, this.players.size);
      },
    });
    this.conn.open();
    this.events.once(Phaser.Scenes.Events.SHUTDOWN, () => this.conn.close());
  }

  update(time: number): void {
    const wanted = this.wantedAction();
    const next = this.pacer.next(time, wanted);
    if (next) {
      const a = next.action;
      const seq = next.seq;
      let sent = false;
      switch (a.kind) {
        case 'move':
          sent = this.conn.send({ t: 'move', seq, dir: a.dir });
          break;
        case 'use':
          sent = this.conn.send({ t: 'use', seq, item: a.item });
          break;
        case 'wait':
          sent = this.conn.send({ t: 'wait', seq });
          break;
        case 'build':
          sent = this.conn.send({ t: 'build', seq, kind: a.building, x: a.x, y: a.y });
          break;
        case 'dismantle':
          sent = this.conn.send({ t: 'dismantle', seq });
          break;
      }
      if (!sent) this.pacer.reset();
    }
    this.updateFacingMarker();
    this.updateGhost();
    this.updateSky();
  }

  private wantedAction(): Action | null {
    // Au pas, nothing goes out until the world waits for us.
    if (this.turn.engaged && !this.turn.ready) return null;
    const down = (keys: Phaser.Input.Keyboard.Key[]) => keys.some((key) => key.isDown);
    for (const dir of ['up', 'down', 'left', 'right'] as const) {
      if (down(this.keys[dir])) {
        if (this.hud.chestOpen) this.hud.showChest(false);
        return { kind: 'move', dir };
      }
    }
    if (down(this.keys.wait) && this.turn.engaged) return { kind: 'wait' };
    if (down(this.keys.use)) {
      if (this.building === DISMANTLE) return { kind: 'dismantle' };
      if (this.building) {
        const spot = this.buildSpot();
        if (spot) return { kind: 'build', building: this.building, x: spot.x, y: spot.y };
        return null;
      }
      if (this.hud.chestOpen) return null;
      return { kind: 'use', item: this.selected };
    }
    return null;
  }

  // ---------- build mode ----------

  private toggleBuild(): void {
    if (this.hud.buildOpen) {
      this.closePanels();
      return;
    }
    this.hud.showChest(false);
    this.hud.setBuilt(this.builtKinds());
    this.hud.showBuild(true, this.building);
    this.claimLayer.setVisible(true);
  }

  private pickBuilding(kind: string | undefined): void {
    if (!kind) return;
    const def = this.content.batiments[kind];
    if (def?.requiert && !this.builtKinds().has(def.requiert)) {
      this.hud.say(`Il faut d'abord : ${this.content.batiments[def.requiert]?.nom ?? def.requiert}.`);
      return;
    }
    this.building = this.building === kind ? '' : kind;
    this.hud.showBuild(true, this.building);
    this.claimLayer.setVisible(true);
  }

  private closePanels(): void {
    this.building = '';
    this.hud.showBuild(false, '');
    this.hud.showChest(false);
    this.claimLayer.setVisible(false);
    this.ghost.clear();
  }

  private builtKinds(): Set<string> {
    return new Set([...this.buildings.values()].map((b) => b.view.kind));
  }

  /** The footprint of the chosen building in front of the player. */
  private buildSpot(): { x: number; y: number; w: number; h: number } | null {
    const me = this.players.get(this.me);
    const def = this.content.batiments[this.building];
    if (!me || !def) return null;
    const tl = footprint(me.view.x, me.view.y, me.view.facing, def.largeur, def.hauteur);
    return { ...tl, w: def.largeur, h: def.hauteur };
  }

  /** A client-side guess of whether a tile can be built on; the server decides. */
  private tileFree(x: number, y: number): boolean {
    const w = this.groundLayer.layer.width;
    if (x < 0 || y < 0 || x >= w || y >= this.groundLayer.layer.height) return false;
    if (this.claim[y * w + x] !== '1') return false;
    if (this.groundLayer.getTileAt(x, y)?.properties?.bloquant) return false;
    const at = (t: [number, number] | null) => t !== null && t[0] === x && t[1] === y;
    if (at(this.bedTile) || at(this.chestTile) || this.plots.has(`${x},${y}`)) return false;
    for (const r of this.resources.values()) if (r.view.x === x && r.view.y === y) return false;
    for (const b of this.buildings.values()) {
      if (x >= b.view.x && x < b.view.x + b.view.w && y >= b.view.y && y < b.view.y + b.view.h) return false;
    }
    for (const c of this.creatures.values()) if (c.view.x === x && c.view.y === y) return false;
    for (const p of this.players.values()) if (p.view.x === x && p.view.y === y) return false;
    return true;
  }

  private updateGhost(): void {
    this.ghost.clear();
    if (!this.building || this.building === DISMANTLE) return;
    const spot = this.buildSpot();
    if (!spot) return;
    for (const [x, y] of footprintTiles(spot.x, spot.y, spot.w, spot.h)) {
      const ok = this.tileFree(x, y);
      this.ghost.fillStyle(ok ? 0x9be36b : 0xd9694a, 0.35).fillRect(x * TILE, y * TILE, TILE, TILE);
      this.ghost.lineStyle(1, ok ? 0x9be36b : 0xd9694a, 0.9).strokeRect(x * TILE + 0.5, y * TILE + 0.5, TILE - 1, TILE - 1);
    }
  }

  private drawClaim(): void {
    const g = this.claimLayer;
    g.clear();
    const w = this.groundLayer.layer.width;
    g.fillStyle(0xf3d27a, 0.12);
    for (let i = 0; i < this.claim.length; i++) {
      if (this.claim[i] === '1') g.fillRect((i % w) * TILE, Math.floor(i / w) * TILE, TILE, TILE);
    }
    // The border, so the edge of the domain reads at a glance.
    g.lineStyle(1, 0xf3d27a, 0.7);
    const inside = (x: number, y: number) => x >= 0 && x < w && this.claim[y * w + x] === '1';
    for (let i = 0; i < this.claim.length; i++) {
      if (this.claim[i] !== '1') continue;
      const x = i % w;
      const y = Math.floor(i / w);
      const px = x * TILE;
      const py = y * TILE;
      if (!inside(x, y - 1)) g.lineBetween(px, py + 0.5, px + TILE, py + 0.5);
      if (!inside(x, y + 1)) g.lineBetween(px, py + TILE - 0.5, px + TILE, py + TILE - 0.5);
      if (!inside(x - 1, y)) g.lineBetween(px + 0.5, py, px + 0.5, py + TILE);
      if (!inside(x + 1, y)) g.lineBetween(px + TILE - 0.5, py, px + TILE - 0.5, py + TILE);
    }
  }

  private get content(): Content {
    return this.cache.json.get('contenu') as Content;
  }

  // ---------- hotbar ----------

  private pick(index: number): void {
    const slot = this.hud.hotbarSlots[index];
    if (!slot) return;
    this.selected = slot.item;
    this.hud.select(slot.item);
  }

  private cycle(step: number): void {
    const slots = this.hud.hotbarSlots;
    if (slots.length === 0) return;
    const i = slots.findIndex((s) => s.item === this.selected);
    this.pick((i + step + slots.length) % slots.length);
  }

  /** Keep a valid selection when the bag changes (e.g. the last seed sown). */
  private keepSelection(): void {
    const slots = this.hud.hotbarSlots;
    if (!slots.some((s) => s.item === this.selected) && slots.length > 0) this.pick(0);
  }

  // ---------- server messages ----------

  private onMessage(msg: ServerMessage): void {
    switch (msg.t) {
      case 'welcome':
        this.reset();
        this.me = msg.you;
        msg.resources.forEach((r) => this.upsertResource(r));
        msg.plots.forEach((p) => this.upsertPlot(p));
        msg.players.forEach((p) => this.upsertPlayer(p));
        this.clock.set(msg.clock, this.time.now);
        this.hud.setTools(msg.tools);
        this.hud.setInventory(msg.inventory ?? {});
        this.hud.setStats(msg.stats);
        msg.creatures.forEach((c) => this.upsertCreature(c));
        msg.buildings.forEach((b) => this.upsertBuilding(b));
        this.claim = msg.claim;
        this.drawClaim();
        this.setTurn(msg.turn);
        this.keepSelection();
        break;
      case 'player':
        this.upsertPlayer(msg.player);
        break;
      case 'player_left':
        this.removePlayer(msg.id);
        break;
      case 'resource':
        this.upsertResource(msg.resource);
        break;
      case 'plot':
        this.upsertPlot(msg.plot);
        break;
      case 'plots':
        this.replacePlots(msg.plots);
        break;
      case 'inventory':
        this.hud.setInventory(msg.items);
        this.keepSelection();
        break;
      case 'clock':
        this.clock.set(msg.clock, this.time.now);
        break;
      case 'stats':
        this.hud.setStats(msg.stats);
        break;
      case 'notice':
        if (msg.code === 'workshop') {
          if (!this.hud.buildOpen) this.toggleBuild();
          break;
        }
        this.hud.say(NOTICES[msg.code] ?? msg.code, ['fainted', 'ko', 'wave'].includes(msg.code) ? 4500 : 2600);
        if (msg.code === 'ko') this.cameras.main.flash(600, 120, 20, 20);
        break;
      case 'creature':
        this.upsertCreature(msg.creature);
        break;
      case 'creature_gone':
        this.removeCreature(msg.id, msg.how === 'mort');
        break;
      case 'building':
        this.upsertBuilding(msg.building);
        if (this.hud.buildOpen) this.hud.setBuilt(this.builtKinds());
        break;
      case 'building_gone':
        this.removeBuilding(msg.id);
        if (this.hud.buildOpen) this.hud.setBuilt(this.builtKinds());
        break;
      case 'claim':
        this.claim = msg.claim;
        this.drawClaim();
        break;
      case 'chest':
        if (msg.open || this.hud.chestOpen) {
          this.closePanels();
          this.hud.showChest(true, msg.items);
        }
        break;
      case 'turn':
        this.setTurn(msg.turn);
        break;
      case 'hit':
        this.showHit(msg);
        break;
      case 'ack':
        this.pacer.acked(msg.seq);
        if (!msg.ok && msg.reason && REASONS[msg.reason]) this.hud.say(REASONS[msg.reason]);
        break;
      case 'error':
        console.warn('server error:', msg.message);
        break;
    }
    this.hud.setConnection('online', this.players.size);
  }

  private reset(): void {
    this.pacer.reset();
    this.players.forEach((p) => {
      p.sprite.destroy();
      p.label.destroy();
    });
    this.players.clear();
    this.resources.forEach((r) => r.sprite.destroy());
    this.resources.clear();
    this.replacePlots([]);
    [...this.creatures.keys()].forEach((id) => this.removeCreature(id, false));
    [...this.buildings.keys()].forEach((id) => this.removeBuilding(id));
  }

  private setTurn(turn: TurnView): void {
    const entering = turn.engaged && !this.turn.engaged;
    this.turn = turn;
    this.hud.setTurn(turn);
    if (entering) this.pacer.reset();
  }

  // ---------- creatures ----------

  private upsertCreature(view: CreatureView): void {
    const frames = CREATURE_FRAMES[view.kind] ?? [24, 24];
    const x = view.x * TILE + TILE / 2;
    const y = view.y * TILE + TILE / 2;
    let c = this.creatures.get(view.id);
    if (!c) {
      const sprite = this.add.sprite(x, y, 'entities', frames[0]).setAlpha(0);
      this.tweens.add({ targets: sprite, alpha: 1, duration: 300 });
      c = { view, sprite, bar: this.add.graphics().setDepth(3e6), warn: [] };
      this.creatures.set(view.id, c);
    } else if (c.view.x !== view.x || c.view.y !== view.y) {
      this.tweens.killTweensOf(c.sprite);
      const far = Math.abs(view.x - c.view.x) + Math.abs(view.y - c.view.y) > 1;
      if (far) c.sprite.setPosition(x, y);
      else this.tweens.add({ targets: c.sprite, x, y, duration: STEP_MS, ease: 'Linear' });
    }
    c.view = view;
    c.sprite
      .setFrame(view.burrowed ? frames[1] : frames[0])
      .setFlipX(view.facing === 'left')
      .setDepth(y + 0.4)
      .setTint(view.stunned ? 0xb0b0ff : 0xffffff);
    // Health bar, once hurt.
    c.bar.clear();
    if (view.hp < view.maxHp) {
      const w = 12;
      c.bar.fillStyle(0x1d1d1d, 0.8).fillRect(x - w / 2, y - 10, w, 2);
      c.bar.fillStyle(0xd2564a, 1).fillRect(x - w / 2, y - 10, (w * view.hp) / view.maxHp, 2);
    }
    // The tiles its next blow will hit, in red.
    c.warn.forEach((r) => r.destroy());
    c.warn = (view.warn ?? []).map((t) => {
      const r = this.add
        .rectangle(t.x * TILE + TILE / 2, t.y * TILE + TILE / 2, TILE - 1, TILE - 1, 0xe0442f, 0.38)
        .setStrokeStyle(1, 0xff7a5c, 0.9)
        .setDepth(0.9);
      this.tweens.add({ targets: r, alpha: 0.55, duration: 380, yoyo: true, repeat: -1 });
      return r;
    });
  }

  private removeCreature(id: number, killed: boolean): void {
    const c = this.creatures.get(id);
    if (!c) return;
    this.creatures.delete(id);
    c.bar.destroy();
    c.warn.forEach((r) => r.destroy());
    this.tweens.killTweensOf(c.sprite);
    this.tweens.add({
      targets: c.sprite,
      alpha: 0,
      scale: killed ? 1.5 : 0.6,
      duration: killed ? 260 : 500,
      onComplete: () => c.sprite.destroy(),
    });
  }

  private showHit(hit: HitMsg): void {
    const x = hit.x * TILE + TILE / 2;
    const y = hit.y * TILE;
    const color = hit.target === 'player' ? '#ff8a73' : hit.amount === 0 ? '#bdbdbd' : '#fff4d6';
    const text = this.add
      .text(x, y, hit.amount === 0 ? 'paré' : `-${hit.amount}`, {
        fontFamily: 'system-ui, sans-serif',
        fontSize: '8px',
        fontStyle: 'bold',
        color,
        stroke: '#1d1d1d',
        strokeThickness: 2,
        resolution: 4,
      })
      .setOrigin(0.5, 1)
      .setDepth(3e6 + 1);
    this.tweens.add({ targets: text, y: y - 10, alpha: 0, duration: 750, ease: 'Cubic.Out', onComplete: () => text.destroy() });
    if (hit.target === 'player' && hit.id === this.me) {
      this.cameras.main.shake(140, 0.004);
    }
    if (hit.target === 'creature') {
      const c = this.creatures.get(Number(hit.id));
      if (c) {
        c.sprite.setTintFill(0xffffff);
        this.time.delayedCall(90, () => c.sprite.active && c.sprite.clearTint());
      }
    }
  }

  // ---------- buildings ----------

  private upsertBuilding(view: BuildingView): void {
    const frames = BUILDING_FRAMES[view.kind] ?? [];
    let b = this.buildings.get(view.id);
    if (!b) {
      const parts = footprintTiles(view.x, view.y, view.w, view.h).map(([tx, ty], i) => {
        const px = tx * TILE + TILE / 2;
        const py = ty * TILE + TILE / 2;
        return this.add.image(px, py, 'entities', frames[i] ?? frames[0] ?? 0).setDepth(py);
      });
      const label = this.add
        .text((view.x + view.w / 2) * TILE, view.y * TILE - 1, '', {
          fontFamily: 'system-ui, sans-serif',
          fontSize: '6px',
          color: '#f3ead2',
          stroke: '#1d2b1f',
          strokeThickness: 2,
          resolution: 4,
        })
        .setOrigin(0.5, 1)
        .setDepth(3e6);
      b = { view, parts, label };
      this.buildings.set(view.id, b);
      parts.forEach((p) => p.setScale(0.6));
      this.tweens.add({ targets: parts, scale: 1, duration: 220, ease: 'Back.Out' });
    } else if ((view.hp ?? 0) < (b.view.hp ?? 0)) {
      b.parts.forEach((p) => p.setTint(0xff9a8a));
      this.time.delayedCall(120, () => b?.parts.forEach((p) => p.active && p.clearTint()));
    }
    b.view = view;
    b.label.setText(
      view.kind === 'sechoir'
        ? view.ready
          ? `${view.ready} rations prêtes`
          : view.load
            ? `Séchage : ${view.load}`
            : ''
        : '',
    );
  }

  private removeBuilding(id: number): void {
    const b = this.buildings.get(id);
    if (!b) return;
    this.buildings.delete(id);
    b.label.destroy();
    this.tweens.add({ targets: b.parts, alpha: 0, duration: 200, onComplete: () => b.parts.forEach((p) => p.destroy()) });
  }

  private upsertPlayer(view: PlayerView): void {
    const x = view.x * TILE + TILE / 2;
    const y = view.y * TILE + TILE / 2;
    let p = this.players.get(view.id);
    if (!p) {
      const sprite = this.add.sprite(x, y, 'entities', PLAYER_FRAMES[view.facing]);
      const label = this.add
        .text(x, y - 11, view.name, {
          fontFamily: 'system-ui, sans-serif',
          fontSize: '6px',
          color: view.id === this.me ? '#f3d27a' : '#f3ead2',
          stroke: '#1d2b1f',
          strokeThickness: 2,
          resolution: 4,
        })
        .setOrigin(0.5, 1)
        .setDepth(3e6);
      p = { view, sprite, label };
      this.players.set(view.id, p);
      if (view.id === this.me) this.cameras.main.startFollow(sprite, true, 0.2, 0.2);
    } else {
      const dist = Math.abs(view.x - p.view.x) + Math.abs(view.y - p.view.y);
      this.tweens.killTweensOf([p.sprite, p.label]);
      if (dist === 1) {
        this.tweens.add({ targets: p.sprite, x, y, duration: STEP_MS, ease: 'Linear' });
        this.tweens.add({ targets: p.label, x, y: y - 11, duration: STEP_MS, ease: 'Linear' });
      } else {
        p.sprite.setPosition(x, y);
        p.label.setPosition(x, y - 11);
      }
      p.view = view;
    }
    p.label.setText(view.sleeping ? `${view.name} · zZ` : view.name);
    p.sprite.setFrame(PLAYER_FRAMES[view.facing]).setDepth(y + 0.5).setAlpha(view.sleeping ? 0.75 : 1);
  }

  private removePlayer(id: string): void {
    const p = this.players.get(id);
    if (!p) return;
    p.sprite.destroy();
    p.label.destroy();
    this.players.delete(id);
  }

  private upsertResource(view: ResourceView): void {
    const frames = RESOURCE_FRAMES[view.kind];
    if (!frames) return;
    if (view.removed) {
      const gone = this.resources.get(view.id);
      if (gone) {
        this.resources.delete(view.id);
        this.tweens.add({ targets: gone.sprite, alpha: 0, scale: 0.5, duration: 250, onComplete: () => gone.sprite.destroy() });
      }
      return;
    }
    const x = view.x * TILE + TILE / 2;
    const y = view.y * TILE + TILE / 2;
    const existing = this.resources.get(view.id);
    const sprite = existing?.sprite ?? this.add.sprite(x, y, 'entities').setDepth(y);
    sprite.setFrame(view.depleted ? frames[1] : frames[0]);
    if (existing && !view.depleted && view.hits < existing.view.hits) {
      // A strike that did not finish it: a little shake.
      this.tweens.killTweensOf(sprite);
      sprite.x = x;
      this.tweens.add({ targets: sprite, x: x + 1.5, duration: 40, yoyo: true, repeat: 1 });
    }
    if (existing && view.depleted && !existing.view.depleted) {
      this.tweens.killTweensOf(sprite);
      sprite.setPosition(x, y).setAlpha(0.4);
      this.tweens.add({ targets: sprite, alpha: 1, duration: 250 });
    }
    this.resources.set(view.id, { view, sprite });
  }

  // ---------- plots ----------

  private replacePlots(views: PlotView[]): void {
    const keep = new Set(views.map((v) => `${v.x},${v.y}`));
    for (const [key, p] of this.plots) {
      if (!keep.has(key)) {
        p.soil.destroy();
        p.plant.destroy();
        this.plots.delete(key);
      }
    }
    views.forEach((v) => this.upsertPlot(v));
  }

  private upsertPlot(view: PlotView): void {
    const key = `${view.x},${view.y}`;
    let p = this.plots.get(key);
    if (!view.state) {
      p?.soil.destroy();
      p?.plant.destroy();
      this.plots.delete(key);
      return;
    }
    const x = view.x * TILE + TILE / 2;
    const y = view.y * TILE + TILE / 2;
    if (!p) {
      p = {
        view,
        soil: this.add.image(x, y, 'entities', F_SOIL_DRY).setDepth(0.5),
        plant: this.add.image(x, y, 'entities', STAGE_FRAMES.seed).setDepth(y - 2),
      };
      this.plots.set(key, p);
    }
    const grew = (view.growth ?? 0) > (p.view.growth ?? 0);
    p.view = view;
    p.soil.setVisible(view.state !== 'friche').setFrame(view.watered ? F_SOIL_WET : F_SOIL_DRY);
    const frame = this.plantFrame(view);
    p.plant.setVisible(frame !== null);
    if (frame !== null) p.plant.setFrame(frame);
    if (grew) {
      p.plant.setScale(0.8);
      this.tweens.add({ targets: p.plant, scale: 1, duration: 300, ease: 'Back.Out' });
    }
  }

  private plantFrame(view: PlotView): number | null {
    switch (view.state) {
      case 'friche':
        return F_WEEDS;
      case 'fane':
        return F_WITHERED;
      case 'culture': {
        const stage = cropStage(view.growth ?? 0, view.days ?? 1);
        return stage === 'ripe' ? (RIPE_FRAMES[view.crop ?? ''] ?? STAGE_FRAMES.young) : STAGE_FRAMES[stage];
      }
      default:
        return null;
    }
  }

  // ---------- sky ----------

  private updateSky(): void {
    const view = this.clock.current;
    if (!view) return;
    const minute = this.clock.minute(this.time.now);
    const rain = view.weather === 'pluie';
    this.night.setAlpha(Math.min(0.75, darkness(minute) + (rain ? 0.12 : 0)));
    const cam = this.cameras.main.worldView;
    this.rain.setPosition(cam.x - 20, cam.y - 10);
    this.rain.ops.x.loadConfig({ x: { min: 0, max: cam.width + 40 } });
    if (rain !== this.rain.emitting) {
      if (rain) this.rain.start();
      else this.rain.stop();
    }
    this.hud.setClock(view, minute);
  }

  private updateFacingMarker(): void {
    const me = this.players.get(this.me);
    if (!me) {
      this.facingMarker.setVisible(false);
      return;
    }
    const [dx, dy] = DIR_STEP[me.view.facing];
    const tx = me.view.x + dx;
    const ty = me.view.y + dy;
    const resource = [...this.resources.values()].some((r) => r.view.x === tx && r.view.y === ty && !r.view.depleted);
    const plot = this.plots.get(`${tx},${ty}`);
    const ripe = plot?.view.state === 'culture' && (plot.view.growth ?? 0) >= (plot.view.days ?? 1);
    const foe = [...this.creatures.values()].some((c) => c.view.x === tx && c.view.y === ty);
    const color = foe ? 0xff7a5c : ripe ? 0x9be36b : resource || plot ? 0xe0b25c : 0xf3ead2;
    this.facingMarker
      .setPosition(tx * TILE + TILE / 2, ty * TILE + TILE / 2)
      .setStrokeStyle(1, color, foe || resource || plot ? 0.9 : 0.3)
      .setVisible(!this.building || this.building === DISMANTLE);
  }

  /** Show roughly 22 × 14 tiles whatever the window size, in whole zoom steps. */
  private fitZoom(): void {
    const { width, height } = this.scale.gameSize;
    const zoom = Math.max(1, Math.floor(Math.min(width / (22 * TILE), height / (14 * TILE))));
    this.cameras.main.setZoom(zoom);
  }
}
