import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import type { ChartSeries } from '../components/StatsChart';
import { makeNode, makePoint, makeStats } from '../test/fixtures';
import { mockApi, renderApp, urlOf } from '../test/render';

// uPlot needs a canvas, which jsdom lacks; the chart's own logic is in uPlot.
vi.mock('../components/StatsChart', () => ({
  StatsChart: ({ title, series }: { title: string; series: ChartSeries[] }) => (
    <figure data-testid="chart">
      {title}: {series.map((s) => s.values.length).join(',')}
    </figure>
  ),
}));

const node = makeNode({ id: '0192f1c4-0000-7000-8000-000000000001', name: 't420' });
const points = [makePoint('2026-10-14T11:00:00Z'), makePoint('2026-10-14T11:00:05Z')];

describe('NodeDetail', () => {
  it('shows system, hardware, network and history', async () => {
    mockApi({
      [`/api/v1/nodes/${node.id}`]: node,
      [`/api/v1/nodes/${node.id}/stats`]: { resolution: 'raw', from: '', to: '', points },
      '/api/v1/cluster': { name: 'Home' },
    });
    renderApp(`/nodes/${node.id}`);

    expect(await screen.findByRole('heading', { name: 't420' })).toBeInTheDocument();
    expect(screen.getByText('Debian GNU/Linux 13 (trixie) 13')).toBeInTheDocument();
    expect(screen.getByText('4236AB1 · ThinkPad T420')).toBeInTheDocument();
    expect(screen.getByText('Intel Core i5-2520M (2 cores, 4 threads)')).toBeInTheDocument();
    expect(screen.getByText('192.168.1.20/24')).toBeInTheDocument();
    expect(screen.getByText('not available')).toBeInTheDocument(); // no CPU temperature

    const charts = await screen.findAllByTestId('chart');
    expect(charts.map((c) => c.textContent)).toEqual(['CPU load: 2', 'Memory: 2', 'Network: 2,2', 'Fullest disk: 2']);
  });

  it('adds temperature and GPU charts only when measured', async () => {
    mockApi({
      [`/api/v1/nodes/${node.id}`]: node,
      [`/api/v1/nodes/${node.id}/stats`]: {
        resolution: 'raw', from: '', to: '',
        points: [makePoint('2026-10-14T11:00:00Z', { cpu_temp_c: 55, gpu_util_pct: 3 })],
      },
      '/api/v1/cluster': { name: 'Home' },
    });
    renderApp(`/nodes/${node.id}`);
    await waitFor(() => {
      expect(screen.getAllByTestId('chart')).toHaveLength(6);
    });
  });

  it('loads a longer range on request', async () => {
    const fetchMock = mockApi({
      [`/api/v1/nodes/${node.id}`]: node,
      [`/api/v1/nodes/${node.id}/stats`]: { resolution: 'raw', from: '', to: '', points },
      '/api/v1/cluster': { name: 'Home' },
    });
    renderApp(`/nodes/${node.id}`);
    await screen.findAllByTestId('chart');

    await userEvent.click(screen.getByRole('button', { name: '7 days' }));
    await waitFor(() => {
      const statsCalls = fetchMock.mock.calls.map(([u]) => urlOf(u)).filter((u) => u.pathname.endsWith('/stats'));
      const last = statsCalls.at(-1);
      expect(last).toBeDefined();
      const from = Date.parse(last?.searchParams.get('from') ?? '');
      const to = Date.parse(last?.searchParams.get('to') ?? '');
      expect(Math.round((to - from) / 86_400_000)).toBe(7);
    });
    expect(screen.getByRole('button', { name: '7 days' })).toHaveAttribute('aria-pressed', 'true');
  });

  it('uses live values for the current state', async () => {
    mockApi({
      [`/api/v1/nodes/${node.id}`]: node,
      [`/api/v1/nodes/${node.id}/stats`]: { resolution: 'raw', from: '', to: '', points: [] },
      '/api/v1/cluster': { name: 'Home' },
    });
    const { live } = renderApp(`/nodes/${node.id}`);
    expect(await screen.findByText('There are no readings for this period.')).toBeInTheDocument();
    live.emit('node.stats', { node_id: node.id, stats: makeStats({ cpu: 88, temp: 71 }) });
    expect(screen.getByText('88%')).toBeInTheDocument();
    expect(screen.getByText('71 °C')).toBeInTheDocument();
  });

  it('tells when the node does not exist', async () => {
    mockApi({ '/api/v1/cluster': { name: 'Home' } });
    renderApp('/nodes/0192f1c4-0000-7000-8000-00000000dead');
    expect(await screen.findByText('This node does not exist (anymore).')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to overview' })).toHaveAttribute('href', '/');
  });
});
