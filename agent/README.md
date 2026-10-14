# agent

The SmartCluster node agent. One Go binary per platform (Windows 10/11 x64,
Debian/Ubuntu amd64), later running as a system service.

## Packages

| Package | Purpose |
|---|---|
| `cmd/smartcluster-agent` | CLI entry point |
| `internal/stats` | Telemetry: CPU, RAM, disks, network, GPU (nvidia-smi), temperatures, battery, uptime |
| `internal/sysinfo` | Inventory for hello: OS, vendor/model/BIOS, CPU, RAM, GPUs, NICs |
| `internal/state` | Node identity on disk (`agent.json`, `node.pem`, `ca.crt`), protected directory |
| `internal/session` | Connection to the control center: hello, stats every 5 s, signed commands, reconnect with backoff |
| `internal/svc` | Windows service / systemd unit |
| `internal/discover` | mDNS search for control centers |

`stats.Collector` turns cumulative counters from a `stats.Reader` into rates.
The real reader (`stats.System`) uses gopsutil plus small platform files;
parsing of sysfs and nvidia-smi output is platform-independent and unit tested
on every OS.

Platform limits (see ARCHITECTURE.md 1.9):
- Windows: no load average, no current CPU clock, CPU temperature only where
  the ACPI thermal zone is exposed. Missing values are `null`, never `0`.
- Linux: temperatures from hwmon, frequency from cpufreq (absent in many VMs).

## Commands

```bash
smartcluster-agent join [--control host[:port]] CODE   # as admin/root
smartcluster-agent service install                      # as admin/root
go run ./agent/cmd/smartcluster-agent inventory
go run ./agent/cmd/smartcluster-agent stats -n 3 -interval 5s
```

## Tests

```bash
go test ./agent/...                      # unit tests only, starts nothing
SC_TEST_SERVERS=1 go test ./agent/...    # CI / test machine: session tests against a fake control center
GOOS=linux go vet ./agent/...            # compile check of the Linux files
```

mDNS discovery (`internal/discover`) has an opt-in round trip test
(`SC_TEST_SERVERS=1 SC_TEST_MDNS=1`). It failed on the Windows development PC
(own announcement not seen) and still has to be verified on a real network.
