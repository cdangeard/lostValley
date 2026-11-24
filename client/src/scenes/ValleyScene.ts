import Phaser from 'phaser';
import { ActionPacer, STEP_MS, type Action } from '../ActionPacer';
import { LocalClock, cropStage, darkness } from '../clock';
import type { Content } from '../content';
import { Hud } from '../hud';
import { Connection, playerId, serverUrl } from '../net/Connection';
import type { Dir, PlayerView, PlotView, ResourceView, ServerMessage } from '../net/protocol';

const TILE = 16;
// Must match the names in the Tiled map (see server/internal/world/tiled.go).
const LAYER_GROUND = 'sol';
const LAYER_MARKERS = 'reperes';
const MARKER_BED = 'lit';

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

const DIR_STEP: Record<Dir, [number, number]> = { up: [0, -1], down: [0, 1], left: [-1, 0], right: [1, 0] };

// What a refused action means to the player. Silent ones are left out.
const REASONS: Record<string, string> = {
  wrong_tool: "Ce n'est pas le bon outil.",
  exhausted: 'Trop fatigué… Va dormir.',
  empty_can: "L'arrosoir est vide : remplis-le à la rivière.",
  no_item: "Tu n'as plus cet objet.",
};
const NOTICES: Record<string, string> = {
  morning: 'Une nouvelle journée commence. Partie sauvegardée.',
  fainted: "Tu t'es effondré de fatigue… Réveil à la maison, avec moins d'énergie.",
  waiting: 'Au lit. La nuit passera quand tout le monde dormira.',
};

interface PlayerSprite {
  view: PlayerView;
  sprite: Phaser.GameObjects.Sprite;
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
  private pacer = new ActionPacer();
  private clock = new LocalClock();
  private selected = '';
  private facingMarker!: Phaser.GameObjects.Rectangle;
  private night!: Phaser.GameObjects.Rectangle;
  private rain!: Phaser.GameObjects.Particles.ParticleEmitter;
  private keys!: Record<'up' | 'down' | 'left' | 'right' | 'use', Phaser.Input.Keyboard.Key[]>;

  constructor() {
    super('valley');
  }

  create(): void {
    const map = this.make.tilemap({ key: 'valley' });
    const tiles = map.addTilesetImage('terrain', 'terrain');
    if (!tiles) throw new Error('tileset "terrain" missing from the map');
    map.createLayer(LAYER_GROUND, tiles, 0, 0);

    const bed = map.getObjectLayer(LAYER_MARKERS)?.objects.find((o) => o.name === MARKER_BED);
    if (bed) {
      const bx = Math.floor((bed.x ?? 0) / TILE) * TILE + TILE / 2;
      const by = Math.floor((bed.y ?? 0) / TILE) * TILE + TILE / 2;
      this.add.image(bx, by, 'entities', F_BED).setDepth(by - 1);
    }

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
    };
    // 1–9 pick a hotbar slot; AZERTY rows send the same key codes.
    const digits = [K.ONE, K.TWO, K.THREE, K.FOUR, K.FIVE, K.SIX, K.SEVEN, K.EIGHT, K.NINE];
    const numpad = [K.NUMPAD_ONE, K.NUMPAD_TWO, K.NUMPAD_THREE, K.NUMPAD_FOUR, K.NUMPAD_FIVE, K.NUMPAD_SIX,
      K.NUMPAD_SEVEN, K.NUMPAD_EIGHT, K.NUMPAD_NINE];
    [digits, numpad].forEach((codes) => codes.forEach((code, i) => k(code).on('down', () => this.pick(i))));
    this.input.on('wheel', (_p: unknown, _o: unknown, _dx: number, dy: number) => this.cycle(dy > 0 ? 1 : -1));

    this.hud = new Hud(this.cache.json.get('contenu') as Content, (i) => this.pick(i));
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
      const sent =
        next.action.kind === 'move'
          ? this.conn.send({ t: 'move', seq: next.seq, dir: next.action.dir })
          : this.conn.send({ t: 'use', seq: next.seq, item: next.action.item });
      if (!sent) this.pacer.reset();
    }
    this.updateFacingMarker();
    this.updateSky();
  }

  private wantedAction(): Action | null {
    const down = (keys: Phaser.Input.Keyboard.Key[]) => keys.some((key) => key.isDown);
    for (const dir of ['up', 'down', 'left', 'right'] as const) {
      if (down(this.keys[dir])) return { kind: 'move', dir };
    }
    if (down(this.keys.use)) return { kind: 'use', item: this.selected };
    return null;
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
        this.hud.say(NOTICES[msg.code] ?? msg.code, msg.code === 'fainted' ? 4500 : 2600);
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
    const color = ripe ? 0x9be36b : resource || plot ? 0xe0b25c : 0xf3ead2;
    this.facingMarker
      .setPosition(tx * TILE + TILE / 2, ty * TILE + TILE / 2)
      .setStrokeStyle(1, color, resource || plot ? 0.9 : 0.3)
      .setVisible(true);
  }

  /** Show roughly 22 × 14 tiles whatever the window size, in whole zoom steps. */
  private fitZoom(): void {
    const { width, height } = this.scale.gameSize;
    const zoom = Math.max(1, Math.floor(Math.min(width / (22 * TILE), height / (14 * TILE))));
    this.cameras.main.setZoom(zoom);
  }
}
