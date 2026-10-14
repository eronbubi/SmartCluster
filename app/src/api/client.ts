import type { Cluster, ErrorBody, Node, NodeList, StatsResponse } from './types';

/** An error answer of the control center (see the Error schema). */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

/**
 * Base URL of the control center. Empty in the web app (same origin); the
 * Electron shell provides it through the preload script.
 */
export function controlBase(): string {
  return window.smartcluster?.controlUrl ?? '';
}

/** WebSocket URL of the live events. */
export function liveUrl(): string {
  const base = controlBase();
  if (base !== '') return `${base.replace(/^http/, 'ws')}/api/v1/live`;
  const scheme = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${scheme}://${window.location.host}/api/v1/live`;
}

function isErrorBody(v: unknown): v is ErrorBody {
  if (typeof v !== 'object' || v === null || !('error' in v)) return false;
  const e = v.error;
  return typeof e === 'object' && e !== null && 'code' in e && 'message' in e;
}

export async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${controlBase()}${path}`, { signal: signal ?? null, headers: { Accept: 'application/json' } });
  if (!res.ok) {
    let code = `http_${res.status}`;
    let message = res.statusText;
    try {
      const body: unknown = await res.json();
      if (isErrorBody(body)) {
        code = body.error.code;
        message = body.error.message;
      }
    } catch {
      // not JSON: keep the HTTP status
    }
    throw new ApiError(res.status, code, message);
  }
  return (await res.json()) as T;
}

export const api = {
  cluster: (signal?: AbortSignal) => getJSON<Cluster>('/api/v1/cluster', signal),
  nodes: async (signal?: AbortSignal) => (await getJSON<NodeList>('/api/v1/nodes', signal)).nodes,
  node: (id: string, signal?: AbortSignal) => getJSON<Node>(`/api/v1/nodes/${encodeURIComponent(id)}`, signal),
  nodeStats: (id: string, from: Date, to: Date, signal?: AbortSignal) => {
    const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
    return getJSON<StatsResponse>(`/api/v1/nodes/${encodeURIComponent(id)}/stats?${q.toString()}`, signal);
  },
};
