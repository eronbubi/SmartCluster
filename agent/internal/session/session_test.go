package session

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/agent/internal/state"
	"github.com/eronbubi/smartcluster/agent/internal/stats"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
	"github.com/eronbubi/smartcluster/pkg/testenv"
)

const nodeID = "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e"

// fakeControl is a minimal control center: TLS with client certificates,
// the agent WebSocket and the renewal endpoint.
type fakeControl struct {
	t        *testing.T
	ca       *x509.Certificate
	caKey    *ecdsa.PrivateKey
	addr     string
	signer   *sign.Signer
	cmdPub   ed25519.PublicKey
	sessions chan *websocket.Conn
	renewed  chan *x509.Certificate
}

func newCert(t *testing.T, tmpl *x509.Certificate, parent *x509.Certificate, pub any, parentKey *ecdsa.PrivateKey) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return c
}

func startControl(t *testing.T) *fakeControl {
	t.Helper()
	testenv.RequireServers(t)
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	ca := newCert(t, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	srvKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	srvCert := newCert(t, &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}, ca, &srvKey.PublicKey, caKey)

	cmdPub, cmdKey, _ := ed25519.GenerateKey(nil)
	f := &fakeControl{t: t, ca: ca, caKey: caKey, signer: sign.NewSigner("cmd-1", cmdKey), cmdPub: cmdPub,
		sessions: make(chan *websocket.Conn, 4), renewed: make(chan *x509.Certificate, 1)}

	mux := http.NewServeMux()
	mux.HandleFunc("/agent/v1/ws", func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{proto.Subprotocol}})
		if err != nil {
			return
		}
		f.sessions <- ws // the connection is hijacked and stays open after return
	})
	mux.HandleFunc(enroll.RenewPath, func(w http.ResponseWriter, r *http.Request) {
		var req enroll.RenewRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		csr, err := x509.ParseCertificateRequest(req.CSR)
		if err != nil || len(r.TLS.VerifiedChains) == 0 {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		cert := f.clientCert(csr.PublicKey)
		f.renewed <- cert
		_ = json.NewEncoder(w).Encode(enroll.RenewResponse{ClientCert: cert.Raw})
	})
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{srvCert.Raw}, PrivateKey: srvKey}},
		ClientAuth:   tls.RequireAndVerifyClientCert, ClientCAs: pool,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	f.addr = srv.Listener.Addr().String()
	return f
}

func (f *fakeControl) clientCert(pub any) *x509.Certificate {
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	return newCert(f.t, &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: nodeID},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}, f.ca, pub, f.caKey)
}

// enroll writes an agent state for this control center.
func (f *fakeControl) enroll(t *testing.T) string {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	dir := filepath.Join(t.TempDir(), "agent")
	id := &enroll.Identity{NodeID: nodeID, Key: key, Cert: f.clientCert(&key.PublicKey), CA: f.ca,
		CmdKeys: []proto.PubKey{{KeyID: "cmd-1", Key: sign.EncodeKey(f.cmdPub)}}}
	if err := state.Save(dir, f.addr, id); err != nil {
		t.Fatal(err)
	}
	return dir
}

func (f *fakeControl) accept() *websocket.Conn {
	f.t.Helper()
	select {
	case ws := <-f.sessions:
		return ws
	case <-time.After(5 * time.Second):
		f.t.Fatal("agent did not connect")
		return nil
	}
}

func (f *fakeControl) expect(ws *websocket.Conn, typ string, v any) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			f.t.Fatalf("waiting for %s: %v", typ, err)
		}
		env, err := proto.Decode(data)
		if err != nil {
			f.t.Fatal(err)
		}
		if env.Type == proto.TypeStats && typ != proto.TypeStats {
			continue // stats keep flowing while we wait for something else
		}
		if env.Type != typ {
			f.t.Fatalf("got %s, want %s", env.Type, typ)
		}
		if v != nil {
			if err := env.Unmarshal(v); err != nil {
				f.t.Fatal(err)
			}
		}
		return
	}
}

func (f *fakeControl) send(ws *websocket.Conn, typ string, data any) {
	f.t.Helper()
	frame, _ := proto.Encode(typ, "", time.Now(), data)
	if err := ws.Write(context.Background(), websocket.MessageText, frame); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeControl) handshake(ws *websocket.Conn) proto.Hello {
	f.t.Helper()
	var h proto.Hello
	f.expect(ws, proto.TypeHello, &h)
	f.send(ws, proto.TypeHelloOK, proto.HelloOK{Protocol: proto.Version, ServerTime: time.Now(), StatsIntervalS: 1,
		CmdPubKeys: []proto.PubKey{{KeyID: "cmd-1", Key: sign.EncodeKey(f.cmdPub)}}})
	return h
}

type quietReader struct{}

