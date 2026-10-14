import { describe, expect, it, vi } from 'vitest';
import { makeStats } from '../test/fixtures';
import { historyLength, LiveStatsStore } from './liveStats';

describe('LiveStatsStore', () => {
  it('keeps the latest sample and a bounded history', () => {
    const store = new LiveStatsStore();
    const listener = vi.fn();
    store.subscribe(listener);
    for (let i = 0; i < historyLength + 5; i++) {
      store.apply({ node_id: 'n1', stats: makeStats({ cpu: i, memUsed: 4, memTotal: 8 }) });
    }
    const n = store.get('n1');
    expect(n?.latest.cpu.pct).toBe(historyLength + 4);
    expect(n?.cpu).toHaveLength(historyLength);
    expect(n?.cpu[0]).toBe(5);
    expect(n?.mem.at(-1)).toBe(50);
    expect(listener).toHaveBeenCalledTimes(historyLength + 5);
  });

  it('returns a new snapshot on every change', () => {
    const store = new LiveStatsStore();
    const before = store.snapshot();
    store.apply({ node_id: 'n1', stats: makeStats({ cpu: 1 }) });
    expect(store.snapshot()).not.toBe(before);
    expect(store.get('n2')).toBeUndefined();
  });
});
