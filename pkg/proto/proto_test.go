package proto

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	ts := time.Date(2026, 10, 14, 12, 0, 5, 0, time.FixedZone("CEST", 2*3600))
	in := CmdAck{CmdID: "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e"}

	frame, err := Encode(TypeCmdAck, "id-1", ts, in)
	if err != nil {
		t.Fatal(err)
	}
	env, err := Decode(frame)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != TypeCmdAck || env.ID != "id-1" {
		t.Errorf("envelope = %+v", env)
	}
	if !env.TS.Equal(ts) || env.TS.Location() != time.UTC {
		t.Errorf("ts = %v, want %v in UTC", env.TS, ts)
	}
	var out CmdAck
	if err := env.Unmarshal(&out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("payload = %+v, want %+v", out, in)
	}
}

func TestEnvelopeWireFormat(t *testing.T) {
	frame, err := Encode(TypeStats, "", time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"t":"stats","ts":"2026-10-14T12:00:00Z","d":{}}`; string(frame) != want {
		t.Errorf("frame = %s, want %s", frame, want)
	}
}

func TestDecodeRejects(t *testing.T) {
	if _, err := Decode([]byte(`{"ts":"2026-10-14T12:00:00Z","d":{}}`)); !errors.Is(err, ErrMissingType) {
		t.Errorf("missing type: got %v", err)
	}
	if _, err := Decode([]byte(`not json`)); err == nil {
		t.Error("invalid JSON accepted")
	}
	big := `{"t":"x","d":"` + strings.Repeat("a", MaxMessageBytes) + `"}`
	if _, err := Decode([]byte(big)); !errors.Is(err, ErrTooLarge) {
		t.Errorf("oversized frame: got %v", err)
	}
}

func TestEncodeRejectsOversized(t *testing.T) {
	_, err := Encode(TypeStats, "", time.Now(), strings.Repeat("a", MaxMessageBytes))
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("got %v, want ErrTooLarge", err)
	}
}

func TestUnmarshalIgnoresUnknownFields(t *testing.T) {
	env, err := Decode([]byte(`{"t":"cmd.ack","ts":"2026-10-14T12:00:00Z","d":{"cmd_id":"x","added_later":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	var ack CmdAck
	if err := env.Unmarshal(&ack); err != nil {
		t.Fatal(err)
	}
	if ack.CmdID != "x" {
		t.Errorf("cmd_id = %q", ack.CmdID)
	}
}

func TestUnmarshalWithoutPayload(t *testing.T) {
	env, err := Decode([]byte(`{"t":"cmd.ack","ts":"2026-10-14T12:00:00Z"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Unmarshal(&CmdAck{}); err == nil {
		t.Error("missing payload accepted")
	}
}

// The control center and the dashboard rely on these field names and on null
// for values a platform cannot measure.
func TestStatsWireFormat(t *testing.T) {
	temp := 52.0
	stats := Stats{
		TS:    time.Date(2026, 10, 14, 12, 0, 5, 0, time.UTC),
		CPU:   CPUStats{Pct: 12.5, PerCore: []float64{10, 15}, TempC: &temp},
		Mem:   MemStats{Total: 8, Used: 3},
		Disks: []DiskStats{{Mount: "/", Total: 100, Used: 40}},
		Net:   []NetStats{{Iface: "eth0", Up: true, RxBps: 5, TxBps: 2}},
		GPUs:  []GPUStats{},
		Temps: []TempStat{{Sensor: "coretemp", C: 52}},
	}
	got, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"ts":"2026-10-14T12:00:05Z",` +
		`"cpu":{"pct":12.5,"per_core":[10,15],"load1":null,"freq_mhz":null,"temp_c":52},` +
		`"mem":{"total":8,"used":3,"swap_total":0,"swap_used":0},` +
		`"disks":[{"mount":"/","total":100,"used":40,"read_bps":null,"write_bps":null}],` +
		`"net":[{"iface":"eth0","up":true,"rx_bps":5,"tx_bps":2}],` +
		`"gpus":[],"temps":[{"sensor":"coretemp","c":52}],"battery":null,"uptime_s":0}`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
