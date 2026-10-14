package api

import (
	"time"

	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Node is a node as the API returns it. Keep in sync with docs/api/openapi.yaml.
type Node struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Hostname     string          `json:"hostname"`
	Status       string          `json:"status"`
	OS           *proto.OSInfo   `json:"os"`
	Hardware     *proto.Hardware `json:"hardware"`
	AgentVersion string          `json:"agent_version"`
	IPAddresses  []string        `json:"ip_addresses"`
	MACAddresses []string        `json:"mac_addresses"`
	CertNotAfter time.Time       `json:"cert_not_after"`
	CreatedAt    time.Time       `json:"created_at"`
	LastSeenAt   *time.Time      `json:"last_seen_at"`
	// Latest is the newest sample since the control center started, or null.
	Latest *proto.Stats `json:"latest"`
}

// StatsResponse is a node's time series.
type StatsResponse struct {
	Resolution string    `json:"resolution"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Points     []Point   `json:"points"`
}

// Point is one sample (raw) or one 5-minute average (5m). Null means not measured.
type Point struct {
	TS          time.Time `json:"ts"`
	CPUPct      *float64  `json:"cpu_pct"`
	MemUsed     *int64    `json:"mem_used"`
	MemTotal    *int64    `json:"mem_total"`
	DiskUsedPct *float64  `json:"disk_used_pct"`
	NetRxBps    *int64    `json:"net_rx_bps"`
	NetTxBps    *int64    `json:"net_tx_bps"`
	CPUTempC    *float64  `json:"cpu_temp_c"`
	GPUUtilPct  *float64  `json:"gpu_util_pct"`
}
