import { useQueryClient } from '@tanstack/react-query';
import { createContext, useContext, useEffect, useState, useSyncExternalStore, type ReactNode } from 'react';
import { topics, type Live, type LiveStatus } from '../api/live';
import { queryKeys } from '../api/queries';
import type { Node, NodeStatsEvent, NodeStatusEvent } from '../api/types';
import { LiveStatsStore, type LiveNode } from '../state/liveStats';

interface LiveContextValue {
  live: Live;
  stats: LiveStatsStore;
}

const LiveContext = createContext<LiveContextValue | null>(null);

function useLiveContext(): LiveContextValue {
  const ctx = useContext(LiveContext);
  if (ctx === null) throw new Error('LiveProvider missing');
  return ctx;
}

function isStatusEvent(d: unknown): d is NodeStatusEvent {
  return typeof d === 'object' && d !== null && 'node_id' in d && 'status' in d;
}

function isStatsEvent(d: unknown): d is NodeStatsEvent {
  return typeof d === 'object' && d !== null && 'node_id' in d && 'stats' in d;
}

/**
 * Connects the live channel to the app state: node.stats go into the live
 * store, node.status updates the cached node data, and after a reconnect
 * everything is refetched because events may have been missed.
 */
export function LiveProvider({ live, children }: { live: Live; children: ReactNode }) {
  const queryClient = useQueryClient();
  const [stats] = useState(() => new LiveStatsStore());

  useEffect(() => {
    let wasOpen = false;
    const offStatus = live.onStatus((s) => {
      if (s === 'open' && wasOpen) void queryClient.invalidateQueries();
      if (s === 'open') wasOpen = true;
    });
    const offEvent = live.onEvent((env) => {
      if (env.t === 'node.stats' && isStatsEvent(env.d)) {
        stats.apply(env.d);
      } else if (env.t === 'node.status' && isStatusEvent(env.d)) {
        const ev = env.d;
        const update = (n: Node): Node => ({ ...n, status: ev.status, last_seen_at: ev.last_seen_at });
        queryClient.setQueryData<Node[]>(queryKeys.nodes, (nodes) => {
          if (nodes === undefined) return undefined;
          if (!nodes.some((n) => n.id === ev.node_id)) {
            void queryClient.invalidateQueries({ queryKey: queryKeys.nodes }); // a new node joined
            return nodes;
          }
          return nodes.map((n) => (n.id === ev.node_id ? update(n) : n));
        });
        queryClient.setQueryData<Node>(queryKeys.node(ev.node_id), (n) => (n === undefined ? undefined : update(n)));
      }
    });
    const unsubscribe = live.subscribe([topics.nodes, topics.stats]);
    live.start();
    return () => {
      unsubscribe();
      offEvent();
      offStatus();
      live.stop();
    };
  }, [live, queryClient, stats]);

  return <LiveContext.Provider value={{ live, stats }}>{children}</LiveContext.Provider>;
}

export function useLiveStatus(): LiveStatus {
  const { live } = useLiveContext();
  return useSyncExternalStore(
    (cb) => live.onStatus(cb),
    () => live.status(),
  );
}

/** Live stats of all nodes that sent any since the app started. */
export function useLiveSnapshot(): ReadonlyMap<string, LiveNode> {
  const { stats } = useLiveContext();
  return useSyncExternalStore(
    (cb) => stats.subscribe(cb),
    () => stats.snapshot(),
  );
}

export function useLiveNode(nodeId: string): LiveNode | undefined {
  return useLiveSnapshot().get(nodeId);
}
