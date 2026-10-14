package stats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// fakeReader returns scripted readings; each Sample call advances step.
type fakeReader struct {
	step     int
	cpu      [][]CPUTimes
	disk     []map[string]IOCounter
	net      [][]NetCounter
	fs       []Filesystem
	temps    []proto.TempStat
	tempsErr error
	battery  *proto.Battery
}

func (f *fakeReader) at(n int) int { return min(f.step, n-1) }

func (f *fakeReader) CPUTimes(context.Context) ([]CPUTimes, error) {
	return f.cpu[f.at(len(f.cpu))], nil
}
func (f *fakeReader) Load1(context.Context) (*float64, error) { return ptr(0.5), nil }
func (f *fakeReader) CPUFreqMHz(context.Context) (*float64, error) {
	return nil, nil
}
func (f *fakeReader) Memory(context.Context) (proto.MemStats, error) {
	return proto.MemStats{Total: 8 << 30, Used: 3 << 30}, nil
}
func (f *fakeReader) Filesystems(context.Context) ([]Filesystem, error) { return f.fs, nil }
func (f *fakeReader) DiskIO(context.Context) (map[string]IOCounter, error) {
	return f.disk[f.at(len(f.disk))], nil
}
func (f *fakeReader) Network(context.Context) ([]NetCounter, error) {
	return f.net[f.at(len(f.net))], nil
}
func (f *fakeReader) Temperatures(context.Context) ([]proto.TempStat, error) {
	return f.temps, f.tempsErr
}
func (f *fakeReader) GPUs(context.Context) ([]proto.GPUStats, error)  { return nil, nil }
func (f *fakeReader) Battery(context.Context) (*proto.Battery, error) { return f.battery, nil }
func (f *fakeReader) Uptime(context.Context) (uint64, error)          { return 3600, nil }

func newFake() *fakeReader {
	return &fakeReader{
		cpu: [][]CPUTimes{
			{{Busy: 100, Total: 1000}, {Busy: 200, Total: 1000}},
			{{Busy: 110, Total: 1010}, {Busy: 202, Total: 1010}}, // core 0: 100 %, core 1: 20 %
		},
		disk: []map[string]IOCounter{
			{"sda1": {ReadBytes: 1000, WriteBytes: 5000}},
			{"sda1": {ReadBytes: 6000, WriteBytes: 5000}},
		},
		net: [][]NetCounter{
			{{Iface: "eth0", Up: true, RxBytes: 0, TxBytes: 100}, {Iface: "lo", Up: true}, {Iface: "docker0", Up: true}},
			{{Iface: "eth0", Up: true, RxBytes: 50000, TxBytes: 600}, {Iface: "lo", Up: true}, {Iface: "docker0", Up: true}},
		},
		fs: []Filesystem{
			{Mount: "/", Device: "/dev/sda1", FSType: "ext4", Total: 100, Used: 40},
			{Mount: "/run", Device: "tmpfs", FSType: "tmpfs", Total: 10, Used: 1},
		},
		temps: []proto.TempStat{{Sensor: "coretemp_package_id_0", C: 55}, {Sensor: "nvme_composite", C: 40}},
	}
}

var t0 = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func TestFirstSampleOnlyPrimes(t *testing.T) {
	c := New(newFake())
	s, errs := c.Sample(context.Background(), t0)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if s.CPU.Pct != 0 || s.Net[0].RxBps != 0 {
		t.Errorf("first sample has rates: cpu %v, rx %v", s.CPU.Pct, s.Net[0].RxBps)
	}
	if len(s.CPU.PerCore) != 2 {
		t.Errorf("per_core has %d entries, want 2", len(s.CPU.PerCore))
	}
	if s.Disks[0].ReadBps != nil {
		t.Error("disk rate set without a previous sample")
	}
}

