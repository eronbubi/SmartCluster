package stats

import (
	"testing"
	"testing/fstest"
)

func TestParseNvidiaSMI(t *testing.T) {
	out := []byte("NVIDIA GeForce 940MX, 3, 412, 2048, 41\n" +
		"Tesla K80, [N/A], 0, 11441, [Not Supported]\n")
	gpus, err := parseNvidiaSMI(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 2 {
		t.Fatalf("got %d gpus", len(gpus))
	}
	g := gpus[0]
	if g.Name != "NVIDIA GeForce 940MX" || *g.UtilPct != 3 || *g.MemUsed != 412<<20 || *g.MemTotal != 2048<<20 || *g.TempC != 41 {
		t.Errorf("gpu 0 = %+v", g)
	}
	if gpus[1].UtilPct != nil || gpus[1].TempC != nil || *gpus[1].MemTotal != 11441<<20 {
		t.Errorf("gpu 1 = %+v", gpus[1])
	}
}

func TestParseNvidiaSMIRejectsGarbage(t *testing.T) {
	if _, err := parseNvidiaSMI([]byte("NVIDIA-SMI has failed because it couldn't communicate with the driver\n")); err == nil {
		t.Error("error output was parsed as GPU list")
	}
	gpus, err := parseNvidiaSMI(nil)
	if err != nil || len(gpus) != 0 {
		t.Errorf("empty output: %v, %v", gpus, err)
	}
}

func TestReadSysfsBattery(t *testing.T) {
	fsys := fstest.MapFS{
		"AC/type":                  {Data: []byte("Mains\n")},
		"AC/online":                {Data: []byte("1\n")},
		"BAT0/type":                {Data: []byte("Battery\n")},
		"BAT0/capacity":            {Data: []byte("81\n")},
		"BAT0/status":              {Data: []byte("Discharging\n")},
		"BAT0/energy_full":         {Data: []byte("30120000\n")},
		"BAT0/energy_full_design":  {Data: []byte("47520000\n")},
		"hidpp_battery_0/type":     {Data: []byte("Battery\n")},
		"hidpp_battery_0/scope":    {Data: []byte("Device\n")},
		"hidpp_battery_0/capacity": {Data: []byte("5\n")},
	}
	bat, err := readSysfsBattery(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if bat == nil || *bat.Pct != 81 || !bat.OnAC || *bat.HealthPct != 63.4 {
		t.Errorf("battery = %+v", bat)
	}
}

func TestReadSysfsBatteryChargeVariantWithoutAC(t *testing.T) {
	fsys := fstest.MapFS{
		"BAT1/type":               {Data: []byte("Battery\n")},
		"BAT1/capacity":           {Data: []byte("40\n")},
		"BAT1/status":             {Data: []byte("Discharging\n")},
		"BAT1/charge_full":        {Data: []byte("4000000\n")},
		"BAT1/charge_full_design": {Data: []byte("5000000\n")},
	}
	bat, err := readSysfsBattery(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if bat == nil || bat.OnAC || *bat.HealthPct != 80 {
		t.Errorf("battery = %+v", bat)
	}
}

func TestReadSysfsBatteryChargingImpliesAC(t *testing.T) {
	fsys := fstest.MapFS{
		"BAT0/type":   {Data: []byte("Battery\n")},
		"BAT0/status": {Data: []byte("Charging\n")},
	}
	bat, _ := readSysfsBattery(fsys)
	if bat == nil || !bat.OnAC || bat.Pct != nil || bat.HealthPct != nil {
		t.Errorf("battery = %+v", bat)
	}
}

func TestReadSysfsNoBattery(t *testing.T) {
	bat, err := readSysfsBattery(fstest.MapFS{"AC/type": {Data: []byte("Mains\n")}, "AC/online": {Data: []byte("1\n")}})
	if err != nil || bat != nil {
		t.Errorf("desktop: battery = %+v, err = %v", bat, err)
	}
}

func TestBatteryFromPowerStatus(t *testing.T) {
	if b := batteryFromPowerStatus(powerStatus{ACLineStatus: 1, BatteryFlag: 128, BatteryLifePercent: 255}); b != nil {
		t.Errorf("desktop without battery: %+v", b)
	}
	if b := batteryFromPowerStatus(powerStatus{BatteryFlag: 255}); b != nil {
		t.Errorf("unknown status: %+v", b)
	}
	b := batteryFromPowerStatus(powerStatus{ACLineStatus: 0, BatteryFlag: 1, BatteryLifePercent: 64})
	if b == nil || b.OnAC || *b.Pct != 64 || b.HealthPct != nil {
		t.Errorf("laptop on battery: %+v", b)
	}
	b = batteryFromPowerStatus(powerStatus{ACLineStatus: 1, BatteryFlag: 8, BatteryLifePercent: 255})
	if b == nil || !b.OnAC || b.Pct != nil {
		t.Errorf("charging, unknown percent: %+v", b)
	}
}

func TestReadCPUFreq(t *testing.T) {
	fsys := fstest.MapFS{
		"cpu0/cpufreq/scaling_cur_freq": {Data: []byte("2400000\n")},
		"cpu1/cpufreq/scaling_cur_freq": {Data: []byte("1600000\n")},
		"cpufreq/policy0/x":             {Data: []byte("ignored")},
	}
	if got := readCPUFreq(fsys); got == nil || *got != 2000 {
		t.Errorf("got %v, want 2000", deref(got))
	}
	if got := readCPUFreq(fstest.MapFS{}); got != nil {
		t.Errorf("without cpufreq: %v", *got)
	}
}
