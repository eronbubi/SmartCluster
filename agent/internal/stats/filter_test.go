package stats

import (
	"testing"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

func TestFilterFilesystems(t *testing.T) {
	in := []Filesystem{
		{Mount: "/home/bind", Device: "/dev/sda1", FSType: "ext4", Total: 100},
		{Mount: "/", Device: "/dev/sda1", FSType: "ext4", Total: 100},
		{Mount: "/boot/efi", Device: "/dev/sda2", FSType: "vfat", Total: 5},
		{Mount: "/snap/core22/1380", Device: "/dev/loop3", FSType: "squashfs", Total: 70},
		{Mount: "/var/lib/docker/overlay2/x/merged", Device: "overlay", FSType: "overlay", Total: 100},
		{Mount: "/run/user/1000", Device: "tmpfs", FSType: "tmpfs", Total: 10},
		{Mount: "/media/cdrom", Device: "/dev/sr0", FSType: "iso9660", Total: 700},
		{Mount: "/mnt/empty", Device: "/dev/sdb1", FSType: "ext4", Total: 0},
		{Mount: "C:", Device: "C:", FSType: "NTFS", Total: 500},
		{Mount: "D:", Device: "D:", FSType: "CDFS", Total: 700},
	}
	got := filterFilesystems(in)
	want := []string{"/", "/boot/efi", "C:"}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want mounts %v", got, want)
	}
	for i, m := range want {
		if got[i].Mount != m {
			t.Errorf("mount %d = %q, want %q", i, got[i].Mount, m)
		}
	}
}

func TestKeepInterface(t *testing.T) {
	for name, want := range map[string]bool{
		"eth0":                               true,
		"enp0s25":                            true,
		"wlp3s0":                             true,
		"Ethernet":                           true,
		"WLAN":                               true,
		"Local Area Connection":              true,
		"lo":                                 false,
		"Loopback Pseudo-Interface 1":        false,
		"docker0":                            false,
		"br-3f2a1b":                          false,
		"veth12ab":                           false,
		"vEthernet (WSL (Hyper-V firewall))": false,
		"Bluetooth-Netzwerkverbindung":       false,
		"isatap.fritz.box":                   false,
	} {
		if got := KeepInterface(name); got != want {
			t.Errorf("KeepInterface(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidTemperatures(t *testing.T) {
	got := validTemperatures([]proto.TempStat{
		{Sensor: "z", C: 41.26}, {Sensor: "zero", C: 0}, {Sensor: "garbage", C: 3000}, {Sensor: "a", C: 30},
	})
	if len(got) != 2 || got[0].Sensor != "a" || got[1].C != 41.3 {
		t.Errorf("got %+v", got)
	}
}

func TestPickCPUTemp(t *testing.T) {
	tests := []struct {
		name  string
		temps []proto.TempStat
		want  *float64
	}{
		{"intel package wins over hotter core", []proto.TempStat{
			{Sensor: "coretemp_core_0", C: 70}, {Sensor: "coretemp_package_id_0", C: 65}}, ptr(65.0)},
		{"amd tctl", []proto.TempStat{{Sensor: "k10temp_tctl", C: 61}, {Sensor: "nvme_composite", C: 45}}, ptr(61.0)},
		{"hottest core", []proto.TempStat{{Sensor: "coretemp_core_0", C: 50}, {Sensor: "coretemp_core_1", C: 58}}, ptr(58.0)},
		{"windows acpi zone", []proto.TempStat{{Sensor: `ACPI\ThermalZone\TZ00_0`, C: 47}}, ptr(47.0)},
		{"only disk sensors", []proto.TempStat{{Sensor: "nvme_composite", C: 45}}, nil},
		{"no sensors", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickCPUTemp(tt.temps)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v, want %v", deref(got), deref(tt.want))
			}
		})
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