func (quietReader) CPUTimes(context.Context) ([]stats.CPUTimes, error) {
	return []stats.CPUTimes{{Busy: 1, Total: 2}}, nil
}
func (quietReader) Load1(context.Context) (*float64, error)      { return nil, nil }
func (quietReader) CPUFreqMHz(context.Context) (*float64, error) { return nil, nil }
func (quietReader) Memory(context.Context) (proto.MemStats, error) {
	return proto.MemStats{Total: 8}, nil
}
func (quietReader) Filesystems(context.Context) ([]stats.Filesystem, error) {
	return nil, nil
}
func (quietReader) DiskIO(context.Context) (map[string]stats.IOCounter, error) { return nil, nil }
func (quietReader) Network(context.Context) ([]stats.NetCounter, error)        { return nil, nil }
func (quietReader) Temperatures(context.Context) ([]proto.TempStat, error) {
	return nil, errors.New("no sensors")
}
func (quietReader) GPUs(context.Context) ([]proto.GPUStats, error)  { return nil, nil }
func (quietReader) Battery(context.Context) (*proto.Battery, error) { return nil, nil }
func (quietReader) Uptime(context.Context) (uint64, error)          { return 42, nil }

func runAgent(t *testing.T, dir string) context.CancelFunc {
	t.Helper()
	r := &Runner{
		StateDir: dir, Version: "0.1.0-test", Collector: stats.New(quietReader{}),
		Inventory: func(context.Context) (proto.OSInfo, proto.Hardware, []error) {
			return proto.OSInfo{Platform: "linux", Hostname: "t420"}, proto.Hardware{}, nil
		},
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		MinBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	return cancel
}

func TestHelloAndStats(t *testing.T) {
	f := startControl(t)
	runAgent(t, f.enroll(t))
	ws := f.accept()
	h := f.handshake(ws)
	if h.NodeID != nodeID || h.AgentVersion != "0.1.0-test" || h.Protocol != proto.Version || h.OS.Hostname != "t420" {
		t.Errorf("hello %+v", h)
	}
	var s proto.Stats
	f.expect(ws, proto.TypeStats, &s)
	if s.UptimeS != 42 || s.Mem.Total != 8 {
		t.Errorf("stats %+v", s)
	}
}

func TestCommands(t *testing.T) {
	f := startControl(t)
	dir := f.enroll(t)
	runAgent(t, dir)
	ws := f.accept()
	f.handshake(ws)

	unknown, _ := f.signer.Sign(nodeID, "shell.exec", map[string]string{"cmd": "rm -rf /"})
	f.send(ws, proto.TypeCmd, unknown)
	var rej proto.CmdReject
	f.expect(ws, proto.TypeCmdReject, &rej)
	if rej.CmdID != unknown.CmdID || rej.Code != proto.RejectUnknownType {
		t.Errorf("unknown type: %+v", rej)
	}

	tampered, _ := f.signer.Sign(nodeID, proto.CmdCertRenew, nil)
	tampered.Payload = json.RawMessage(`{"extra":1}`)
	f.send(ws, proto.TypeCmd, tampered)
	f.expect(ws, proto.TypeCmdReject, &rej)
	if rej.Code != proto.RejectBadSignature {
		t.Errorf("tampered: %+v", rej)
	}

	before, _ := state.Load(dir)
	renew, _ := f.signer.Sign(nodeID, proto.CmdCertRenew, nil)
	f.send(ws, proto.TypeCmd, renew)
	var ack proto.CmdAck
	f.expect(ws, proto.TypeCmdAck, &ack)
	var res proto.CmdResult
	f.expect(ws, proto.TypeCmdResult, &res)
	if !res.OK || res.CmdID != renew.CmdID {
		t.Fatalf("renew result %+v", res)
	}
	issued := <-f.renewed
	after, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if string(after.Cert.Certificate[0]) != string(issued.Raw) || string(after.Cert.Certificate[0]) == string(before.Cert.Certificate[0]) {
		t.Error("renewed certificate was not stored")
	}

	f.send(ws, proto.TypeCmd, renew) // replay
	f.expect(ws, proto.TypeCmdReject, &rej)
	if rej.Code != proto.RejectReplay {
		t.Errorf("replay: %+v", rej)
	}
}

func TestReconnects(t *testing.T) {
	f := startControl(t)
	runAgent(t, f.enroll(t))
	first := f.accept()
	f.handshake(first)
	first.Close(websocket.StatusGoingAway, "restart")

	second := f.accept()
	if h := f.handshake(second); h.NodeID != nodeID {
		t.Errorf("second hello %+v", h)
	}
}

func TestRunRequiresEnrollment(t *testing.T) {
	r := &Runner{StateDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := r.Run(context.Background()); !errors.Is(err, state.ErrNotEnrolled) {
		t.Errorf("got %v, want ErrNotEnrolled", err)
	}
}
