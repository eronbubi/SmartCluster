import { screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { makeNode, makeStats } from '../test/fixtures';
import { mockApi, renderApp, urlOf } from '../test/render';

const t420 = makeNode({ id: '0192f1c4-0000-7000-8000-000000000001', name: 't420' });
const elitebook = makeNode({
  id: '0192f1c4-0000-7000-8000-000000000002',
  name: 'elitebook',
  status: 'offline',
  last_seen_at: new Date(Date.now() - 3 * 60_000).toISOString(),
  latest: null,
});

function card(name: string) {
  const link = screen.getByRole('link', { name });
  const article = link.closest('article');
  if (article === null) throw new Error(`no card for ${name}`);
  return within(article);
}

describe('Dashboard', () => {
  it('shows every node with status and current values', async () => {
    mockApi({ '/api/v1/nodes': { nodes: [t420, elitebook] }, '/api/v1/cluster': { name: 'Home' } });
    renderApp('/');

    expect(await screen.findByText('1 of 2 online')).toBeInTheDocument();
    expect(card('t420').getByTestId('status')).toHaveTextContent('Online');
    expect(card('t420').getByText('12%')).toBeInTheDocument();
    expect(card('t420').getByText('2 GB / 8 GB')).toBeInTheDocument();
    expect(card('t420').getByText(/1 Mbit\/s/)).toBeInTheDocument();
    expect(card('elitebook').getByTestId('status')).toHaveTextContent('Offline');
    expect(card('elitebook').getByText('Last seen 3 minutes ago')).toBeInTheDocument();
    expect(await screen.findByText('Home')).toBeInTheDocument();
  });

  it('subscribes to live events and updates values without reloading', async () => {
    const fetchMock = mockApi({ '/api/v1/nodes': { nodes: [t420] }, '/api/v1/cluster': { name: 'Home' } });
    const { live } = renderApp('/');
    await screen.findByText('t420');
    expect([...live.subscribed]).toEqual(expect.arrayContaining(['nodes', 'stats']));
    const calls = fetchMock.mock.calls.length;

    live.emit('node.stats', { node_id: t420.id, stats: makeStats({ cpu: 73, onAC: false }) });
    expect(card('t420').getByText('73%')).toBeInTheDocument();
    expect(card('t420').getByText(/Running on battery/)).toBeInTheDocument();

    live.emit('node.status', { node_id: t420.id, status: 'offline', last_seen_at: new Date().toISOString() });
    // The query cache notifies asynchronously.
    await waitFor(() => {
      expect(card('t420').getByTestId('status')).toHaveTextContent('Offline');
    });
    expect(fetchMock.mock.calls.length).toBe(calls);
  });

  it('reloads the list when an unknown node reports in', async () => {
    const fetchMock = mockApi({ '/api/v1/nodes': { nodes: [t420] }, '/api/v1/cluster': { name: 'Home' } });
    const { live } = renderApp('/');
    await screen.findByText('t420');
    const before = fetchMock.mock.calls.filter(([u]) => urlOf(u).pathname.endsWith('/nodes')).length;
    live.emit('node.status', { node_id: '0192f1c4-0000-7000-8000-000000000099', status: 'online', last_seen_at: new Date().toISOString() });
    await waitFor(() => {
      expect(fetchMock.mock.calls.filter(([u]) => urlOf(u).pathname.endsWith('/nodes')).length).toBe(before + 1);
    });
  });

  it('explains how to add the first node', async () => {
    mockApi({ '/api/v1/nodes': { nodes: [] }, '/api/v1/cluster': { name: 'Home' } });
    renderApp('/');
    expect(await screen.findByText('No nodes yet')).toBeInTheDocument();
    expect(screen.getByText('smartcluster-control token create')).toBeInTheDocument();
  });

  it('shows load errors with a retry button', async () => {
    mockApi({
      '/api/v1/nodes': () => ({ status: 500, body: { error: { code: 'internal', message: 'internal error' } } }),
      '/api/v1/cluster': { name: 'Home' },
    });
    renderApp('/');
    expect(await screen.findByRole('alert')).toHaveTextContent('Loading failed: internal error');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });
});
