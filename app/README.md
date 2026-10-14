# app

The SmartCluster control app: one React/TypeScript code base for the web UI
(served by the control center) and the Electron shell (ADR-0020).

Milestone a: dashboard with all nodes and live values, node detail with
system/hardware/network info, current readings and history charts
(1 h, 24 h, 7 days, 90 days). German and English, light/dark/system theme.
No login before milestone b.

## Layout

| Path | Content |
|---|---|
| `src/api/schema.d.ts` | Generated from `docs/api/openapi.yaml` (`npm run gen:api`), never edited by hand |
| `src/api/` | REST client, TanStack Query hooks, live WebSocket client with resubscribe and backoff |
| `src/live/` | Connects live events to the query cache and the live stats store |
| `src/pages/`, `src/components/` | Dashboard, node detail, cards, meters, charts (uPlot, loaded only on the detail page) |
| `src/i18n/` | German and English texts; `de.ts` defines the keys, `en.ts` must match its type |
| `electron/` | Electron shell: `app://` protocol for the bundled renderer, CA pinning, navigation lock, preload |

## Electron shell

The renderer is served from `dist/` through the custom `app://smartcluster`
protocol, not through a network server. It talks only to the configured
control center, whose TLS certificate must be signed by the pinned cluster
CA, be valid, carry the server usage and cover the host.

Until the setup wizard exists (milestone b) the shell is started with:

```bash
npm run build && npm run build:electron
npx electron . --control=https://<address>:8443 --ca=<path to ca.crt>
```

## Development

No dev server is used on developer PCs. Checks and builds:

```bash
npm ci                       # ELECTRON_SKIP_BINARY_DOWNLOAD=1 skips the Electron runtime
npm run gen:api              # after changing docs/api/openapi.yaml
npm run typecheck
npm run lint
npm test                     # Vitest + Testing Library in jsdom, no server
npm run build                # dist/
npm run embed                # copies dist/ into control/internal/web/dist for go:embed
```

Deviations from ARCHITECTURE.md:
- npm instead of pnpm (no extra tool).
- TypeScript 5.9 instead of 7: typescript-eslint and openapi-typescript do not support 7 yet.
- Radix UI is not used yet; milestone a has no dialogs or menus.
