package enroll

import (
	"bytes"
	"context"
	"crypto/ed25519"
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

	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
	"github.com/eronbubi/smartcluster/pkg/testenv"
)

type memStore struct {
	mu    sync.Mutex
	codes map[string]*memCode
	nodes []NewNode
}

type memCode struct {
	Code
	used bool
}

func (s *memStore) add(id string, c enroll.Code, expires time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codes == nil {
		s.codes = map[string]*memCode{}
	}
	s.codes[id] = &memCode{Code: Code{ID: id, Key: c.Key(), ExpiresAt: expires}}
}

func (s *memStore) ActiveCodes(_ context.Context, now time.Time) ([]Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Code
	for _, c := range s.codes {
		if !c.used && now.Before(c.ExpiresAt) {
			out = append(out, c.Code)
		}
	}
	return out, nil
}

func (s *memStore) CompleteEnrollment(_ context.Context, codeID string, n NewNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.codes[codeID]
	if c == nil || c.used {
		return ErrCodeUsed
	}
	c.used = true
	s.nodes = append(s.nodes, n)
	return nil
}

type env struct {
	store  *memStore
	ca     *pki.CA
	cmdPub ed25519.PublicKey
	addr   string // host:port of the TLS agent listener
	now    time.Time
}

// startServer runs the enroll handler behind a TLS listener on 127.0.0.1,
// like the control center's agent port.
func startServer(t *testing.T) *env {
	t.Helper()
	testenv.RequireServers(t)
	now := time.Now()
	ca, err := pki.NewCA("test", now)
	if err != nil {
		t.Fatal(err)
	}
	serverCert, err := ca.IssueServer([]string{"127.0.0.1", "localhost"}, now)
	if err != nil {
		t.Fatal(err)
	}
	cmdPub, _, _ := ed25519.GenerateKey(nil)
	e := &env{store: &memStore{}, ca: ca, cmdPub: cmdPub, now: now}
	h := &Handler{
		Store:      e.store,
		Issuer:     ca,
		CACert:     ca.Cert,
		ServerCert: func() *x509.Certificate { return serverCert.Leaf },
		CmdKeys:    func() []proto.PubKey { return []proto.PubKey{{KeyID: "cmd-1", Key: sign.EncodeKey(cmdPub)}} },
		Now:        func() time.Time { return e.now },
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	mux := http.NewServeMux()
	mux.Handle(enroll.Path, h)
	srv := httptest.NewUnstartedServer(mux)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS13}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	e.addr = srv.Listener.Addr().String()
	return e
}

func TestEnrollSucceeds(t *testing.T) {
	e := startServer(t)
	code := enroll.NewCode()
	e.store.add("code-1", code, e.now.Add(30*time.Minute))

	id, err := enroll.Enroll(context.Background(), e.addr, code, "Lenovo T420!")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.store.nodes) != 1 || e.store.nodes[0].ID != id.NodeID {
		t.Fatalf("stored nodes %+v, identity %s", e.store.nodes, id.NodeID)
	}
	if got := e.store.nodes[0].Name; got != "LenovoT420" {
		t.Errorf("node name %q", got)
	}
	if !id.CA.Equal(e.ca.Cert) || id.Cert.Subject.CommonName != id.NodeID {
		t.Error("identity does not match the cluster")
	}
	if len(id.CmdKeys) != 1 || id.CmdKeys[0].Key != sign.EncodeKey(e.cmdPub) {
		t.Errorf("command keys %+v", id.CmdKeys)
	}
}

func TestEnrollRefusesWrongExpiredAndUsedCodes(t *testing.T) {
	e := startServer(t)
	valid := enroll.NewCode()
	expired := enroll.NewCode()
	e.store.add("valid", valid, e.now.Add(30*time.Minute))
	e.store.add("expired", expired, e.now.Add(-time.Second))

	ctx := context.Background()
	if _, err := enroll.Enroll(ctx, e.addr, enroll.NewCode(), "x"); !errors.Is(err, enroll.ErrRefused) {
		t.Errorf("unknown code: %v", err)
	}
	if _, err := enroll.Enroll(ctx, e.addr, expired, "x"); !errors.Is(err, enroll.ErrRefused) {
		t.Errorf("expired code: %v", err)
	}
	if _, err := enroll.Enroll(ctx, e.addr, valid, "x"); err != nil {
		t.Fatalf("valid code: %v", err)
	}
	if _, err := enroll.Enroll(ctx, e.addr, valid, "x"); !errors.Is(err, enroll.ErrRefused) {
		t.Errorf("reused code: %v", err)
	}
	if len(e.store.nodes) != 1 {
		t.Errorf("%d nodes stored, want 1", len(e.store.nodes))
	}
}

