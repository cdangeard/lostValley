import Phaser from 'phaser';

/** Loads the shared assets (served from ../assets) before entering the valley. */
export class BootScene extends Phaser.Scene {
  constructor() {
    super('boot');
  }

  preload(): void {
    this.load.tilemapTiledJSON('valley', 'maps/valley.json');
    this.load.image('terrain', 'tiles/terrain.png');
    this.load.spritesheet('entities', 'sprites/entities.png', { frameWidth: 16, frameHeight: 16 });
    this.load.json('contenu', 'data/contenu.json');
  }

  create(): void {
    this.scene.start('valley');
  }
}
