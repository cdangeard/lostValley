import Phaser from 'phaser';
import { ActionPacer, STEP_MS, type Action } from '../ActionPacer';
import { Hud } from '../hud';
import { Connection, playerId, serverUrl } from '../net/Connection';
import type { Dir, PlayerView, ResourceView, ServerMessage } from '../net/protocol';

const TILE = 16;
// Must match the layer names in the Tiled map (see server/internal/world/tiled.go).
const LAYER_GROUND = 'sol';

// Frames of sprites/entities.png (see server/cmd/genassets).
const RESOURCE_FRAMES: Record<string, [alive: number, depleted: number]> = {
  arbre: [0, 1],
  rocher: [2, 3],
  herbes: [4, 5],
};
const PLAYER_FRAMES: Record<Dir, number> = { down: 6, left: 7, right: 8, up: 9 };

const DIR_STEP: Record<Dir, [number, number]> = { up: [0, -1], down: [0, 1], left: [-1, 0], right: [1, 0] };

interface PlayerSprite {
  view: PlayerView;
  sprite: Phaser.GameObjects.Sprite;
  label: Phaser.GameObjects.Text;
}

/** The valley: draws what the server says, and sends what the player wants. */
export class ValleyScene extends Phaser.Scene {
  private conn!: Connection;
  private hud!: Hud;
  private me = '';
  private players = new Map<string, PlayerSprite>();
  private resources = new Map<number, { view: ResourceView; sprite: Phaser.GameObjects.Sprite }>();
  private pacer = new ActionPacer();
  private facingMarker!: Phaser.GameObjects.Rectangle;
  private keys!: Record<'up' | 'down' | 'left' | 'right' | 'gather', Phaser.Input.Keyboard.Key[]>;

  constructor() {
    super('valley');
  }

  create(): void {
    const map = this.make.tilemap({ key: 'valley' });
    const tiles = map.addTilesetImage('terrain', 'terrain');
    if (!tiles) throw new Error('tileset "terrain" missing from the map');
    map.createLayer(LAYER_GROUND, tiles, 0, 0);

    this.cameras.main.setBounds(0, 0, map.widthInPixels, map.heightInPixels);
    this.fitZoom();
    this.scale.on('resize', () => this.fitZoom());

    this.facingMarker = this.add
      .rectangle(0, 0, TILE, TILE)
      .setStrokeStyle(1, 0xf3ead2, 0.55)
      .setDepth(1e6)
      .setVisible(false);

    const kb = this.input.keyboard!;
    const k = (code: number) => kb.addKey(code, true);
    const K = Phaser.Input.Keyboard.KeyCodes;
    // Arrows, ZQSD (AZERTY) and WASD (QWERTY) all work.
    this.keys = {
      up: [k(K.UP), k(K.Z), k(K.W)],
      down: [k(K.DOWN), k(K.S)],
      left: [k(K.LEFT), k(K.Q), k(K.A)],
      right: [k(K.RIGHT), k(K.D)],
      gather: [k(K.SPACE), k(K.E)],
    };

    this.hud = new Hud();
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
          : this.conn.send({ t: 'gather', seq: next.seq });
      if (!sent) this.pacer.reset();
    }
    this.updateFacingMarker();
  }

  private wantedAction(): Action | null {
    const down = (keys: Phaser.Input.Keyboard.Key[]) => keys.some((key) => key.isDown);
    for (const dir of ['up', 'down', 'left', 'right'] as const) {
      if (down(this.keys[dir])) return { kind: 'move', dir };
    }
    if (down(this.keys.gather)) return { kind: 'gather' };
    return null;
  }

  // ---------- server messages ----------

  private onMessage(msg: ServerMessage): void {
    switch (msg.t) {
      case 'welcome':
        this.reset();
        this.me = msg.you;
        msg.resources.forEach((r) => this.upsertResource(r));
        msg.players.forEach((p) => this.upsertPlayer(p));
        this.hud.setInventory(msg.inventory ?? {});
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
      case 'inventory':
        this.hud.setInventory(msg.items);
        break;
      case 'ack':
        this.pacer.acked(msg.seq);
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
        .setDepth(1e6);
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
      p.label.setText(view.name);
      p.view = view;
    }
    p.sprite.setFrame(PLAYER_FRAMES[view.facing]).setDepth(y + 0.5);
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

  private updateFacingMarker(): void {
    const me = this.players.get(this.me);
    if (!me) {
      this.facingMarker.setVisible(false);
      return;
    }
    const [dx, dy] = DIR_STEP[me.view.facing];
    const tx = me.view.x + dx;
    const ty = me.view.y + dy;
    const target = [...this.resources.values()].find((r) => r.view.x === tx && r.view.y === ty && !r.view.depleted);
    this.facingMarker
      .setPosition(tx * TILE + TILE / 2, ty * TILE + TILE / 2)
      .setStrokeStyle(1, target ? 0xe0b25c : 0xf3ead2, target ? 0.9 : 0.3)
      .setVisible(true);
  }

  /** Show roughly 22 × 14 tiles whatever the window size, in whole zoom steps. */
  private fitZoom(): void {
    const { width, height } = this.scale.gameSize;
    const zoom = Math.max(1, Math.floor(Math.min(width / (22 * TILE), height / (14 * TILE))));
    this.cameras.main.setZoom(zoom);
  }
}
