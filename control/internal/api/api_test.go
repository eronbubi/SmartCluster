package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/control/internal/enroll"
	"github.com/eronbubi/smartcluster/control/internal/secrets"
	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/control/internal/testdb"
	pkgenroll "github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type fakeLive map[string]proto.Stats

func (f fakeLive) Latest(id string) (proto.Stats, bool) { s, ok := f[id]; return s, ok }

func setup(t *testing.T) (*httptest.Server, *store.Store, string) {
	t.Helper()
	ctx := context.Background()
	pool, err := db.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	box, _ := secrets.NewBox(make([]byte, 32))
	st := store.New(pool, box)
	if err := st.EnsurePartitions(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	codeID, _ := st.CreateJoinCode(ctx, pkgenroll.NewCode().Key(), time.Now().Add(time.Hour))
	nodeID := ids.NewV7()
	if err := st.CompleteEnrollment(ctx, codeID, enroll.NewNode{ID: nodeID, Name: "t420", CertSerial: "1", CertNotAfter: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertStats(ctx, nodeID, time.Now().Add(-10*time.Minute), proto.Stats{CPU: proto.CPUStats{Pct: 33}}); err != nil {
		t.Fatal(err)
	}
	a := &API{
		Store: st, Live: fakeLive{nodeID: {CPU: proto.CPUStats{Pct: 7}}},
		Events:  http.NotFoundHandler(),
		Cluster: Cluster{ID: "c", Name: "home", AgentPort: 8444},
		CAPEM:   []byte("-----BEGIN CERTIFICATE-----\n"),
		Version: "1.2.3", Now: time.Now, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	return srv, st, nodeID
}

func get(t *testing.T, url string, v any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			t.Fatalf("decode %s: %v", url, err)
		}
	}
	return resp.StatusCode
}

func TestNodes(t *testing.T) {
	srv, _, nodeID := setup(t)
	var list struct{ Nodes []Node }
	if code := get(t, srv.URL+"/api/v1/nodes", &list); code != 200 || len(list.Nodes) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	n := list.Nodes[0]
	if n.ID != nodeID || n.Name != "t420" || n.Status != "enrolling" || n.Latest == nil || n.Latest.CPU.Pct != 7 {
		t.Errorf("node %+v", n)
	}
	if n.IPAddresses == nil || n.MACAddresses == nil {
		t.Error("address lists must be [] not null")
	}

	var one Node
	if code := get(t, srv.URL+"/api/v1/nodes/"+nodeID, &one); code != 200 || one.ID != nodeID {
		t.Errorf("detail: %d %+v", code, one)
	}
	var e ErrorBody
	if code := get(t, srv.URL+"/api/v1/nodes/"+ids.NewV7(), &e); code != 404 || e.Error.Code != "node_not_found" {
		t.Errorf("unknown node: %d %+v", code, e)
	}
	if code := get(t, srv.URL+"/api/v1/nodes/not-a-uuid", &e); code != 404 {
		t.Errorf("invalid id: %d", code)
	}
}

func TestNodeStats(t *testing.T) {
	srv, _, nodeID := setup(t)
	var res StatsResponse
	if code := get(t, srv.URL+"/api/v1/nodes/"+nodeID+"/stats", &res); code != 200 {
		t.Fatalf("status %d", code)
	}
	if res.Resolution != "raw" || len(res.Points) != 1 || *res.Points[0].CPUPct != 33 || res.Points[0].GPUUtilPct != nil {
		t.Errorf("stats %+v", res)
	}

	from := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if code := get(t, srv.URL+"/api/v1/nodes/"+nodeID+"/stats?from="+from, &res); code != 200 || res.Resolution != "5m" {
		t.Errorf("7 days: %d %s", code, res.Resolution)
	}

	var e ErrorBody
	for _, q := range []string{"?from=yesterday", "?res=1h", "?from=2026-01-01T00:00:00Z&to=2025-01-01T00:00:00Z", "?from=2020-01-01T00:00:00Z"} {
		if code := get(t, srv.URL+"/api/v1/nodes/"+nodeID+"/stats"+q, &e); code != 400 {
			t.Errorf("%s: status %d", q, code)
		}
	}
}

func TestMiscEndpoints(t *testing.T) {
	srv, _, _ := setup(t)
	var v map[string]any
	if code := get(t, srv.URL+"/api/v1/version", &v); code != 200 || v["version"] != "1.2.3" {
		t.Errorf("version %d %v", code, v)
	}
	if code := get(t, srv.URL+"/api/v1/readyz", nil); code != 200 {
		t.Errorf("readyz %d", code)
	}
	var c Cluster
	if code := get(t, srv.URL+"/api/v1/cluster", &c); code != 200 || c.Name != "home" {
		t.Errorf("cluster %d %+v", code, c)
	}
	var e ErrorBody
	if code := get(t, srv.URL+"/api/v1/nope", &e); code != 404 || e.Error.Code != "not_found" {
		t.Errorf("unknown endpoint %d %+v", code, e)
	}
	resp, err := http.Post(srv.URL+"/api/v1/nodes", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Errorf("POST /nodes: %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers missing")
	}
}
