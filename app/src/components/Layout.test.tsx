import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { mockApi, renderApp } from '../test/render';
import { sparklinePoints } from './Sparkline';

describe('Layout', () => {
  it('switches language and theme', async () => {
    mockApi({ '/api/v1/nodes': { nodes: [] }, '/api/v1/cluster': { name: 'Home' } });
    renderApp('/');
    expect(await screen.findByText('No nodes yet')).toBeInTheDocument();

    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Language' }), 'de');
    expect(await screen.findByText('Noch keine Nodes')).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('de');

    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Darstellung' }), 'dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Darstellung' }), 'system');
    expect(document.documentElement.dataset.theme).toBeUndefined();
  });

  it('shows the state of the live connection', async () => {
    mockApi({ '/api/v1/nodes': { nodes: [] }, '/api/v1/cluster': { name: 'Home' } });
    const { live } = renderApp('/');
    expect(await screen.findByRole('status')).toHaveTextContent('Live');
    live.setStatus('closed');
    expect(await screen.findByRole('status')).toHaveTextContent('Disconnected, reconnecting');
  });
});

describe('sparklinePoints', () => {
  it('maps 0..100 onto the height and clamps outliers', () => {
    expect(sparklinePoints([0, 100, 150], 100, 20)).toBe('0.0,20.0 50.0,0.0 100.0,0.0');
    expect(sparklinePoints([42], 100, 20)).toBe('');
  });
});
