// Mirror of server/internal/protocol/messages.go. Change both together.

export const PROTOCOL_VERSION = 3;

export type Dir = 'up' | 'down' | 'left' | 'right';

// ---------- client → server ----------

export interface Hello { t: 'hello'; version: number; playerId: string; name?: string }
export interface Move { t: 'move'; seq: number; dir: Dir }
/** item: a tool ("houe"…), a seed ("graine-navet"…) or "" for bare hands. */
export interface Use { t: 'use'; seq: number; item: string }
/** Spend one combat tick doing nothing (only au pas). */
export interface Wait { t: 'wait'; seq: number }
/** Place a building with its top-left tile at x, y; it must cover the faced tile. */
export interface Build { t: 'build'; seq: number; kind: string; x: number; y: number }
/** Take down the faced building; its cost comes back. */
export interface Dismantle { t: 'dismantle'; seq: number }
/** Move items between the bag and the house chest; count <= 0 moves the whole stack. */
export interface ChestMove { t: 'chest_move'; seq: number; op: 'deposer' | 'prendre'; item: string; count: number }
export type ClientMessage = Hello | Move | Use | Wait | Build | Dismantle | ChestMove;

// ---------- server → client ----------

export interface PlayerView { id: string; name: string; x: number; y: number; facing: Dir; sleeping?: boolean; hp: number }

export interface ResourceView {
  id: number;
  kind: string; // arbre | rocher | herbes
  x: number;
  y: number;
  hits: number;
  depleted: boolean;
  removed?: boolean; // uprooted for good
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

export interface StatsView {
  hp: number;
  maxHp: number;
  energy: number;
  maxEnergy: number;
  water: number;
  waterMax: number;
}

export interface Tile { x: number; y: number }

export interface CreatureView {
  id: number;
  kind: string;
  x: number;
  y: number;
  facing: Dir;
  hp: number;
  maxHp: number;
  armor?: number;
  burrowed?: boolean;
  stunned?: boolean;
  wave?: boolean;
  warn?: Tile[]; // tiles its next blow will hit
}

export interface BuildingView {
  id: number;
  kind: string;
  x: number; // top-left tile
  y: number;
  w: number;
  h: number;
  hp?: number;
  load?: number;
  ready?: number;
}

/** engaged: "au pas" (step-by-step combat) · ready: the world waits for you */
export interface TurnView { engaged: boolean; ready: boolean; tick: number }

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
  creatures: CreatureView[];
  buildings: BuildingView[];
  claim: string; // one char per tile, row by row: '1' inside the domain
  turn: TurnView;
}
export interface PlayerMsg { t: 'player'; player: PlayerView }
export interface PlayerLeft { t: 'player_left'; id: string }
export interface ResourceMsg { t: 'resource'; resource: ResourceView }
export interface InventoryMsg { t: 'inventory'; items: Record<string, number> }
export interface PlotMsg { t: 'plot'; plot: PlotView }
export interface PlotsMsg { t: 'plots'; plots: PlotView[] }
export interface ClockMsg { t: 'clock'; clock: ClockView }
export interface StatsMsg { t: 'stats'; stats: StatsView }
export type NoticeCode =
  | 'morning'
  | 'fainted'
  | 'waiting'
  | 'ko'
  | 'wave'
  | 'wave_over'
  | 'uprooted'
  | 'workshop'
  | 'dried'
  | 'drying';
export interface NoticeMsg { t: 'notice'; code: NoticeCode }
export interface CreatureMsg { t: 'creature'; creature: CreatureView }
export interface CreatureGone { t: 'creature_gone'; id: number; how: 'mort' | 'fuite' }
export interface BuildingMsg { t: 'building'; building: BuildingView }
export interface BuildingGone { t: 'building_gone'; id: number }
export interface ClaimMsg { t: 'claim'; claim: string }
export interface ChestMsg { t: 'chest'; items: Record<string, number>; open?: boolean }
export interface TurnMsg { t: 'turn'; turn: TurnView }
export interface HitMsg { t: 'hit'; target: 'player' | 'creature' | 'building'; id: string; x: number; y: number; amount: number }
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
  | CreatureMsg
  | CreatureGone
  | BuildingMsg
  | BuildingGone
  | ClaimMsg
  | ChestMsg
  | TurnMsg
  | HitMsg
  | Ack
  | ErrorMsg;