func TestRatesBetweenSamples(t *testing.T) {
	f := newFake()
	c := New(f)
	c.Sample(context.Background(), t0)
	f.step = 1
	s, errs := c.Sample(context.Background(), t0.Add(5*time.Second))
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	if s.CPU.PerCore[0] != 100 || s.CPU.PerCore[1] != 20 {
		t.Errorf("per core = %v, want [100 20]", s.CPU.PerCore)
	}
	if s.CPU.Pct != 60 {
		t.Errorf("cpu pct = %v, want 60", s.CPU.Pct)
	}
	if len(s.Disks) != 1 || s.Disks[0].Mount != "/" {
		t.Fatalf("disks = %+v, want only /", s.Disks)
	}
	if got := *s.Disks[0].ReadBps; got != 1000 {
		t.Errorf("read rate = %d B/s, want 1000", got)
	}
	if got := *s.Disks[0].WriteBps; got != 0 {
		t.Errorf("write rate = %d B/s, want 0", got)
	}
	if len(s.Net) != 1 || s.Net[0].Iface != "eth0" {
		t.Fatalf("net = %+v, want only eth0", s.Net)
	}
	if s.Net[0].RxBps != 10000 || s.Net[0].TxBps != 100 {
		t.Errorf("eth0 rx/tx = %d/%d, want 10000/100", s.Net[0].RxBps, s.Net[0].TxBps)
	}
	if s.CPU.TempC == nil || *s.CPU.TempC != 55 {
		t.Errorf("cpu temp = %v, want 55", s.CPU.TempC)
	}
	if !s.TS.Equal(t0.Add(5*time.Second)) || s.UptimeS != 3600 || *s.CPU.Load1 != 0.5 {
		t.Errorf("unexpected scalar fields %+v", s)
	}
}

func TestCounterResetGivesZero(t *testing.T) {
	f := newFake()
	f.net = [][]NetCounter{
		{{Iface: "eth0", Up: true, RxBytes: 900000, TxBytes: 900000}},
		{{Iface: "eth0", Up: true, RxBytes: 10, TxBytes: 10}},
	}
	c := New(f)
	c.Sample(context.Background(), t0)
	f.step = 1
	s, _ := c.Sample(context.Background(), t0.Add(5*time.Second))
	if s.Net[0].RxBps != 0 || s.Net[0].TxBps != 0 {
		t.Errorf("rates after reset = %d/%d, want 0/0", s.Net[0].RxBps, s.Net[0].TxBps)
	}
}

func TestFailedReadingDoesNotBreakSample(t *testing.T) {
	f := newFake()
	f.temps = nil
	f.tempsErr = errors.New("WMI access denied")
	s, errs := New(f).Sample(context.Background(), t0)
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want one error", errs)
	}
	if s.CPU.TempC != nil || len(s.Temps) != 0 {
		t.Error("temperature set despite failed reading")
	}
	if s.Mem.Total == 0 || len(s.Disks) != 1 {
		t.Error("other readings missing")
	}
}

func TestEmptySlicesAreNotNull(t *testing.T) {
	f := newFake()
	f.fs, f.temps = nil, nil
	f.net = [][]NetCounter{nil}
	s, _ := New(f).Sample(context.Background(), t0)
	if s.Disks == nil || s.Net == nil || s.GPUs == nil || s.Temps == nil {
		t.Errorf("nil slice would encode as null: %+v", s)
	}
}

func TestCPUPercentWithChangedCoreCount(t *testing.T) {
	// CPU hotplug or a reader hiccup must not panic or produce garbage.
	pct, perCore := cpuPercent([]CPUTimes{{Busy: 1, Total: 10}}, []CPUTimes{{Busy: 6, Total: 20}, {Busy: 5, Total: 5}})
	if pct != 50 || len(perCore) != 2 || perCore[0] != 50 || perCore[1] != 0 {
		t.Errorf("pct %v per core %v", pct, perCore)
	}
}

func TestRate(t *testing.T) {
	tests := []struct {
		before, now uint64
		seconds     float64
		want        uint64
	}{
		{0, 5000, 5, 1000},
		{5000, 4000, 5, 0},
		{0, 5000, 0, 0},
		{100, 100, 5, 0},
	}
	for _, tt := range tests {
		if got := rate(tt.before, tt.now, tt.seconds); got != tt.want {
			t.Errorf("rate(%d, %d, %v) = %d, want %d", tt.before, tt.now, tt.seconds, got, tt.want)
		}
	}
}

func TestDeviceKey(t *testing.T) {
	for in, want := range map[string]string{"/dev/sda1": "sda1", "/dev/nvme0n1p2": "nvme0n1p2", "C:": "C:"} {
		if got := deviceKey(in); got != want {
			t.Errorf("deviceKey(%q) = %q, want %q", in, got, want)
		}
	}
}
