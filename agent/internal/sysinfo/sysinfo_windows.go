package sysinfo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows/registry"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

func platformOSInfo(info *proto.OSInfo) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	// DisplayVersion is "22H2", "24H2", ...; CurrentBuild plus UBR is the exact build.
	if v, _, err := k.GetStringValue("DisplayVersion"); err == nil {
		info.Version = v
	}
	build, _, err := k.GetStringValue("CurrentBuild")
	if err != nil {
		return err
	}
	info.Build = build
	if ubr, _, err := k.GetIntegerValue("UBR"); err == nil {
		info.Build = fmt.Sprintf("%s.%d", build, ubr)
	}
	return nil
}

type win32ComputerSystem struct {
	Manufacturer string
	Model        string
}

type win32ComputerSystemProduct struct {
	Version string
}

type win32BIOS struct {
	SMBIOSBIOSVersion string
}

type win32VideoController struct {
	Name string
}

func readDMI(context.Context) (dmiInfo, error) {
	var info dmiInfo
	var cs []win32ComputerSystem
	var product []win32ComputerSystemProduct
	var bios []win32BIOS
	err := errors.Join(
		wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &cs),
		wmi.Query("SELECT Version FROM Win32_ComputerSystemProduct", &product),
		wmi.Query("SELECT SMBIOSBIOSVersion FROM Win32_BIOS", &bios),
	)
	if len(cs) > 0 {
		info.vendor, info.model = cleanDMI(cs[0].Manufacturer), cleanDMI(cs[0].Model)
	}
	if len(product) > 0 {
		info.productVersion = cleanDMI(product[0].Version)
	}
	if len(bios) > 0 {
		info.biosVersion = cleanDMI(bios[0].SMBIOSBIOSVersion)
	}
	return info, err
}

func otherGPUs(context.Context) ([]proto.GPUInfo, error) {
	var controllers []win32VideoController
	if err := wmi.Query("SELECT Name FROM Win32_VideoController", &controllers); err != nil {
		return nil, err
	}
	out := []proto.GPUInfo{}
	for _, c := range controllers {
		name := strings.TrimSpace(c.Name)
		// Remote desktop and indirect display drivers are not GPUs.
		if name == "" || strings.Contains(strings.ToLower(name), "remote") || strings.Contains(strings.ToLower(name), "indirect") {
			continue
		}
		out = append(out, proto.GPUInfo{Name: name})
	}
	return out, nil
}
