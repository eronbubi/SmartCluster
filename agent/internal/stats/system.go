package stats

import (
	"context"
	"errors"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/sensors"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// System is the Reader for the machine the agent runs on.
type System struct{}

var _ Reader = System{}

func (System) CPUTimes(ctx context.Context) ([]CPUTimes, error) {
	times, err := cpu.TimesWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]CPUTimes, len(times))
	for i, t := range times {
		// Guest time is already part of User on Linux, so it is not added again.
		total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
		out[i] = CPUTimes{Busy: total - t.Idle - t.Iowait, Total: total}
	}
	return out, nil
}

func (System) Load1(ctx context.Context) (*float64, error) {
	return load1(ctx)
}

func (System) CPUFreqMHz(context.Context) (*float64, error) {
	return cpuFreqMHz(), nil
}

func (System) Memory(ctx context.Context) (proto.MemStats, error) {
	var m proto.MemStats
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return m, err
	}
	m.Total = vm.Total
	// "Used" means not available for new programs; caches count as free.
	if vm.Available <= vm.Total {
		m.Used = vm.Total - vm.Available
	}
	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		return m, err
	}
	m.SwapTotal, m.SwapUsed = swap.Total, swap.Used
	return m, nil
}

func (System) Filesystems(ctx context.Context) ([]Filesystem, error) {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]Filesystem, 0, len(parts))
	for _, p := range parts {
		usage, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil {
			continue // e.g. card reader or optical drive without media
		}
		out = append(out, Filesystem{Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype, Total: usage.Total, Used: usage.Used})
	}
	return out, nil
}

func (System) DiskIO(ctx context.Context) (map[string]IOCounter, error) {
	counters, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]IOCounter, len(counters))
	for name, c := range counters {
		out[name] = IOCounter{ReadBytes: c.ReadBytes, WriteBytes: c.WriteBytes}
	}
	return out, nil
}

func (System) Network(ctx context.Context) ([]NetCounter, error) {
	counters, err := net.IOCountersWithContext(ctx, true)
	if err != nil {
		return nil, err
	}
	ifaces, err := net.InterfacesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	up := make(map[string]bool, len(ifaces))
	for _, i := range ifaces {
		for _, f := range i.Flags {
			if f == "up" {
				up[i.Name] = true
			}
		}
	}
	out := make([]NetCounter, 0, len(counters))
	for _, c := range counters {
		out = append(out, NetCounter{Iface: c.Name, Up: up[c.Name], RxBytes: c.BytesRecv, TxBytes: c.BytesSent})
	}
	return out, nil
}

func (System) Temperatures(ctx context.Context) ([]proto.TempStat, error) {
	temps, err := sensors.TemperaturesWithContext(ctx)
	out := make([]proto.TempStat, 0, len(temps))
	for _, t := range temps {
		out = append(out, proto.TempStat{Sensor: t.SensorKey, C: t.Temperature})
	}
	// gopsutil returns partial results together with warnings for single
	// sensors; those are not worth reporting every five seconds.
	var warnings *sensors.Warnings
	if errors.As(err, &warnings) && len(out) > 0 {
		err = nil
	}
	return out, err
}

func (System) GPUs(ctx context.Context) ([]proto.GPUStats, error) {
	return QueryNVIDIA(ctx)
}

func (System) Battery(context.Context) (*proto.Battery, error) {
	return readBattery()
}

func (System) Uptime(ctx context.Context) (uint64, error) {
	return host.UptimeWithContext(ctx)
}
