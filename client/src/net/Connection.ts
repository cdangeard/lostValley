import { PROTOCOL_VERSION, type ClientMessage, type ServerMessage } from './protocol';

export type ConnectionState = 'connecting' | 'online' | 'offline';

export interface ConnectionOptions {
  url: string;
  playerId: string;
  name?: string;
  onMessage: (msg: ServerMessage) => void;
  onState: (state: ConnectionState) => void;
}

/** A WebSocket to the game server that says hello and reconnects by itself. */
export class Connection {
  private ws: WebSocket | null = null;
  private retry = 0;
  private closed = false;

  constructor(private readonly opts: ConnectionOptions) {}

  open(): void {
    this.closed = false;
    this.opts.onState('connecting');
    const ws = new WebSocket(this.opts.url);
    this.ws = ws;
    ws.onopen = () => {
      this.retry = 0;
      this.send({ t: 'hello', version: PROTOCOL_VERSION, playerId: this.opts.playerId, name: this.opts.name });
      this.opts.onState('online');
    };
    ws.onmessage = (ev) => {
      try {
        this.opts.onMessage(JSON.parse(ev.data as string) as ServerMessage);
      } catch (err) {
        console.error('bad server message', err);
      }
    };
    ws.onclose = () => {
      this.ws = null;
      this.opts.onState('offline');
      if (this.closed) return;
      const delay = Math.min(8000, 500 * 2 ** this.retry++);
      setTimeout(() => this.open(), delay);
    };
  }

  send(msg: ClientMessage): boolean {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return false;
    this.ws.send(JSON.stringify(msg));
    return true;
  }

  close(): void {
    this.closed = true;
    this.ws?.close();
  }
}

/** Where the server is: VITE_SERVER_URL, else /ws on the page's own host. */
export function serverUrl(): string {
  const configured = import.meta.env.VITE_SERVER_URL as string | undefined;
  if (configured) return configured;
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  return `${proto}://${location.host}/ws`;
}

/**
 * The browser's player token. Every tab of a browser shares it, so two tabs
 * drive the same character. `?joueur=<id>` overrides it to test two players
 * from one browser. Real accounts come in phase 4.
 */
export function playerId(): string {
  const fromUrl = new URLSearchParams(location.search).get('joueur');
  if (fromUrl && /^[A-Za-z0-9_-]{8,64}$/.test(fromUrl)) return fromUrl;
  const key = 'lostvalley.playerId';
  try {
    const saved = localStorage.getItem(key);
    if (saved) return saved;
    const id = randomId();
    localStorage.setItem(key, id);
    return id;
  } catch {
    return randomId();
  }
}

// crypto.randomUUID only exists on https or localhost; getRandomValues works everywhere.
function randomId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}
