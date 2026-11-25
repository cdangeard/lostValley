import type { Dir } from './net/protocol';

/** How long one step takes on screen. The server allows one every 120 ms. */
export const STEP_MS = 160;
/** How long a tool swing takes. The server allows one every 250 ms. */
export const USE_MS = 300;

export type Action =
  | { kind: 'move'; dir: Dir }
  | { kind: 'use'; item: string }
  | { kind: 'wait' }
  | { kind: 'build'; building: string; x: number; y: number }
  | { kind: 'dismantle' };

/**
 * Decides when the player may send the next action. The server is the
 * authority: we wait for its answer to an action before sending another,
 * and never send faster than the animation, so honest play is never refused.
 */
export class ActionPacer {
  private pendingSeq: number | null = null;
  private busyUntil = 0;
  private seq = 0;

  /** Returns the action to send now with its sequence number, or null. */
  next(now: number, wanted: Action | null): { seq: number; action: Action } | null {
    if (!wanted || this.pendingSeq !== null || now < this.busyUntil) return null;
    this.seq += 1;
    this.pendingSeq = this.seq;
    this.busyUntil = now + (wanted.kind === 'move' ? STEP_MS : USE_MS);
    return { seq: this.seq, action: wanted };
  }

  /** Call on every ack from the server. */
  acked(seq: number): void {
    if (seq === this.pendingSeq) this.pendingSeq = null;
  }

  /** Forget anything in flight, e.g. after a reconnect. */
  reset(): void {
    this.pendingSeq = null;
    this.busyUntil = 0;
  }
}
