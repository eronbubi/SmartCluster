import { QueryClient } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { act } from 'react';
import { createMemoryRouter } from 'react-router';
import { vi } from 'vitest';
import { App, routes } from '../App';
import type { Live, LiveStatus } from '../api/live';
import type { Envelope } from '../api/types';

/** A live channel driven by the test instead of a server. */
export class FakeLive implements Live {
  current: LiveStatus = 'closed';
  subscribed = new Set<string>();
  private events = new Set<(e: Envelope) => void>();
  private statuses = new Set<(s: LiveStatus) => void>();

  start(): void {
    this.setStatus('open');
  }
  stop(): void {
    this.setStatus('closed');
  }
  status(): LiveStatus {
    return this.current;
  }
  subscribe(topics: string[]): () => void {
    for (const t of topics) this.subscribed.add(t);
    return () => {
      for (const t of topics) this.subscribed.delete(t);
    };
  }
  onEvent(l: (e: Envelope) => void): () => void {
    this.events.add(l);
    return () => this.events.delete(l);
  }
  onStatus(l: (s: LiveStatus) => void): () => void {
    this.statuses.add(l);
    return () => this.statuses.delete(l);
  }
  emit(t: Envelope['t'], d: unknown): void {
    act(() => {
      for (const l of this.events) l({ t, ts: new Date().toISOString(), d });
    });
  }
  setStatus(s: LiveStatus): void {
    this.current = s;
    for (const l of this.statuses) l(s);
  }
}

export type Routes = Record<string, unknown>;

/** The URL a fetch call was made with. */
export function urlOf(input: RequestInfo | URL): URL {
  return new URL(typeof input === 'string' ? input : input instanceof URL ? input.href : input.url, 'http://control');
}

/**
 * Replaces fetch with a table of API answers keyed by path (without query).
 * A function value can inspect the URL and choose status and body.
 */
export function mockApi(table: Routes) {
  const fetchMock = vi.fn((input: RequestInfo | URL) => {
    const url = urlOf(input);
    const entry = table[url.pathname];
    let status = 200;
    let body: unknown = entry;
    if (entry === undefined) {
      status = 404;
      body = { error: { code: 'not_found', message: 'no such endpoint' } };
    } else if (typeof entry === 'function') {
      ({ status, body } = (entry as (u: URL) => { status: number; body: unknown })(url));
    }
    return Promise.resolve(new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }));
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

export function renderApp(path: string, live = new FakeLive()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  const utils = render(<App live={live} queryClient={queryClient} router={router} />);
  return { ...utils, live, queryClient, router };
}
