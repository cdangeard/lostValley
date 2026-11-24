// Mirror of server/internal/protocol/messages.go. Change both together.

export const PROTOCOL_VERSION = 2;

export type Dir = 'up' | 'down' | 'left' | 'right';

// ---------- client → server ----------

export interface Hello { t: 'hello'; version: number; playerId: string; name?: string }
export interface Move { t: 'move'; seq: number; dir: Dir }
/** item: a tool ("houe"…), a seed ("graine-navet"…) or "" for bare hands. */
export interface Use { t: 'use'; seq: number; item: string }
export type ClientMessage = Hello | Move | Use;

// ---------- server → client ----------

export interface PlayerView { id: string; name: string; x: number; y: number; facing: Dir; sleeping?: boolean }

export interface ResourceView {
  id: number;
  kind: string; // arbre | rocher | herbes
  x: number;
  y: number;
  hits: number;
  depleted: boolean;
}

/** labour: tilled soil · culture: growing or ripe · fane: withered · friche: weeds · '': gone */
export type PlotState = 'labour' | 'culture' | 'fane' | 'friche' | '';

export interface PlotView {
  x: number;
  y: number;
  state: PlotState;
  crop?: string;
  growth?: number;
  days?: number;
  watered?: boolean;
}

export interface ClockView {
  day: number;
  minute: number; // minutes since midnight
  year: number;
  season: string;
  seasonDay: number;
  weather: 'soleil' | 'pluie';
  minuteMs: number; // real ms per game minute
}

export interface StatsView { energy: number; maxEnergy: number; water: number; waterMax: number }

export interface Welcome {
  t: 'welcome';
  version: number;
  you: string;
  map: string;
  players: PlayerView[];
  resources: ResourceView[];
  plots: PlotView[];
  clock: ClockView;
  tools: string[];
  inventory: Record<string, number> | null;
  stats: StatsView;
}
export interface PlayerMsg { t: 'player'; player: PlayerView }
export interface PlayerLeft { t: 'player_left'; id: string }
export interface ResourceMsg { t: 'resource'; resource: ResourceView }
export interface InventoryMsg { t: 'inventory'; items: Record<string, number> }
export interface PlotMsg { t: 'plot'; plot: PlotView }
export interface PlotsMsg { t: 'plots'; plots: PlotView[] }
export interface ClockMsg { t: 'clock'; clock: ClockView }
export interface StatsMsg { t: 'stats'; stats: StatsView }
export interface NoticeMsg { t: 'notice'; code: 'morning' | 'fainted' | 'waiting' }
export interface Ack { t: 'ack'; seq: number; ok: boolean; reason?: string }
export interface ErrorMsg { t: 'error'; message: string }

export type ServerMessage =
  | Welcome
  | PlayerMsg
  | PlayerLeft
  | ResourceMsg
  | InventoryMsg
  | PlotMsg
  | PlotsMsg
  | ClockMsg
  | StatsMsg
  | NoticeMsg
  | Ack
  | ErrorMsg;
