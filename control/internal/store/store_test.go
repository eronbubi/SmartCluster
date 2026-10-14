package store

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/control/internal/enroll"
	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/control/internal/secrets"
	"github.com/eronbubi/smartcluster/control/internal/testdb"
	pkgenroll "github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func newStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	box, _ := secrets.NewBox(make([]byte, 32))
	return New(pool, box)
}

func TestClusterRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.LoadCluster(ctx); !errors.Is(err, ErrNoCluster) {
		t.Fatalf("empty database: %v", err)
	}
	ca, _ := pki.NewCA("home", time.Now())
	_, cmdKey, _ := ed25519.GenerateKey(nil)
	want := &Cluster{ID: ids.NewV7(), Name: "home", CA: ca, CmdKeyID: "cmd-1", CmdKey: cmdKey}
	if err := s.CreateCluster(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadCluster(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || !got.CA.Cert.Equal(ca.Cert) || !got.CA.Key.Equal(ca.Key) || !got.CmdKey.Equal(cmdKey) {
		t.Error("cluster changed in the round trip")
	}
	if err := s.CreateCluster(ctx, want); err == nil {
		t.Error("a second cluster was created")
	}
}

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ca, _ := pki.NewCA("home", time.Now())
	_, cmdKey, _ := ed25519.GenerateKey(nil)
	if err := s.CreateCluster(ctx, &Cluster{ID: ids.NewV7(), Name: "home", CA: ca, CmdKeyID: "cmd-1", CmdKey: cmdKey}); err != nil {
		t.Fatal(err)
	}
	var ct []byte
	if err := s.pool.QueryRow(ctx, `SELECT ciphertext FROM secrets WHERE name = 'cmd_key'`).Scan(&ct); err != nil {
		t.Fatal(err)
	}
	if string(ct) == string(cmdKey.Seed()) {
		t.Error("command key stored in plain text")
	}
	wrong, _ := secrets.NewBox(append(make([]byte, 31), 1))
	if _, err := New(s.pool, wrong).LoadCluster(ctx); err == nil {
		t.Error("cluster loaded with the wrong master key")
	}
}

