import { describe, expect, it } from 'vitest';
import { LocalClock, NIGHT, cropStage, darkness, formatTime } from './clock';
import type { ClockView } from './net/protocol';

const view: ClockView = {
  day: 3, minute: 6 * 60, year: 1, season: 'Printemps', seasonDay: 3, weather: 'soleil', minuteMs: 750,
};

describe('LocalClock', () => {
  it('runs between server updates', () => {
    const c = new LocalClock();
    c.set(view, 1000);
    expect(c.minute(1000)).toBe(360);
    expect(c.minute(1000 + 750 * 10)).toBe(370);
  });

  it('does not run away if updates stop', () => {
    const c = new LocalClock();
    c.set(view, 0);
    expect(c.minute(750 * 1000)).toBe(420);
  });

  it('wraps at midnight', () => {
    const c = new LocalClock();
    c.set({ ...view, minute: 1435 }, 0);
    expect(c.minute(750 * 10)).toBe(5);
  });
});

describe('formatTime', () => {
  it('shows ten-minute steps', () => {
    expect(formatTime(6 * 60 + 47)).toBe('06:40');
    expect(formatTime(23 * 60 + 59)).toBe('23:50');
    expect(formatTime(0)).toBe('00:00');
  });
});

describe('darkness', () => {
  it('is light by day, dark by night, in between at dusk', () => {
    expect(darkness(12 * 60)).toBe(0);
    expect(darkness(23 * 60)).toBe(NIGHT);
    expect(darkness(1 * 60)).toBe(NIGHT);
    const dusk = darkness(19.5 * 60);
    expect(dusk).toBeGreaterThan(0);
    expect(dusk).toBeLessThan(NIGHT);
    expect(darkness(6 * 60)).toBeCloseTo(NIGHT / 2);
  });
});

describe('cropStage', () => {
  it('follows growth', () => {
    expect(cropStage(0, 4)).toBe('seed');
    expect(cropStage(1, 4)).toBe('sprout');
    expect(cropStage(3, 4)).toBe('young');
    expect(cropStage(4, 4)).toBe('ripe');
  });
});
