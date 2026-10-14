import type { NodeStatsEvent, Stats } from '../api/types';
import { usedPercent } from '../lib/format';

/** Number of live samples kept per node for sparklines (5 minutes at 5 s). */
export const historyLength = 60;

export interface LiveNode {
  latest: Stats;
  /** CPU load in percent, oldest first. */
  cpu: number[];
  /** Memory usage in percent, oldest first. */
  mem: number[];
}

/**
 * In-memory store of the stats that arrived over the live channel since the
 * app started. React reads it with useSyncExternalStore.
 */
export class LiveStatsStore {
  private nodes = new Map<string, LiveNode>();
  private readonly listeners = new Set<() => void>();

  apply(event: NodeStatsEvent): void {
    const prev = this.nodes.get(event.node_id);
    const append = (series: number[] | undefined, value: number) => [...(series ?? []), value].slice(-historyLength);
    const next: LiveNode = {
      latest: event.stats,
      cpu: append(prev?.cpu, event.stats.cpu.pct),
      mem: append(prev?.mem, usedPercent(event.stats.mem.used, event.stats.mem.total) ?? 0),
    };
    // A new Map per change keeps snapshots immutable for React.
    this.nodes = new Map(this.nodes).set(event.node_id, next);
    for (const l of this.listeners) l();
  }

  get(nodeId: string): LiveNode | undefined {
    return this.nodes.get(nodeId);
  }

  snapshot(): ReadonlyMap<string, LiveNode> {
    return this.nodes;
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }
}
