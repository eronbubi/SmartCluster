import { QueryClient } from '@tanstack/react-query';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createBrowserRouter } from 'react-router';
import { App, routes } from './App';
import { liveUrl } from './api/client';
import { LiveClient } from './api/live';
import { shouldRetry } from './api/queries';
import i18n from './i18n';
import { applyTheme, storedTheme } from './lib/theme';
import './theme/app.css';

applyTheme(storedTheme());
document.documentElement.lang = i18n.language;

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: shouldRetry, refetchOnWindowFocus: false } },
});
const live = new LiveClient(liveUrl());
const router = createBrowserRouter(routes);

const root = document.getElementById('root');
if (root === null) throw new Error('#root is missing in index.html');
createRoot(root).render(
  <StrictMode>
    <App live={live} queryClient={queryClient} router={router} />
  </StrictMode>,
);
