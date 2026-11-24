// Mirror of server/internal/protocol/messages.go. Change both together.

export const PROTOCOL_VERSION = 1;

export type Dir = 'up' | 'down' | 'left' | 'right';

// ---------- client → server ----------

export interface Hello { t: 'hello'; version: number; playerId: string; name?: string }
export interface Move { t: 'move'; seq: number; dir: Dir }
export interface Gather { t: 'gather'; seq: number }
export type ClientMessage = Hello | Move | Gather;

// ---------- server → client ----------

export interface PlayerView { id: string; name: string; x: number; y: number; facing: Dir }

export interface ResourceView {
  id: number;
  kind: string; // arbre | rocher | herbes
  x: number;
  y: number;
  hits: number;
  depleted: boolean;
}

export interface Welcome {
  t: 'welcome';
  version: number;
  you: string;
  map: string;
  players: PlayerView[];
  resources: ResourceView[];
  inventory: Record<string, number> | null;
}
export interface PlayerMsg { t: 'player'; player: PlayerView }
export interface PlayerLeft { t: 'player_left'; id: string }
export interface ResourceMsg { t: 'resource'; resource: ResourceView }
export interface InventoryMsg { t: 'inventory'; items: Record<string, number> }
export interface Ack { t: 'ack'; seq: number; ok: boolean; reason?: string }
export interface ErrorMsg { t: 'error'; message: string }

export type ServerMessage =
  | Welcome
  | PlayerMsg
  | PlayerLeft
  | ResourceMsg
  | InventoryMsg
  | Ack
  | ErrorMsg;
