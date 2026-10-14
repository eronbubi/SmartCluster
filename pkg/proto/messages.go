package proto

import (
	"encoding/json"
	"time"
)

// Hello is the first frame an agent sends after connecting.
type Hello struct {
	Protocol     int      `json:"protocol"`
	AgentVersion string   `json:"agent_version"`
	NodeID       string   `json:"node_id"`
	OS           OSInfo   `json:"os"`
	Hardware     Hardware `json:"hardware"`
	Capabilities []string `json:"capabilities"`
}

// OSInfo describes the operating system of a node.
type OSInfo struct {
	Platform string `json:"platform"` // "linux" or "windows"
	Name     string `json:"name"`     // e.g. "Debian GNU/Linux 13 (trixie)"
	Version  string `json:"version"`
	Build    string `json:"build,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
}

// Hardware is the static hardware inventory of a node.
type Hardware struct {
	Vendor         string    `json:"vendor"`
	Model          string    `json:"model"`
	ProductVersion string    `json:"product_version,omitempty"` // friendly model name on some vendors (Lenovo)
	BIOSVersion    string    `json:"bios_version,omitempty"`
	CPU            CPUInfo   `json:"cpu"`
	RAMTotal       uint64    `json:"ram_total"`
	GPUs           []GPUInfo `json:"gpus"`
	NICs           []NICInfo `json:"nics"`
}

// CPUInfo describes the processor.
type CPUInfo struct {
	Model   string `json:"model"`
	Cores   int    `json:"cores"`
	Threads int    `json:"threads"`
}

// GPUInfo describes a graphics adapter.
type GPUInfo struct {
	Name     string  `json:"name"`
	MemTotal *uint64 `json:"mem_total"`
}

// NICInfo describes a network interface.
type NICInfo struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	Addrs []string `json:"addrs"` // CIDR notation
}

// HelloOK is the control center's answer to Hello.
type HelloOK struct {
	Protocol        int       `json:"protocol"`
	ServerTime      time.Time `json:"server_time"`
	StatsIntervalS  int       `json:"stats_interval_s"`
	MinAgentVersion string    `json:"min_agent_version"`
	CmdPubKeys      []PubKey  `json:"cmd_pubkeys"`
}

// PubKey is an Ed25519 public key, base64url without padding.
type PubKey struct {
	KeyID string `json:"kid"`
	Key   string `json:"key"`
}

// Stats is the periodic telemetry frame. Byte counts are integers; values a
// platform cannot provide are null rather than zero.
type Stats struct {
	TS      time.Time   `json:"ts"`
	CPU     CPUStats    `json:"cpu"`
	Mem     MemStats    `json:"mem"`
	Disks   []DiskStats `json:"disks"`
	Net     []NetStats  `json:"net"`
	GPUs    []GPUStats  `json:"gpus"`
	Temps   []TempStat  `json:"temps"`
	Battery *Battery    `json:"battery"`
	UptimeS uint64      `json:"uptime_s"`
}

// CPUStats is processor load.
type CPUStats struct {
	Pct     float64   `json:"pct"`
	PerCore []float64 `json:"per_core"`
	Load1   *float64  `json:"load1"` // null on Windows
	FreqMHz *float64  `json:"freq_mhz"`
	TempC   *float64  `json:"temp_c"`
}

// MemStats is memory usage in bytes.
type MemStats struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	SwapTotal uint64 `json:"swap_total"`
	SwapUsed  uint64 `json:"swap_used"`
}

// DiskStats is usage and throughput of one mounted file system.
type DiskStats struct {
	Mount    string  `json:"mount"`
	Total    uint64  `json:"total"`
	Used     uint64  `json:"used"`
	ReadBps  *uint64 `json:"read_bps"`
	WriteBps *uint64 `json:"write_bps"`
}

// NetStats is throughput of one network interface.
type NetStats struct {
	Iface string `json:"iface"`
	Up    bool   `json:"up"`
	RxBps uint64 `json:"rx_bps"`
	TxBps uint64 `json:"tx_bps"`
}

// GPUStats is load of one graphics adapter.
type GPUStats struct {
	Name     string   `json:"name"`
	UtilPct  *float64 `json:"util_pct"`
	MemUsed  *uint64  `json:"mem_used"`
	MemTotal *uint64  `json:"mem_total"`
	TempC    *float64 `json:"temp_c"`
}

// TempStat is one temperature sensor.
type TempStat struct {
	Sensor string  `json:"sensor"`
	C      float64 `json:"c"`
}

// Battery is the state of the built-in battery.
type Battery struct {
	Pct       *float64 `json:"pct"`
	OnAC      bool     `json:"on_ac"`
	HealthPct *float64 `json:"health_pct"`
}

// CmdAck confirms that a command passed verification and will be executed.
type CmdAck struct {
	CmdID string `json:"cmd_id"`
}

// CmdReject reports a command that failed verification.
type CmdReject struct {
	CmdID  string `json:"cmd_id"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// Reject codes (CmdReject.Code).
const (
	RejectBadSignature   = "bad_signature"
	RejectExpired        = "expired"
	RejectReplay         = "replay"
	RejectWrongNode      = "wrong_node"
	RejectUnknownType    = "unknown_type"
	RejectInvalidPayload = "invalid_payload"
	RejectBusy           = "busy"
)

// CmdResult is the outcome of an executed command.
type CmdResult struct {
	CmdID     string          `json:"cmd_id"`
	OK        bool            `json:"ok"`
	ErrorCode string          `json:"error_code,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}
