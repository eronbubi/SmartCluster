// Package sysinfo collects the static inventory an agent sends in hello:
// operating system and hardware.
package sysinfo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"runtime"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/eronbubi/smartcluster/agent/internal/stats"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Collect returns OS and hardware inventory. Parts that cannot be read are
// left empty and reported in errs.
func Collect(ctx context.Context) (proto.OSInfo, proto.Hardware, []error) {
	var errs []error
	fail := func(what string, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", what, err))
		}
	}

	osInfo, err := osInfo(ctx)
	fail("os", err)

	hw := proto.Hardware{GPUs: []proto.GPUInfo{}, NICs: []proto.NICInfo{}}
	dmi, err := readDMI(ctx)
	fail("dmi", err)
	hw.Vendor, hw.Model, hw.ProductVersion, hw.BIOSVersion = dmi.vendor, dmi.model, dmi.productVersion, dmi.biosVersion

	if infos, err := cpu.InfoWithContext(ctx); err != nil {
		fail("cpu info", err)
	} else if len(infos) > 0 {
		hw.CPU.Model = strings.TrimSpace(infos[0].ModelName)
	}
	hw.CPU.Cores, err = cpu.CountsWithContext(ctx, false)
	fail("cpu cores", err)
	hw.CPU.Threads, err = cpu.CountsWithContext(ctx, true)
	fail("cpu threads", err)

	if vm, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		fail("memory", err)
	} else {
		hw.RAMTotal = vm.Total
	}

	gpus, err := stats.QueryNVIDIA(ctx)
	fail("nvidia", err)
	for _, g := range gpus {
		hw.GPUs = append(hw.GPUs, proto.GPUInfo{Name: g.Name, MemTotal: g.MemTotal})
	}
	others, err := otherGPUs(ctx)
	fail("gpus", err)
	for _, g := range others {
		// nvidia-smi already listed NVIDIA cards, with better data.
		if len(gpus) > 0 && strings.Contains(strings.ToLower(g.Name), "nvidia") {
			continue
		}
		hw.GPUs = append(hw.GPUs, g)
	}

	ifaces, err := net.InterfacesWithContext(ctx)
	fail("interfaces", err)
	hw.NICs = nics(ifaces)

	return osInfo, hw, errs
}

func osInfo(ctx context.Context) (proto.OSInfo, error) {
	info := proto.OSInfo{Platform: runtime.GOOS, Arch: runtime.GOARCH}
	h, err := host.InfoWithContext(ctx)
	if err != nil {
		return info, err
	}
	info.Hostname = h.Hostname
	info.Kernel = h.KernelVersion
	info.Name = h.Platform
	info.Version = h.PlatformVersion
	return info, platformOSInfo(&info)
}

type dmiInfo struct {
	vendor, model, productVersion, biosVersion string
}

// nics lists real network interfaces with their MAC and addresses.
func nics(ifaces []net.InterfaceStat) []proto.NICInfo {
	out := []proto.NICInfo{}
	for _, i := range ifaces {
		if i.HardwareAddr == "" || !stats.KeepInterface(i.Name) {
			continue
		}
		n := proto.NICInfo{Name: i.Name, MAC: strings.ToLower(i.HardwareAddr), Addrs: []string{}}
		for _, a := range i.Addrs {
			n.Addrs = append(n.Addrs, a.Addr)
		}
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b proto.NICInfo) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// parseOSRelease returns PRETTY_NAME and VERSION_ID from /etc/os-release.
func parseOSRelease(data []byte) (prettyName, versionID string) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, `"'`)
		switch key {
		case "PRETTY_NAME":
			prettyName = value
		case "VERSION_ID":
			versionID = value
		}
	}
	return prettyName, versionID
}

// readSysfsDMI reads vendor, model and BIOS from a view of /sys/class/dmi/id.
func readSysfsDMI(fsys fs.FS) dmiInfo {
	read := func(name string) string {
		b, err := fs.ReadFile(fsys, name)
		if err != nil {
			return ""
		}
		return cleanDMI(string(b))
	}
	return dmiInfo{
		vendor:         read("sys_vendor"),
		model:          read("product_name"),
		productVersion: read("product_version"),
		biosVersion:    read("bios_version"),
	}
}

// cleanDMI drops the placeholder strings many vendors leave in SMBIOS fields.
func cleanDMI(s string) string {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "", "to be filled by o.e.m.", "default string", "system product name", "system manufacturer",
		"system version", "not applicable", "not specified", "none", "0", "x.x":
		return ""
	}
	return s
}

// readDRMGPUs lists GPUs from a view of /sys/class/drm as "driver (vendor:device)".
// Linux has no product names without the PCI ID database, so this stays basic.
func readDRMGPUs(fsys fs.FS) []proto.GPUInfo {
	matches, _ := fs.Glob(fsys, "card[0-9]*/device/uevent")
	out := []proto.GPUInfo{}
	for _, m := range matches {
		if strings.Contains(path.Dir(path.Dir(m)), "-") { // card0-HDMI-A-1 are connectors, not cards
			continue
		}
		data, err := fs.ReadFile(fsys, m)
		if err != nil {
			continue
		}
		var driver, pciID string
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "DRIVER="); ok {
				driver = v
			}
			if v, ok := strings.CutPrefix(sc.Text(), "PCI_ID="); ok {
				pciID = strings.ToLower(v)
			}
		}
		if driver == "" {
			continue
		}
		name := driver
		if pciID != "" {
			name = fmt.Sprintf("%s (%s)", driver, pciID)
		}
		out = append(out, proto.GPUInfo{Name: name})
	}
	return out
}