// A man in the middle terminates TLS with its own certificate and relays the
// request. The agent's proof is bound to the MITM certificate, so the real
// control center refuses, and the code stays unused.
func TestEnrollDetectsManInTheMiddle(t *testing.T) {
	e := startServer(t)
	code := enroll.NewCode()
	e.store.add("code-1", code, e.now.Add(30*time.Minute))

	mitmCA, _ := pki.NewCA("mitm", e.now)
	mitmCert, _ := mitmCA.IssueServer([]string{"127.0.0.1"}, e.now)
	relay := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // test relay
		resp, err := client.Post("https://"+e.addr+enroll.Path, "application/json", bytes.NewReader(body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	relay.TLS = &tls.Config{Certificates: []tls.Certificate{mitmCert}}
	relay.StartTLS()
	defer relay.Close()

	_, err := enroll.Enroll(context.Background(), relay.Listener.Addr().String(), code, "x")
	if !errors.Is(err, enroll.ErrRefused) {
		t.Fatalf("got %v, want ErrRefused", err)
	}
	if len(e.store.nodes) != 0 {
		t.Error("a node was created through the relay")
	}
	if codes, _ := e.store.ActiveCodes(context.Background(), e.now); len(codes) != 1 {
		t.Error("the code was consumed by the relayed attempt")
	}
	// The real control center still accepts the code afterwards.
	if _, err := enroll.Enroll(context.Background(), e.addr, code, "x"); err != nil {
		t.Errorf("direct enrollment after MITM attempt: %v", err)
	}
}

// A fake control center that does not know the code cannot produce the
// server proof, whatever it answers.
func TestEnrollRejectsServerWithoutCode(t *testing.T) {
	testenv.RequireServers(t)
	fakeCA, _ := pki.NewCA("fake", time.Now())
	fakeCert, _ := fakeCA.IssueServer([]string{"127.0.0.1"}, time.Now())
	fake := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req enroll.Request
		_ = jsonDecode(r.Body, &req)
		cert, _ := fakeCA.SignClientCSR(req.CSR, "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", time.Now())
		writeJSON(w, http.StatusOK, enroll.Response{
			NodeID: "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e", ClientCert: cert.Raw, CACert: fakeCA.Cert.Raw,
			Proof: enroll.ServerProof(enroll.NewCode().Key(), fakeCert.Leaf.Raw, cert.Raw, req.Nonce),
		})
	}))
	fake.TLS = &tls.Config{Certificates: []tls.Certificate{fakeCert}}
	fake.StartTLS()
	defer fake.Close()

	_, err := enroll.Enroll(context.Background(), fake.Listener.Addr().String(), enroll.NewCode(), "x")
	if !errors.Is(err, enroll.ErrServerProof) {
		t.Fatalf("got %v, want ErrServerProof", err)
	}
}

func TestEnrollRateLimit(t *testing.T) {
	e := startServer(t)
	var last error
	for range limitAttempts + 1 {
		_, last = enroll.Enroll(context.Background(), e.addr, enroll.NewCode(), "x")
	}
	if last == nil || errors.Is(last, enroll.ErrRefused) {
		t.Errorf("attempt %d was not rate limited: %v", limitAttempts+1, last)
	}
}

func TestLimiterWindow(t *testing.T) {
	var l limiter
	t0 := time.Now()
	for range limitAttempts {
		if !l.allow("1.2.3.4", t0) {
			t.Fatal("blocked too early")
		}
	}
	if l.allow("1.2.3.4", t0) {
		t.Error("not blocked after limit")
	}
	if !l.allow("5.6.7.8", t0) {
		t.Error("other address blocked")
	}
	if !l.allow("1.2.3.4", t0.Add(limitWindow)) {
		t.Error("still blocked after the window")
	}
}

func TestNodeName(t *testing.T) {
	id := "0192f1c4-7e2a-7b3c-9d4e-5f6a7b8c9d0e"
	for in, want := range map[string]string{
		"thinkpad-t420":          "thinkpad-t420",
		"DESKTOP-4G7H2K":         "DESKTOP-4G7H2K",
		"<script>alert</script>": "scriptalertscript",
		"":                       "node-8c9d0e",
		"   ":                    "node-8c9d0e",
	} {
		if got := nodeName(in, id); got != want {
			t.Errorf("nodeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnrollRejectsNonPost(t *testing.T) {
	e := startServer(t)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: e.ca.Pool()}}}
	resp, err := client.Get("https://" + e.addr + enroll.Path)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status %d", resp.StatusCode)
	}
}

func jsonDecode(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}
