package sysinfo

import (
	"context"
	"os"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

func platformOSInfo(info *proto.OSInfo) error {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	if name, version := parseOSRelease(data); name != "" {
		info.Name, info.Version = name, version
	}
	return nil
}

func readDMI(context.Context) (dmiInfo, error) {
	return readSysfsDMI(os.DirFS("/sys/class/dmi/id")), nil
}

func otherGPUs(context.Context) ([]proto.GPUInfo, error) {
	return readDRMGPUs(os.DirFS("/sys/class/drm")), nil
}
