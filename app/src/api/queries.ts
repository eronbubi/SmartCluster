import { useQuery } from '@tanstack/react-query';
import { ApiError, api } from './client';

export const ranges = {
  '1h': { ms: 3_600_000, refetchMs: 30_000 },
  '24h': { ms: 86_400_000, refetchMs: 300_000 },
  '7d': { ms: 7 * 86_400_000, refetchMs: 300_000 },
  '90d': { ms: 90 * 86_400_000, refetchMs: 900_000 },
} as const;

export type RangeKey = keyof typeof ranges;

export const queryKeys = {
  cluster: ['cluster'] as const,
  nodes: ['nodes'] as const,
  node: (id: string) => ['node', id] as const,
  stats: (id: string, range: RangeKey) => ['stats', id, range] as const,
};

/** Retries network trouble, but not answers that will not change (404, 400). */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status >= 400 && error.status < 500) return false;
  return failureCount < 3;
}

export function useCluster() {
  return useQuery({ queryKey: queryKeys.cluster, queryFn: ({ signal }) => api.cluster(signal), staleTime: Infinity });
}

export function useNodes() {
  return useQuery({ queryKey: queryKeys.nodes, queryFn: ({ signal }) => api.nodes(signal) });
}

export function useNode(id: string) {
  return useQuery({ queryKey: queryKeys.node(id), queryFn: ({ signal }) => api.node(id, signal) });
}

export function useNodeStats(id: string, range: RangeKey) {
  const r = ranges[range];
  return useQuery({
    queryKey: queryKeys.stats(id, range),
    queryFn: ({ signal }) => {
      const to = new Date();
      return api.nodeStats(id, new Date(to.getTime() - r.ms), to, signal);
    },
    refetchInterval: r.refetchMs,
  });
}
