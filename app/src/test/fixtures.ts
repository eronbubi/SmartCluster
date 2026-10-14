import type { Node, Point, Stats } from '../api/types';

export function makeStats(o: { cpu?: number; memUsed?: number; memTotal?: number; temp?: number | null; onAC?: boolean | null } = {}): Stats {
  return {
    ts: '2026-10-14T12:00:00Z',
    cpu: { pct: o.cpu ?? 12, per_core: [10, 14], load1: null, freq_mhz: null, temp_c: o.temp ?? null },
    mem: { total: o.memTotal ?? 8 * 1024 ** 3, used: o.memUsed ?? 2 * 1024 ** 3, swap_total: 0, swap_used: 0 },
    disks: [{ mount: '/', total: 100 * 1024 ** 3, used: 40 * 1024 ** 3, read_bps: 1024, write_bps: null }],
    net: [{ iface: 'enp0s25', up: true, rx_bps: 125_000, tx_bps: 12_500 }],
    gpus: [],
    temps: [],
    battery: o.onAC == null ? null : { pct: 54, on_ac: o.onAC, health_pct: 80 },
    uptime_s: 30_059,
  };
}

export function makeNode(o: Partial<Node> & { id: string; name: string }): Node {
  return {
    hostname: o.name,
    status: 'online',
    os: { platform: 'linux', name: 'Debian GNU/Linux 13 (trixie)', version: '13', kernel: '6.12.0', arch: 'amd64', hostname: o.name },
    hardware: {
      vendor: 'LENOVO',
      model: '4236AB1',
      product_version: 'ThinkPad T420',
      bios_version: '83ET82WW',
      cpu: { model: 'Intel Core i5-2520M', cores: 2, threads: 4 },
      ram_total: 8 * 1024 ** 3,
      gpus: [{ name: 'i915 (8086:0126)', mem_total: null }],
      nics: [{ name: 'enp0s25', mac: 'f0:de:f1:00:00:01', addrs: ['192.168.1.20/24'] }],
    },
    agent_version: '0.1.0',
    ip_addresses: ['192.168.1.20/24'],
    mac_addresses: ['f0:de:f1:00:00:01'],
    cert_not_after: '2027-10-14T12:00:00Z',
    created_at: '2026-10-14T10:00:00Z',
    last_seen_at: '2026-10-14T11:59:55Z',
    latest: makeStats(),
    ...o,
  };
}

export function makePoint(ts: string, o: Partial<Point> = {}): Point {
  return {
    ts,
    cpu_pct: 20,
    mem_used: 2 * 1024 ** 3,
    mem_total: 8 * 1024 ** 3,
    disk_used_pct: 40,
    net_rx_bps: 1000,
    net_tx_bps: 100,
    cpu_temp_c: null,
    gpu_util_pct: null,
    ...o,
  };
}
