import { describe, expect, it } from 'vitest';
import { ActionPacer, GATHER_MS, STEP_MS } from './ActionPacer';

describe('ActionPacer', () => {
  it('sends nothing when nothing is wanted', () => {
    expect(new ActionPacer().next(0, null)).toBeNull();
  });

  it('waits for the ack before sending again', () => {
    const p = new ActionPacer();
    const first = p.next(0, { kind: 'move', dir: 'up' });
    expect(first?.seq).toBe(1);
    expect(p.next(STEP_MS * 10, { kind: 'move', dir: 'up' })).toBeNull();
    p.acked(1);
    expect(p.next(STEP_MS * 10, { kind: 'move', dir: 'up' })?.seq).toBe(2);
  });

  it('waits for the animation even when the ack is fast', () => {
    const p = new ActionPacer();
    p.next(0, { kind: 'move', dir: 'left' });
    p.acked(1);
    expect(p.next(STEP_MS - 1, { kind: 'move', dir: 'left' })).toBeNull();
    expect(p.next(STEP_MS, { kind: 'move', dir: 'left' })).not.toBeNull();
  });

  it('gives gathering a longer swing', () => {
    const p = new ActionPacer();
    p.next(0, { kind: 'gather' });
    p.acked(1);
    expect(p.next(STEP_MS, { kind: 'gather' })).toBeNull();
    expect(p.next(GATHER_MS, { kind: 'gather' })).not.toBeNull();
  });

  it('ignores stale acks and recovers after reset', () => {
    const p = new ActionPacer();
    p.next(0, { kind: 'gather' });
    p.acked(99);
    expect(p.next(1000, { kind: 'gather' })).toBeNull();
    p.reset();
    expect(p.next(1000, { kind: 'gather' })).not.toBeNull();
  });
});
