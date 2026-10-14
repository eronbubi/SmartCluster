// Package stats collects the periodic telemetry an agent sends (proto.Stats).
//
// A Reader returns raw, cumulative readings from the machine. The Collector
// turns them into rates and percentages, filters pseudo file systems and
// virtual interfaces, and never lets one failing reading break the sample.
package stats

import (
	"context"
	"fmt"
	"math"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// CPUTimes are cumulative seconds of one logical CPU.
type CPUTimes struct {
	Busy  float64
	Total float64
}

// Filesystem is one mounted file system.
type Filesystem struct {
	Mount  string
	Device string
	FSType string
	Total  uint64
	Used   uint64
}

// IOCounter holds cumulative bytes of one block device.
type IOCounter struct {
	ReadBytes  uint64
	WriteBytes uint64
}

// NetCounter holds cumulative bytes of one network interface.
type NetCounter struct {
	Iface   string
	Up      bool
	RxBytes uint64
	TxBytes uint64
}

// Reader returns raw readings. A nil pointer result means "not available on
// this machine" and is not an error.
type Reader interface {
	CPUTimes(ctx context.Context) ([]CPUTimes, error)
	Load1(ctx context.Context) (*float64, error)
	CPUFreqMHz(ctx context.Context) (*float64, error)
	Memory(ctx context.Context) (proto.MemStats, error)
	Filesystems(ctx context.Context) ([]Filesystem, error)
	DiskIO(ctx context.Context) (map[string]IOCounter, error)
	Network(ctx context.Context) ([]NetCounter, error)
	Temperatures(ctx context.Context) ([]proto.TempStat, error)
	GPUs(ctx context.Context) ([]proto.GPUStats, error)
	Battery(ctx context.Context) (*proto.Battery, error)
	Uptime(ctx context.Context) (uint64, error)
}

// Collector produces Stats samples. It is not safe for concurrent use.
type Collector struct {
	r    Reader
	prev *snapshot
}

type snapshot struct {
	at   time.Time
	cpu  []CPUTimes
	disk map[string]IOCounter
	net  map[string]NetCounter
}

// New returns a Collector reading from r.
func New(r Reader) *Collector {
	return &Collector{r: r}
}

// Sample reads all values at time now. Rates and CPU load are relative to the
// previous call; on the first call they are zero, so callers should prime the
// collector with one Sample before sending. Failed readings are returned in
// errs and leave their part of the sample empty or null.
func (c *Collector) Sample(ctx context.Context, now time.Time) (proto.Stats, []error) {
	var errs []error
	fail := func(what string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", what, err))
		}
	}

	s := proto.Stats{
		TS:    now.UTC(),
		Disks: []proto.DiskStats{},
		Net:   []proto.NetStats{},
		GPUs:  []proto.GPUStats{},
		Temps: []proto.TempStat{},
	}
	cur := &snapshot{at: now, disk: map[string]IOCounter{}, net: map[string]NetCounter{}}
	var elapsed float64
	if c.prev != nil {
		elapsed = now.Sub(c.prev.at).Seconds()
	}

	cpuTimes, err := c.r.CPUTimes(ctx)
	fail("cpu times", err)
	cur.cpu = cpuTimes
	s.CPU.PerCore = make([]float64, len(cpuTimes))
	if c.prev != nil {
		s.CPU.Pct, s.CPU.PerCore = cpuPercent(c.prev.cpu, cpuTimes)
	}
	s.CPU.Load1, err = c.r.Load1(ctx)
	fail("load average", err)
	s.CPU.FreqMHz, err = c.r.CPUFreqMHz(ctx)
	fail("cpu frequency", err)

	s.Mem, err = c.r.Memory(ctx)
	fail("memory", err)

	filesystems, err := c.r.Filesystems(ctx)
	fail("filesystems", err)
	io, err := c.r.DiskIO(ctx)
	fail("disk io", err)
	if io != nil {
		cur.disk = io
	}
	for _, f := range filterFilesystems(filesystems) {
		d := proto.DiskStats{Mount: f.Mount, Total: f.Total, Used: f.Used}
		key := deviceKey(f.Device)
		if after, ok := cur.disk[key]; ok && c.prev != nil {
			if before, ok := c.prev.disk[key]; ok {
				d.ReadBps = ptr(rate(before.ReadBytes, after.ReadBytes, elapsed))
				d.WriteBps = ptr(rate(before.WriteBytes, after.WriteBytes, elapsed))
			}
		}
		s.Disks = append(s.Disks, d)
	}

	ifaces, err := c.r.Network(ctx)
	fail("network", err)
	for _, n := range ifaces {
		if !KeepInterface(n.Iface) {
			continue
		}
		cur.net[n.Iface] = n
		ns := proto.NetStats{Iface: n.Iface, Up: n.Up}
		if c.prev != nil {
			if before, ok := c.prev.net[n.Iface]; ok {
				ns.RxBps = rate(before.RxBytes, n.RxBytes, elapsed)
				ns.TxBps = rate(before.TxBytes, n.TxBytes, elapsed)
			}
		}
		s.Net = append(s.Net, ns)
	}
	slices.SortFunc(s.Net, func(a, b proto.NetStats) int { return strings.Compare(a.Iface, b.Iface) })

	temps, err := c.r.Temperatures(ctx)
	fail("temperatures", err)
	s.Temps = validTemperatures(temps)
	s.CPU.TempC = pickCPUTemp(s.Temps)

	gpus, err := c.r.GPUs(ctx)
	fail("gpus", err)
	if gpus != nil {
		s.GPUs = gpus
	}

	s.Battery, err = c.r.Battery(ctx)
	fail("battery", err)

	s.UptimeS, err = c.r.Uptime(ctx)
	fail("uptime", err)

	c.prev = cur
	return s, errs
}

// cpuPercent returns overall and per-core busy percentages between two readings.
func cpuPercent(prev, cur []CPUTimes) (float64, []float64) {
	perCore := make([]float64, len(cur))
	var busySum, totalSum float64
	for i := range cur {
		if i >= len(prev) {
			continue
		}
		busy := cur[i].Busy - prev[i].Busy
		total := cur[i].Total - prev[i].Total
		if total <= 0 || busy < 0 {
			continue
		}
		perCore[i] = percent(busy, total)
		busySum += busy
		totalSum += total
	}
	if totalSum == 0 {
		return 0, perCore
	}
	return percent(busySum, totalSum), perCore
}

func percent(part, whole float64) float64 {
	p := part / whole * 100
	return math.Round(min(max(p, 0), 100)*10) / 10
}

// rate converts the growth of a cumulative counter into bytes per second.
// A counter that went backwards (reset, wrap, device replaced) yields 0.
func rate(before, now uint64, seconds float64) uint64 {
	if seconds <= 0 || now < before {
		return 0
	}
	return uint64(float64(now-before) / seconds)
}

// deviceKey maps a partition device to the key used by the IO counters:
// "/dev/sda1" -> "sda1" on Linux, "C:" stays "C:" on Windows.
func deviceKey(device string) string {
	return path.Base(device)
}

func ptr[T any](v T) *T { return &v }
