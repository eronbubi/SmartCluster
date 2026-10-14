package stats

import (
	"io/fs"
	"math"
	"strconv"
	"strings"
)

// readCPUFreq returns the average current frequency in MHz from a view of
// /sys/devices/system/cpu, or nil if cpufreq is not available (common in VMs).
func readCPUFreq(fsys fs.FS) *float64 {
	matches, err := fs.Glob(fsys, "cpu[0-9]*/cpufreq/scaling_cur_freq")
	if err != nil || len(matches) == 0 {
		return nil
	}
	var sum float64
	var n int
	for _, m := range matches {
		b, err := fs.ReadFile(fsys, m)
		if err != nil {
			continue
		}
		khz, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil || khz <= 0 {
			continue
		}
		sum += khz / 1000
		n++
	}
	if n == 0 {
		return nil
	}
	return ptr(math.Round(sum / float64(n)))
}
