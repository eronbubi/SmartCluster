import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LiveClient, type SocketLike } from './live';
import type { Envelope } from './types';

class FakeSocket implements SocketLike {
  readyState = 0;
  sent: Envelope[] = [];
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  send(data: string): void {
    this.sent.push(JSON.parse(data) as Envelope);
  }
  close(): void {
    this.readyState = 3;
    this.onclose?.(new CloseEvent('close'));
  }
  open(): void {
    this.readyState = 1;
    this.onopen?.(new Event('open'));
  }
  receive(env: Envelope): void {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(env) }));
  }
}

function setup() {
  const sockets: FakeSocket[] = [];
  const client = new LiveClient('wss://control/api/v1/live', () => {
    const s = new FakeSocket();
    sockets.push(s);
    return s;
  });
  return { client, sockets, last: () => sockets[sockets.length - 1] as FakeSocket };
}

describe('LiveClient', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('subscribes on open and passes events on', () => {
    const { client, last } = setup();
    const events: Envelope[] = [];
    client.onEvent((e) => events.push(e));
    client.subscribe(['nodes', 'stats']);
    client.start();
    expect(client.status()).toBe('connecting');
    last().open();
    expect(client.status()).toBe('open');
    expect(last().sent).toHaveLength(1);
    expect(last().sent[0]).toMatchObject({ t: 'sub', d: { topics: ['nodes', 'stats'] } });

    last().receive({ t: 'node.status', ts: '2026-10-14T12:00:00Z', d: { node_id: 'n1', status: 'offline' } });
    expect(events.map((e) => e.t)).toEqual(['node.status']);
  });

  it('counts references per topic', () => {
    const { client, last } = setup();
    client.start();
    last().open();
    const a = client.subscribe(['stats']);
    const b = client.subscribe(['stats']);
    expect(last().sent.filter((f) => f.t === 'sub')).toHaveLength(1);
    a();
    expect(last().sent.filter((f) => f.t === 'unsub')).toHaveLength(0);
    b();
    expect(last().sent.at(-1)).toMatchObject({ t: 'unsub', d: { topics: ['stats'] } });
  });

  it('reconnects with backoff and subscribes again', () => {
    const { client, sockets, last } = setup();
    const statuses: string[] = [];
    client.onStatus((s) => statuses.push(s));
    client.subscribe(['nodes']);
    client.start();
    last().open();
    last().close();
    expect(client.status()).toBe('closed');
    expect(sockets).toHaveLength(1);

    vi.advanceTimersByTime(1000);
    expect(sockets).toHaveLength(2);
    last().open();
    expect(last().sent[0]).toMatchObject({ t: 'sub', d: { topics: ['nodes'] } });
    expect(statuses).toEqual(['connecting', 'open', 'closed', 'connecting', 'open']);
  });

  it('stays closed after stop', () => {
    const { client, sockets, last } = setup();
    client.start();
    last().open();
    client.stop();
    vi.advanceTimersByTime(60_000);
    expect(sockets).toHaveLength(1);
    expect(client.status()).toBe('closed');
  });

  it('ignores frames that are not JSON', () => {
    const { client, last } = setup();
    const listener = vi.fn();
    client.onEvent(listener);
    client.start();
    last().open();
    last().onmessage?.(new MessageEvent('message', { data: 'not json' }));
    expect(listener).not.toHaveBeenCalled();
  });
});
