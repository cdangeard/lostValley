import type { ClockView } from './net/protocol';

/**
 * The valley clock as the client sees it: the server's last word plus the
 * real time elapsed since, so the hands move smoothly between updates.
 */
export class LocalClock {
  private view: ClockView | null = null;
  private at = 0;

  set(view: ClockView, now: number): void {
    this.view = view;
    this.at = now;
  }

  get current(): ClockView | null {
    return this.view;
  }

  /** Minutes since midnight now. It never runs past the next update by more than an hour. */
  minute(now: number): number {
    if (!this.view) return 12 * 60;
    const ahead = Math.min(60, (now - this.at) / Math.max(1, this.view.minuteMs));
    return (this.view.minute + ahead) % 1440;
  }
}

/** "06:40" — shown in ten-minute steps, like a farm clock. */
export function formatTime(minute: number): string {
  const m = Math.floor(minute / 10) * 10;
  const h = Math.floor(m / 60) % 24;
  return `${String(h).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
}

/**
 * How dark it is, from 0 (full day) to `NIGHT` (deep night). Dusk falls
 * from 18:00 to 21:00 and dawn rises from 5:00 to 7:00.
 */
export const NIGHT = 0.62;
export function darkness(minute: number): number {
  const h = minute / 60;
  if (h >= 7 && h < 18) return 0;
  if (h >= 18 && h < 21) return ((h - 18) / 3) * NIGHT;
  if (h >= 5 && h < 7) return (1 - (h - 5) / 2) * NIGHT;
  return NIGHT;
}

/** Which growth picture a crop shows. */
export type CropStage = 'seed' | 'sprout' | 'young' | 'ripe';
export function cropStage(growth: number, days: number): CropStage {
  if (growth >= days) return 'ripe';
  if (growth === 0) return 'seed';
  return growth < days / 2 ? 'sprout' : 'young';
}
