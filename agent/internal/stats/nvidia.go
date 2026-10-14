package stats

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

var nvidiaArgs = []string{
	"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu",
	"--format=csv,noheader,nounits",
}

const nvidiaTimeout = 3 * time.Second

// QueryNVIDIA returns stats of NVIDIA GPUs via nvidia-smi. Without the
// NVIDIA driver it returns nil and no error.
func QueryNVIDIA(ctx context.Context) ([]proto.GPUStats, error) {
	bin, err := exec.LookPath("nvidia-smi")
	if errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, nvidiaTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, nvidiaArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi: %w", err)
	}
	return parseNvidiaSMI(out)
}

func parseNvidiaSMI(out []byte) ([]proto.GPUStats, error) {
	r := csv.NewReader(bytes.NewReader(out))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = 5
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse nvidia-smi output: %w", err)
	}
	gpus := make([]proto.GPUStats, 0, len(records))
	for _, rec := range records {
		gpus = append(gpus, proto.GPUStats{
			Name:     strings.TrimSpace(rec[0]),
			UtilPct:  parseFloat(rec[1]),
			MemUsed:  mib(rec[2]),
			MemTotal: mib(rec[3]),
			TempC:    parseFloat(rec[4]),
		})
	}
	return gpus, nil
}

// parseFloat returns nil for "[N/A]", "[Not Supported]" and other non-numbers.
func parseFloat(s string) *float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return &v
}

func mib(s string) *uint64 {
	v := parseFloat(s)
	if v == nil || *v < 0 {
		return nil
	}
	return ptr(uint64(*v) << 20)
}
