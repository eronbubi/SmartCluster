# SmartCluster

Turns old laptops and PCs into a self-hosted server cluster.

**Proprietary and confidential. All rights reserved.** No use, copying,
modification or distribution without written permission; see [LICENSE](LICENSE).

- Architecture: [ARCHITECTURE.md](ARCHITECTURE.md)
- Decisions: [docs/adr](docs/adr/README.md)

## Layout

| Path | Content |
|---|---|
| `pkg/` | Go module shared by agent and control center: protocol, signing, ids |
| `agent/` | Go module: node agent (system service) |
| `control/` | Go module: control center backend |
| `app/` | Control app (React + Electron) |
| `roles/` | Role templates |
| `tools/` | Vendor tools and spikes, never shipped |
| `docs/` | ADRs, API and protocol docs |

The Go modules are tied together by `go.work`.

## Development

Requirements: Go 1.27.

```bash
go test ./pkg/... ./agent/... ./control/...   # unit tests, start no servers
cd app && npm ci && npm test                  # app tests (jsdom), no server
```

Tests that start servers (Postgres, TLS listeners, the control center) only
run with `SC_TEST_SERVERS=1`: in CI and on test machines, not on a developer PC.

## Status

Milestone a is implemented: agent stats, enrollment, control center, dashboard
(web and Electron). Open: the acceptance test on real machines (two nodes live
in the dashboard, offline within 60 s after pulling the network cable) and the
Windows Docker/WSL spike (ADR-0002), both on test laptops.
