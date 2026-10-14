# SmartCluster

Proprietary, closed source. Turns old laptops and PCs into a self-hosted server cluster.

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
```

Tests that start servers (Postgres, TLS listeners, the control center) only
run with `SC_TEST_SERVERS=1`: in CI and on test machines, not on a developer PC.

## Status

Milestone a (agent stats, enrollment, control center, dashboard) is in progress.
