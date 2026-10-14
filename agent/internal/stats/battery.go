package stats

import (
	"errors"
	"io/fs"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// readSysfsBattery reads the built-in battery from a view of
// /sys/class/power_supply. It returns nil if the machine has no battery.
// Batteries of peripherals (scope "Device", e.g. a wireless mouse) are ignored.
func readSysfsBattery(fsys fs.FS) (*proto.Battery, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var bat *proto.Battery
	var status string
	onAC := false
	for _, e := range entries {
		dir := e.Name()
		switch readAttr(fsys, dir, "type") {
		case "Mains", "USB", "USB_C", "USB_PD":
			if readAttr(fsys, dir, "online") == "1" {
				onAC = true
			}
		case "Battery":
			if bat != nil || readAttr(fsys, dir, "scope") == "Device" {
				continue
			}
			bat = &proto.Battery{}
			if v, err := strconv.ParseFloat(readAttr(fsys, dir, "capacity"), 64); err == nil {
				bat.Pct = ptr(v)
			}
			bat.HealthPct = batteryHealth(fsys, dir)
			status = readAttr(fsys, dir, "status")
		}
	}
	if bat == nil {
		return nil, nil
	}
	bat.OnAC = onAC || status == "Charging" || status == "Full" || status == "Not charging"
	return bat, nil
}

func batteryHealth(fsys fs.FS, dir string) *float64 {
	for _, pair := range [][2]string{{"energy_full", "energy_full_design"}, {"charge_full", "charge_full_design"}} {
		full, err1 := strconv.ParseFloat(readAttr(fsys, dir, pair[0]), 64)
		design, err2 := strconv.ParseFloat(readAttr(fsys, dir, pair[1]), 64)
		if err1 == nil && err2 == nil && design > 0 {
			return ptr(math.Round(full/design*1000) / 10)
		}
	}
	return nil
}

func readAttr(fsys fs.FS, dir, name string) string {
	b, err := fs.ReadFile(fsys, path.Join(dir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// powerStatus mirrors the Windows SYSTEM_POWER_STATUS structure.
type powerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

const (
	batteryFlagNoBattery = 128
	batteryFlagUnknown   = 255
	unknownPercent       = 255
)

// batteryFromPowerStatus converts GetSystemPowerStatus output. Windows does
// not report battery health there; that stays null.
func batteryFromPowerStatus(s powerStatus) *proto.Battery {
	if s.BatteryFlag == batteryFlagUnknown || s.BatteryFlag&batteryFlagNoBattery != 0 {
		return nil
	}
	bat := &proto.Battery{OnAC: s.ACLineStatus == 1}
	if s.BatteryLifePercent != unknownPercent {
		bat.Pct = ptr(float64(s.BatteryLifePercent))
	}
	return bat
}
