//go:build !linux && !windows

package stats

import (
	"context"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Only Linux and Windows nodes are supported; this keeps the package building
// elsewhere (e.g. for developers on macOS).

func load1(context.Context) (*float64, error) { return nil, nil }

func cpuFreqMHz() *float64 { return nil }

func readBattery() (*proto.Battery, error) { return nil, nil }
