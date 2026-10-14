package stats

import (
	"context"
	"os"

	"github.com/shirou/gopsutil/v4/load"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

func load1(ctx context.Context) (*float64, error) {
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return ptr(avg.Load1), nil
}

func cpuFreqMHz() *float64 {
	return readCPUFreq(os.DirFS("/sys/devices/system/cpu"))
}

func readBattery() (*proto.Battery, error) {
	return readSysfsBattery(os.DirFS("/sys/class/power_supply"))
}
