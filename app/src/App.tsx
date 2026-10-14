import { QueryClientProvider, type QueryClient } from '@tanstack/react-query';
import { RouterProvider, type RouteObject, type createBrowserRouter } from 'react-router';
import type { Live } from './api/live';
import { Layout } from './components/Layout';
import { LiveProvider } from './live/LiveContext';
import { Dashboard } from './pages/Dashboard';
import { NodeDetail } from './pages/NodeDetail';
import { NotFound } from './pages/NotFound';

export const routes: RouteObject[] = [
  {
    element: <Layout />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'nodes/:id', element: <NodeDetail /> },
      { path: '*', element: <NotFound /> },
    ],
  },
];

type Router = ReturnType<typeof createBrowserRouter>;

export function App({ live, queryClient, router }: { live: Live; queryClient: QueryClient; router: Router }) {
  return (
    <QueryClientProvider client={queryClient}>
      <LiveProvider live={live}>
        <RouterProvider router={router} />
      </LiveProvider>
    </QueryClientProvider>
  );
}
