package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/control/internal/testdb"
	"github.com/eronbubi/smartcluster/pkg/enroll"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// Smoke test of the wiring: the control center starts, an agent enrolls on
// the agent port, and the node shows up in the API over verified TLS.
func TestRun(t *testing.T) {
	dbURL := testdb.New(t)
	agentAddr := "127.0.0.1:" + strconv.Itoa(freePort(t))
	apiAddr := "127.0.0.1:" + strconv.Itoa(freePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			DatabaseURL: dbURL, DataDir: t.TempDir(), AgentAddr: agentAddr, APIAddr: apiAddr, ClusterName: "test",
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	}()

	// Fetch the CA once without verification, then use it for everything else.
	insecure := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // bootstrap of the test
	var caPEM []byte
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := insecure.Get("https://" + apiAddr + "/api/v1/ca.crt")
		if err == nil {
			caPEM, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("control center did not start: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	block, _ := pem.Decode(caPEM)
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}

	conn, err := db.Open(context.Background(), dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	code := enroll.NewCode()
	if _, err := store.New(conn, nil).CreateJoinCode(context.Background(), code.Key(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	id, err := enroll.Enroll(context.Background(), agentAddr, code, "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if !id.CA.Equal(ca) {
		t.Error("agent and API see different CAs")
	}

	resp, err := client.Get("https://" + apiAddr + "/api/v1/nodes")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Nodes []struct{ ID, Name string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Nodes) != 1 || list.Nodes[0].ID != id.NodeID || list.Nodes[0].Name != "smoke" {
		t.Errorf("nodes %+v", list.Nodes)
	}
}

func TestServerHostsRejectPublicNames(t *testing.T) {
	if _, err := serverHosts([]string{"example.com"}); err == nil {
		t.Error("public name accepted")
	}
	hosts, err := serverHosts([]string{"10.0.0.7", "nas.local"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"localhost", "127.0.0.1", "10.0.0.7", "nas.local"} {
		found := false
		for _, h := range hosts {
			found = found || h == want
		}
		if !found {
			t.Errorf("%s missing in %v", want, hosts)
		}
	}
}
