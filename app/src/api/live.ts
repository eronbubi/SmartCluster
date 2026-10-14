import type { Envelope } from './types';

export type LiveStatus = 'connecting' | 'open' | 'closed';

/** The live event channel as the app uses it; tests provide a fake. */
export interface Live {
  start(): void;
  stop(): void;
  status(): LiveStatus;
  /** Subscribes to topics; the returned function unsubscribes. */
  subscribe(topics: string[]): () => void;
  onEvent(listener: (event: Envelope) => void): () => void;
  onStatus(listener: (status: LiveStatus) => void): () => void;
}

/** The part of the browser WebSocket the client needs. */
export interface SocketLike {
  readonly readyState: number;
  onopen: ((ev: Event) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  onerror: ((ev: Event) => void) | null;
  send(data: string): void;
  close(): void;
}

const OPEN = 1;
const minBackoffMs = 1000;
const maxBackoffMs = 30_000;

let requestCounter = 0;

/**
 * WebSocket client for /api/v1/live. Keeps reference counts per topic,
 * resubscribes after reconnects and reconnects with exponential backoff.
 */
export class LiveClient implements Live {
  private socket: SocketLike | null = null;
  private current: LiveStatus = 'closed';
  private stopped = true;
  private backoff = minBackoffMs;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private readonly topics = new Map<string, number>();
  private readonly eventListeners = new Set<(event: Envelope) => void>();
  private readonly statusListeners = new Set<(status: LiveStatus) => void>();

  constructor(
    private readonly url: string,
    private readonly createSocket: (url: string) => SocketLike = (u) => new WebSocket(u),
  ) {}

  start(): void {
    if (!this.stopped) return;
    this.stopped = false;
    this.connect();
  }

  stop(): void {
    this.stopped = true;
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
    this.socket?.close();
    this.socket = null;
    this.setStatus('closed');
  }

  status(): LiveStatus {
    return this.current;
  }

  subscribe(topics: string[]): () => void {
    const added = topics.filter((t) => {
      const n = this.topics.get(t) ?? 0;
      this.topics.set(t, n + 1);
      return n === 0;
    });
    if (added.length > 0) this.send('sub', added);
    return () => {
      const removed = topics.filter((t) => {
        const n = (this.topics.get(t) ?? 1) - 1;
        if (n <= 0) this.topics.delete(t);
        else this.topics.set(t, n);
        return n <= 0;
      });
      if (removed.length > 0) this.send('unsub', removed);
    };
  }

  onEvent(listener: (event: Envelope) => void): () => void {
    this.eventListeners.add(listener);
    return () => this.eventListeners.delete(listener);
  }

  onStatus(listener: (status: LiveStatus) => void): () => void {
    this.statusListeners.add(listener);
    return () => this.statusListeners.delete(listener);
  }

  private connect(): void {
    this.setStatus('connecting');
    const socket = this.createSocket(this.url);
    this.socket = socket;
    socket.onopen = () => {
      this.backoff = minBackoffMs;
      this.setStatus('open');
      if (this.topics.size > 0) this.send('sub', [...this.topics.keys()]);
    };
    socket.onmessage = (ev) => {
      if (typeof ev.data !== 'string') return;
      let env: Envelope;
      try {
        env = JSON.parse(ev.data) as Envelope;
      } catch {
        return;
      }
      for (const l of this.eventListeners) l(env);
    };
    socket.onerror = () => {
      socket.close();
    };
    socket.onclose = () => {
      if (this.socket !== socket) return;
      this.socket = null;
      this.setStatus('closed');
      if (this.stopped) return;
      // Jitter keeps many open windows from reconnecting at the same moment.
      const delay = this.backoff / 2 + Math.random() * (this.backoff / 2);
      this.backoff = Math.min(this.backoff * 2, maxBackoffMs);
      this.timer = setTimeout(() => {
        this.timer = null;
        this.connect();
      }, delay);
    };
  }

  private send(type: 'sub' | 'unsub', topics: string[]): void {
    const socket = this.socket;
    if (socket?.readyState !== OPEN) return; // sent again on (re)connect
    requestCounter += 1;
    const frame: Envelope = { t: type, id: `r${requestCounter}`, ts: new Date().toISOString(), d: { topics } };
    socket.send(JSON.stringify(frame));
  }

  private setStatus(status: LiveStatus): void {
    if (status === this.current) return;
    this.current = status;
    for (const l of this.statusListeners) l(status);
  }
}

export const topics = {
  nodes: 'nodes',
  stats: 'stats',
  nodeStats: (id: string) => `node:${id}:stats`,
} as const;