func TestEnrollmentFlow(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Now()
	code := pkgenroll.NewCode()
	codeID, err := s.CreateJoinCode(ctx, code.Key(), now.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateJoinCode(ctx, pkgenroll.NewCode().Key(), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, err := s.ActiveCodes(ctx, now)
	if err != nil || len(active) != 1 || active[0].ID != codeID || string(active[0].Key) != string(code.Key()) {
		t.Fatalf("active codes %+v, %v", active, err)
	}

	node := enroll.NewNode{ID: ids.NewV7(), Name: "t420", CertSerial: "ab12", CertNotAfter: now.Add(365 * 24 * time.Hour)}
	if err := s.CompleteEnrollment(ctx, codeID, node); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteEnrollment(ctx, codeID, enroll.NewNode{ID: ids.NewV7(), Name: "x", CertSerial: "1"}); !errors.Is(err, enroll.ErrCodeUsed) {
		t.Errorf("second use: %v", err)
	}
	if active, _ := s.ActiveCodes(ctx, now); len(active) != 0 {
		t.Error("used code is still active")
	}
	got, err := s.GetNode(ctx, node.ID)
	if err != nil || got.Name != "t420" || got.Status != StatusEnrolling || got.CertSerial != "ab12" {
		t.Errorf("node %+v, %v", got, err)
	}
}

func createNode(t *testing.T, s *Store) string {
	t.Helper()
	ctx := context.Background()
	codeID, _ := s.CreateJoinCode(ctx, pkgenroll.NewCode().Key(), time.Now().Add(time.Hour))
	id := ids.NewV7()
	if err := s.CompleteEnrollment(ctx, codeID, enroll.NewNode{ID: id, Name: "n", CertSerial: "1", CertNotAfter: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecordHelloAndStatus(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id := createNode(t, s)
	now := time.Now().UTC().Truncate(time.Millisecond)
	hello := proto.Hello{
		Protocol: 1, AgentVersion: "0.1.0", NodeID: id,
		OS: proto.OSInfo{Platform: "linux", Name: "Debian GNU/Linux 13 (trixie)", Hostname: "t420"},
		Hardware: proto.Hardware{Vendor: "LENOVO", NICs: []proto.NICInfo{
			{Name: "enp0s25", MAC: "f0:de:f1:00:00:01", Addrs: []string{"192.168.1.20/24", "fe80::1/64", "garbage"}},
		}},
	}
	if err := s.RecordHello(ctx, id, hello, now); err != nil {
		t.Fatal(err)
	}
	n, err := s.GetNode(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if n.Status != StatusOnline || n.Hostname != "t420" || n.AgentVersion != "0.1.0" || n.Hardware.Vendor != "LENOVO" {
		t.Errorf("node %+v", n)
	}
	if len(n.IPAddresses) != 2 || n.IPAddresses[0] != "192.168.1.20/24" || n.MACAddresses[0] != "f0:de:f1:00:00:01" {
		t.Errorf("addresses %v %v", n.IPAddresses, n.MACAddresses)
	}
	if n.LastSeenAt == nil || !n.LastSeenAt.Equal(now) {
		t.Errorf("last seen %v, want %v", n.LastSeenAt, now)
	}

	if err := s.SetNodeOffline(ctx, id, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.GetNode(ctx, id); n.Status != StatusOffline {
		t.Errorf("status %s after SetNodeOffline", n.Status)
	}
	if err := s.RecordHello(ctx, ids.NewV7(), hello, now); !errors.Is(err, ErrNotFound) {
		t.Errorf("hello of unknown node: %v", err)
	}
	if _, err := s.GetNode(ctx, "../etc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("invalid id: %v", err)
	}
}

func TestMarkAllOffline(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a, b := createNode(t, s), createNode(t, s)
	for _, id := range []string{a, b} {
		if err := s.RecordHello(ctx, id, proto.Hello{NodeID: id}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkAllOffline(ctx); err != nil {
		t.Fatal(err)
	}
	nodes, _ := s.ListNodes(ctx)
	if len(nodes) != 2 || nodes[0].Status != StatusOffline || nodes[1].Status != StatusOffline {
		t.Errorf("nodes %+v", nodes)
	}
}

func TestStatsInsertQueryAndRollup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id := createNode(t, s)
	base := time.Now().UTC().Truncate(5 * time.Minute).Add(-10 * time.Minute)
	if err := s.EnsurePartitions(ctx, base); err != nil {
		t.Fatal(err)
	}
	temp := 50.0
	for i := range 4 {
		st := proto.Stats{
			CPU:   proto.CPUStats{Pct: float64(10 * (i + 1)), TempC: &temp},
			Mem:   proto.MemStats{Total: 1000, Used: uint64(100 * (i + 1))},
			Disks: []proto.DiskStats{{Mount: "/", Total: 200, Used: 50}, {Mount: "/data", Total: 100, Used: 90}},
			Net:   []proto.NetStats{{Iface: "eth0", RxBps: 100, TxBps: 10}, {Iface: "wlan0", RxBps: 50}},
		}
		if err := s.InsertStats(ctx, id, base.Add(time.Duration(i)*time.Minute), st); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := s.QueryStats(ctx, id, base, base.Add(time.Hour), ResolutionRaw)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 || *raw[0].CPUPct != 10 || *raw[0].DiskUsedPct != 90 || *raw[0].NetRxBps != 150 || *raw[3].MemUsed != 400 {
		t.Fatalf("raw points %+v", raw[0])
	}
	if raw[0].GPUUtilPct != nil {
		t.Error("gpu util set without GPU")
	}

	if err := s.Rollup(ctx, base.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Rollup(ctx, base.Add(5*time.Minute)); err != nil { // idempotent
		t.Fatal(err)
	}
	agg, err := s.QueryStats(ctx, id, base.Add(-time.Hour), base.Add(time.Hour), ResolutionFiveMin)
	if err != nil {
		t.Fatal(err)
	}
	if len(agg) != 1 || !agg[0].TS.Equal(base) || *agg[0].CPUPct != 25 || *agg[0].MemUsed != 250 || *agg[0].MemTotal != 1000 {
		t.Fatalf("rollup %+v", agg)
	}
	if _, err := s.QueryStats(ctx, id, base, base, "1h"); err == nil {
		t.Error("unknown resolution accepted")
	}
}

func TestPartitionRetention(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	old := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := s.EnsurePartitions(ctx, old); err != nil {
		t.Fatal(err)
	}
	now := old.Add(4 * 24 * time.Hour)
	if err := s.EnsurePartitions(ctx, now); err != nil {
		t.Fatal(err)
	}
	dropped, err := s.DropOldPartitions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	// Raw keeps 48 h: the partitions for Dec 31 .. Jan 2 end before Jan 3 12:00 and go.
	want := []string{"node_stats_raw_p20251231", "node_stats_raw_p20260101", "node_stats_raw_p20260102"}
	if len(dropped) != len(want) {
		t.Fatalf("dropped %v, want %v", dropped, want)
	}
	for i := range want {
		if dropped[i] != want[i] {
			t.Errorf("dropped %v, want %v", dropped, want)
		}
	}
}
