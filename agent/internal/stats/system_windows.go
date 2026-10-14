package stats

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

var procGetSystemPowerStatus = syscall.NewLazyDLL("kernel32.dll").NewProc("GetSystemPowerStatus")

// Windows has no load average.
func load1(context.Context) (*float64, error) { return nil, nil }

// The current clock speed is only available through WMI, which is too slow to
// query every few seconds.
func cpuFreqMHz() *float64 { return nil }

func readBattery() (*proto.Battery, error) {
	var s powerStatus
	r, _, err := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&s)))
	if r == 0 {
		return nil, fmt.Errorf("GetSystemPowerStatus: %w", err)
	}
	return batteryFromPowerStatus(s), nil
}
