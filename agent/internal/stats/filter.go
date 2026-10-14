package stats

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

var ignoredFSTypes = map[string]bool{
	"autofs": true, "cdfs": true, "cgroup": true, "cgroup2": true, "devtmpfs": true,
	"efivarfs": true, "fuse.lxcfs": true, "iso9660": true, "nsfs": true, "overlay": true,
	"proc": true, "ramfs": true, "squashfs": true, "sysfs": true, "tmpfs": true, "udf": true,
}

var ignoredMountPrefixes = []string{"/snap/", "/var/lib/docker/", "/run/", "/sys/", "/proc/", "/dev/"}

// filterFilesystems drops pseudo and container file systems and bind mounts of
// a device that is already listed. The result is sorted by mount point.
func filterFilesystems(in []Filesystem) []Filesystem {
	sorted := slices.Clone(in)
	// Shorter mount points first, so "/" wins over a bind mount of the same device.
	slices.SortStableFunc(sorted, func(a, b Filesystem) int { return cmp.Compare(len(a.Mount), len(b.Mount)) })

	seen := map[string]bool{}
	var out []Filesystem
	for _, f := range sorted {
		if f.Total == 0 || ignoredFSTypes[strings.ToLower(f.FSType)] {
			continue
		}
		if slices.ContainsFunc(ignoredMountPrefixes, func(p string) bool { return strings.HasPrefix(f.Mount, p) }) {
			continue
		}
		if f.Device != "" && seen[f.Device] {
			continue
		}
		seen[f.Device] = true
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b Filesystem) int { return strings.Compare(a.Mount, b.Mount) })
	return out
}

var ignoredInterfacePrefixes = []string{
	"loopback", "docker", "br-", "veth", "virbr", "vethernet", "isatap", "teredo",
	"6to4", "cni", "flannel", "vmnet", "vboxnet", "bluetooth",
}

// KeepInterface reports whether a network interface is a real uplink worth
// showing, as opposed to loopback, container bridges and tunnel adapters.
func KeepInterface(name string) bool {
	lower := strings.ToLower(name)
	if lower == "lo" || strings.HasPrefix(lower, "lo:") {
		return false
	}
	return !slices.ContainsFunc(ignoredInterfacePrefixes, func(p string) bool { return strings.HasPrefix(lower, p) })
}

// validTemperatures drops implausible readings (unsupported sensors often
// report 0 or garbage) and sorts by sensor name.
func validTemperatures(in []proto.TempStat) []proto.TempStat {
	out := []proto.TempStat{}
	for _, t := range in {
		if t.C > 0 && t.C < 150 {
			out = append(out, proto.TempStat{Sensor: t.Sensor, C: math.Round(t.C*10) / 10})
		}
	}
	slices.SortFunc(out, func(a, b proto.TempStat) int { return strings.Compare(a.Sensor, b.Sensor) })
	return out
}

// pickCPUTemp chooses the sensor that best represents the CPU: the package
// sensor (Intel), Tctl/Tdie (AMD), the hottest core, or an ACPI thermal zone.
func pickCPUTemp(temps []proto.TempStat) *float64 {
	stages := []func(string) bool{
		func(s string) bool { return strings.Contains(s, "package") },
		func(s string) bool { return strings.Contains(s, "tctl") || strings.Contains(s, "tdie") },
		func(s string) bool {
			return strings.Contains(s, "coretemp") || strings.Contains(s, "k10temp") || strings.Contains(s, "cpu")
		},
		func(s string) bool { return strings.Contains(s, "acpitz") || strings.Contains(s, "thermalzone") },
	}
	for _, match := range stages {
		var best *float64
		for _, t := range temps {
			if match(strings.ToLower(t.Sensor)) && (best == nil || t.C > *best) {
				best = ptr(t.C)
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}
