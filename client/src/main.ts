import Phaser from 'phaser';
import { BootScene } from './scenes/BootScene';
import { ValleyScene } from './scenes/ValleyScene';

new Phaser.Game({
  type: Phaser.AUTO,
  parent: 'game',
  backgroundColor: '#1d2b1f',
  pixelArt: true,
  roundPixels: true,
  scale: { mode: Phaser.Scale.RESIZE, width: '100%', height: '100%' },
  scene: [BootScene, ValleyScene],
});
