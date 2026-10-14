package sysinfo

import (
	"testing"
	"testing/fstest"

	"github.com/shirou/gopsutil/v4/net"
)

func TestParseOSRelease(t *testing.T) {
	data := []byte(`PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
ID=debian
`)
	name, version := parseOSRelease(data)
	if name != "Debian GNU/Linux 13 (trixie)" || version != "13" {
		t.Errorf("got %q %q", name, version)
	}
}

func TestReadSysfsDMI(t *testing.T) {
	info := readSysfsDMI(fstest.MapFS{
		"sys_vendor":      {Data: []byte("LENOVO\n")},
		"product_name":    {Data: []byte("4236AB1\n")},
		"product_version": {Data: []byte("ThinkPad T420\n")},
		"bios_version":    {Data: []byte("83ET82WW (1.52 )\n")},
	})
	want := dmiInfo{vendor: "LENOVO", model: "4236AB1", productVersion: "ThinkPad T420", biosVersion: "83ET82WW (1.52 )"}
	if info != want {
		t.Errorf("got %+v, want %+v", info, want)
	}
}

func TestCleanDMIDropsPlaceholders(t *testing.T) {
	for _, s := range []string{"To Be Filled By O.E.M.", "Default string", "System Product Name", "System Version", "  "} {
		if got := cleanDMI(s); got != "" {
			t.Errorf("cleanDMI(%q) = %q", s, got)
		}
	}
	if got := cleanDMI(" HP EliteBook 840 G3 "); got != "HP EliteBook 840 G3" {
		t.Errorf("got %q", got)
	}
}

func TestReadDRMGPUs(t *testing.T) {
	gpus := readDRMGPUs(fstest.MapFS{
		"card0/device/uevent":          {Data: []byte("DRIVER=i915\nPCI_CLASS=30000\nPCI_ID=8086:0126\n")},
		"card0-HDMI-A-1/device/uevent": {Data: []byte("DRIVER=i915\n")},
		"card1/device/uevent":          {Data: []byte("PCI_ID=10DE:134D\n")},
	})
	if len(gpus) != 1 || gpus[0].Name != "i915 (8086:0126)" {
		t.Errorf("got %+v", gpus)
	}
}

func TestNICs(t *testing.T) {
	got := nics([]net.InterfaceStat{
		{Name: "lo", HardwareAddr: "", Addrs: net.InterfaceAddrList{{Addr: "127.0.0.1/8"}}},
		{Name: "wlp3s0", HardwareAddr: "AA:BB:CC:00:11:22", Addrs: net.InterfaceAddrList{{Addr: "192.168.1.20/24"}}},
		{Name: "docker0", HardwareAddr: "02:42:ac:11:00:01"},
		{Name: "enp0s25", HardwareAddr: "f0:de:f1:00:00:01"},
	})
	if len(got) != 2 || got[0].Name != "enp0s25" || got[1].MAC != "aa:bb:cc:00:11:22" || got[1].Addrs[0] != "192.168.1.20/24" {
		t.Errorf("got %+v", got)
	}
	if got[0].Addrs == nil {
		t.Error("addrs must be an empty list, not null")
	}
}
