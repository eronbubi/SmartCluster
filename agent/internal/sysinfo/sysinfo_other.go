//go:build !linux && !windows

package sysinfo

import (
	"context"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

func platformOSInfo(*proto.OSInfo) error { return nil }

func readDMI(context.Context) (dmiInfo, error) { return dmiInfo{}, nil }

func otherGPUs(context.Context) ([]proto.GPUInfo, error) { return nil, nil }
