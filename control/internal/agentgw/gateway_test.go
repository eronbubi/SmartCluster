package agentgw

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/control/internal/db"
	controlenroll "github.com/eronbubi/smartcluster/control/internal/enroll"
	"github.com/eronbubi/smartcluster/control/internal/live"
	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/control/internal/secrets"
	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/control/internal/testdb"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type event struct {
	topic, typ string
	data       any
}

type recorder struct {
	mu     sync.Mutex
	events []event
}

func (r *recorder) Publish(topic, typ string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event{topic, typ, data})
}

func (r *recorder) has(topic, typ string, match func(any) bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.topic == topic && e.typ == typ && match(e.data) {
			return true
		}
	}
	return false
}

type env struct {
	t      *testing.T
	store  *store.Store
	gw     *Gateway
	events *recorder
	ca     *pki.CA
	cmdPub ed25519.PublicKey
	addr   string
	mu     sync.Mutex
	now    time.Time
}

func (e *env) clock() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.now.IsZero() {
		return time.Now()
	}
	return e.now
}

func newEnv(t *testing.T) *env {
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

	ca, _ := pki.NewCA("test", time.Now())
	serverCert, err := ca.IssueServer([]string{"127.0.0.1"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cmdPub, cmdKey, _ := ed25519.GenerateKey(nil)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &env{t: t, store: st, events: &recorder{}, ca: ca, cmdPub: cmdPub}
	cmdKeys := []proto.PubKey{{KeyID: "cmd-1", Key: sign.EncodeKey(cmdPub)}}
	e.gw = &Gateway{Store: st, Hub: e.events, Signer: sign.NewSigner("cmd-1", cmdKey), CmdKeys: cmdKeys, Issuer: ca, Now: e.clock, Log: logger}

	mux := http.NewServeMux()
	mux.Handle(enroll.Path, &controlenroll.Handler{
		Store: st, Issuer: ca, CACert: ca.Cert,
		ServerCert: func() *x509.Certificate { return serverCert.Leaf },
		CmdKeys:    func() []proto.PubKey { return cmdKeys },
		Now:        time.Now, Log: logger,
	})
	mux.Handle(WSPath, e.gw)
	mux.HandleFunc(enroll.RenewPath, e.gw.ServeRenew)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS13,
		ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: ca.Pool(),
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	e.addr = srv.Listener.Addr().String()
	return e
}

// join enrolls a new node and returns its TLS identity.
func (e *env) join() (*enroll.Identity, tls.Certificate) {
	e.t.Helper()
	code := enroll.NewCode()
	if _, err := e.store.CreateJoinCode(context.Background(), code.Key(), time.Now().Add(time.Hour)); err != nil {
		e.t.Fatal(err)
	}
	id, err := enroll.Enroll(context.Background(), e.addr, code, "t420")
	if err != nil {
		e.t.Fatal(err)
	}
	return id, tls.Certificate{Certificate: [][]byte{id.Cert.Raw}, PrivateKey: id.Key}
}

func (e *env) client(cert *tls.Certificate) *http.Client {
	cfg := &tls.Config{RootCAs: e.ca.Pool(), MinVersion: tls.VersionTLS13}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func (e *env) dial(cert tls.Certificate) *websocket.Conn {
	e.t.Helper()
	c, _, err := websocket.Dial(context.Background(), "wss://"+e.addr+WSPath, &websocket.DialOptions{ //nolint:bodyclose // websocket.Dial owns the body: "You never need to close resp.Body yourself."
		HTTPClient: e.client(&cert), Subprotocols: []string{proto.Subprotocol},
	})
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

func send(t *testing.T, c *websocket.Conn, typ string, data any) {
	t.Helper()
	frame, err := proto.Encode(typ, "", time.Now(), data)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Write(context.Background(), websocket.MessageText, frame); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, c *websocket.Conn) (proto.Envelope, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		return proto.Envelope{}, err
	}
	return proto.Decode(data)
}

func hello(nodeID string) proto.Hello {
	return proto.Hello{Protocol: proto.Version, AgentVersion: "0.1.0", NodeID: nodeID,
		OS: proto.OSInfo{Platform: "linux", Hostname: "t420"}, Capabilities: []string{}}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAgentSession(t *testing.T) {
	e := newEnv(t)
	id, cert := e.join()
	c := e.dial(cert)

	send(t, c, proto.TypeHello, hello(id.NodeID))
	env, err := read(t, c)
	if err != nil || env.Type != proto.TypeHelloOK {
		t.Fatalf("got %v %v, want hello.ok", env.Type, err)
	}
	var ok proto.HelloOK
	_ = env.Unmarshal(&ok)
	if ok.StatsIntervalS != 5 || len(ok.CmdPubKeys) != 1 || time.Since(ok.ServerTime) > time.Minute {
		t.Errorf("hello.ok %+v", ok)
	}
	node, _ := e.store.GetNode(context.Background(), id.NodeID)
	if node.Status != store.StatusOnline || node.AgentVersion != "0.1.0" {
		t.Errorf("node after hello %+v", node)
	}

	send(t, c, proto.TypeStats, proto.Stats{CPU: proto.CPUStats{Pct: 42}, Mem: proto.MemStats{Total: 100, Used: 50}})
	eventually(t, "stats in the database", func() bool {
		pts, _ := e.store.QueryStats(context.Background(), id.NodeID, time.Now().Add(-time.Minute), time.Now().Add(time.Minute), store.ResolutionRaw)
		return len(pts) == 1 && *pts[0].CPUPct == 42
	})
	if latest, ok := e.gw.Latest(id.NodeID); !ok || latest.CPU.Pct != 42 {
		t.Errorf("latest %+v", latest)
	}
	isNode := func(d any) bool { ns, ok := d.(NodeStats); return ok && ns.NodeID == id.NodeID }
	if !e.events.has(live.TopicStats, live.TypeNodeStats, isNode) || !e.events.has(live.TopicNodeStats(id.NodeID), live.TypeNodeStats, isNode) {
		t.Error("stats were not published")
	}

	c.Close(websocket.StatusNormalClosure, "")
	eventually(t, "node offline", func() bool {
		n, _ := e.store.GetNode(context.Background(), id.NodeID)
		return n.Status == store.StatusOffline
	})
	if !e.events.has(live.TopicNodes, live.TypeNodeStatus, func(d any) bool {
		s, ok := d.(NodeStatus)
		return ok && s.NodeID == id.NodeID && s.Status == store.StatusOffline
	}) {
		t.Error("offline status was not published")
	}
}

func TestWebSocketRequiresClientCertificate(t *testing.T) {
	e := newEnv(t)
	_, resp, err := websocket.Dial(context.Background(), "wss://"+e.addr+WSPath, &websocket.DialOptions{ //nolint:bodyclose // websocket.Dial owns the body: "You never need to close resp.Body yourself."
		HTTPClient: e.client(nil), Subprotocols: []string{proto.Subprotocol},
	})
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %v, %+v; want 401", err, resp)
	}
}

func TestHelloMustMatchProtocolAndCertificate(t *testing.T) {
	e := newEnv(t)
	id, cert := e.join()

	c := e.dial(cert)
	h := hello(id.NodeID)
	h.Protocol = 2
	send(t, c, proto.TypeHello, h)
	if _, err := read(t, c); websocket.CloseStatus(err) != proto.CloseProtocolUnsupported {
		t.Errorf("protocol 2: %v", err)
	}

	c = e.dial(cert)
	other, _ := e.join()
	send(t, c, proto.TypeHello, hello(other.NodeID))
	if _, err := read(t, c); websocket.CloseStatus(err) != proto.CloseAuthFailed {
		t.Errorf("foreign node id: %v", err)
	}

	c = e.dial(cert)
	send(t, c, proto.TypeStats, proto.Stats{})
	if _, err := read(t, c); websocket.CloseStatus(err) != proto.CloseProtocolUnsupported {
		t.Errorf("stats before hello: %v", err)
	}
}

func TestNewConnectionSupersedesOld(t *testing.T) {
	e := newEnv(t)
	id, cert := e.join()
	first := e.dial(cert)
	send(t, first, proto.TypeHello, hello(id.NodeID))
	if _, err := read(t, first); err != nil {
		t.Fatal(err)
	}

	second := e.dial(cert)
	send(t, second, proto.TypeHello, hello(id.NodeID))
	if _, err := read(t, second); err != nil {
		t.Fatal(err)
	}
	if _, err := read(t, first); websocket.CloseStatus(err) != proto.CloseSuperseded {
		t.Errorf("old connection: %v, want close %d", err, proto.CloseSuperseded)
	}
	time.Sleep(100 * time.Millisecond)
	if n, _ := e.store.GetNode(context.Background(), id.NodeID); n.Status != store.StatusOnline {
		t.Errorf("node is %s after the old connection closed, want online", n.Status)
	}
}

// A certificate close to expiry triggers a signed cert.renew command; the
// agent renews over mTLS and the new certificate is recorded.
func TestCertificateRenewal(t *testing.T) {
	e := newEnv(t)
	id, cert := e.join()
	e.mu.Lock()
	e.now = id.Cert.NotAfter.Add(-10 * 24 * time.Hour)
	e.mu.Unlock()

	c := e.dial(cert)
	send(t, c, proto.TypeHello, hello(id.NodeID))
	if env, err := read(t, c); err != nil || env.Type != proto.TypeHelloOK {
		t.Fatalf("hello.ok: %v %v", env.Type, err)
	}
	env, err := read(t, c)
	if err != nil || env.Type != proto.TypeCmd {
		t.Fatalf("got %v %v, want cmd", env.Type, err)
	}
	verifier := sign.NewVerifier(id.NodeID, map[string]ed25519.PublicKey{"cmd-1": e.cmdPub}, time.Now)
	cmd, err := verifier.Verify(env.Data)
	if err != nil {
		t.Fatalf("command does not verify: %v", err)
	}
	if cmd.Type != proto.CmdCertRenew {
		t.Fatalf("command type %s", cmd.Type)
	}

	newKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	csr, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, newKey)
	body, _ := json.Marshal(enroll.RenewRequest{CSR: csr})
	resp, err := e.client(&cert).Post("https://"+e.addr+enroll.RenewPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var renewed enroll.RenewResponse
	if err := json.NewDecoder(resp.Body).Decode(&renewed); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("renew: %d %v", resp.StatusCode, err)
	}
	newCert, _ := x509.ParseCertificate(renewed.ClientCert)
	if newCert.Subject.CommonName != id.NodeID || !newCert.PublicKey.(*ecdsa.PublicKey).Equal(&newKey.PublicKey) {
		t.Error("renewed certificate is wrong")
	}
	node, _ := e.store.GetNode(context.Background(), id.NodeID)
	if node.CertSerial != newCert.SerialNumber.Text(16) {
		t.Error("new serial not recorded")
	}

	unauth, err := e.client(nil).Post("https://"+e.addr+enroll.RenewPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	unauth.Body.Close()
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Errorf("renew without certificate: %d", unauth.StatusCode)
	}
}

func TestSendCommandToOfflineNode(t *testing.T) {
	e := newEnv(t)
	if _, err := e.gw.SendCommand(context.Background(), "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", proto.CmdCertRenew, nil); !errors.Is(err, ErrOffline) {
		t.Errorf("got %v, want ErrOffline", err)
	}
}
