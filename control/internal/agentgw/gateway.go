// Package agentgw is the control center side of the agent WebSocket
// (ARCHITECTURE.md 8.2): authentication by client certificate, hello,
// telemetry, signed commands and certificate renewal.
package agentgw

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/control/internal/live"
	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
)

// WSPath is the agent WebSocket endpoint.
const WSPath = "/agent/v1/ws"

const (
	statsInterval   = 5 * time.Second
	minStatsSpacing = time.Second
	helloTimeout    = 10 * time.Second
	idleTimeout     = 60 * time.Second // ARCHITECTURE.md 8.1: offline after 60 s without a frame
	pingInterval    = 20 * time.Second
	writeTimeout    = 10 * time.Second
	maxRenewBytes   = 16 << 10
	// minAgentVersion becomes meaningful once agents update themselves (milestone g).
	minAgentVersion = "0.0.0"
)

// ErrOffline is returned when a command is sent to a node without connection.
var ErrOffline = errors.New("agentgw: node is not connected")

// Store is the persistence the gateway needs.
type Store interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)
	RecordHello(ctx context.Context, id string, h proto.Hello, now time.Time) error
	InsertStats(ctx context.Context, nodeID string, received time.Time, st proto.Stats) error
	SetNodeOffline(ctx context.Context, id string, at time.Time) error
	UpdateNodeCert(ctx context.Context, id, serial string, notAfter time.Time) error
}

// Publisher forwards events to the app.
type Publisher interface {
	Publish(topic, typ string, data any)
}

// Issuer signs renewed agent certificates.
type Issuer interface {
	SignClientCSR(csrDER []byte, nodeID string, now time.Time) (*x509.Certificate, error)
}

// Gateway serves agent connections.
type Gateway struct {
	Store   Store
	Hub     Publisher
	Signer  *sign.Signer
	CmdKeys []proto.PubKey
	Issuer  Issuer
	Now     func() time.Time
	Log     *slog.Logger

	mu     sync.Mutex
	conns  map[string]*agentConn
	latest map[string]proto.Stats
}

type agentConn struct {
	ws        *websocket.Conn
	lastStats time.Time
}

// NodeStatus is the payload of live.TypeNodeStatus events.
type NodeStatus struct {
	NodeID     string    `json:"node_id"`
	Status     string    `json:"status"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// NodeStats is the payload of live.TypeNodeStats events.
type NodeStats struct {
	NodeID string      `json:"node_id"`
	Stats  proto.Stats `json:"stats"`
}

// Latest returns the newest stats a node sent since the control center started.
func (g *Gateway) Latest(nodeID string) (proto.Stats, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.latest[nodeID]
	return s, ok
}

// verifiedNode returns the node id from a client certificate that the TLS
// layer verified against the cluster CA.
func verifiedNode(r *http.Request) (string, *x509.Certificate, bool) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return "", nil, false
	}
	leaf := r.TLS.VerifiedChains[0][0]
	return leaf.Subject.CommonName, leaf, true
}

// ServeHTTP serves the agent WebSocket.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	nodeID, cert, ok := verifiedNode(r)
	if !ok {
		http.Error(w, "client certificate required", http.StatusUnauthorized)
		return
	}
	if _, err := g.Store.GetNode(r.Context(), nodeID); err != nil {
		http.Error(w, "unknown node", http.StatusForbidden)
		return
	}
	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{proto.Subprotocol}})
	if err != nil {
		return
	}
	defer func() { _ = ws.CloseNow() }()
	if ws.Subprotocol() != proto.Subprotocol {
		_ = ws.Close(websocket.StatusCode(proto.CloseProtocolUnsupported), "subprotocol "+proto.Subprotocol+" required")
		return
	}
	ws.SetReadLimit(proto.MaxMessageBytes)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	log := g.Log.With("node", nodeID)

	hello, err := g.readHello(ctx, ws, nodeID)
	if err != nil {
		log.Warn("agentgw: hello failed", "err", err)
		return
	}
	now := g.Now()
	if err := g.Store.RecordHello(ctx, nodeID, hello, now); err != nil {
		log.Error("agentgw: record hello", "err", err)
		_ = ws.Close(websocket.StatusInternalError, "")
		return
	}
	c := &agentConn{ws: ws}
	g.register(nodeID, c)
	defer g.unregister(context.WithoutCancel(ctx), nodeID, c)
	g.Hub.Publish(live.TopicNodes, live.TypeNodeStatus, NodeStatus{NodeID: nodeID, Status: store.StatusOnline, LastSeenAt: now})
	log.Info("agentgw: agent connected", "version", hello.AgentVersion, "remote", r.RemoteAddr)

	if err := g.write(ctx, ws, proto.TypeHelloOK, proto.HelloOK{
		Protocol:        proto.Version,
		ServerTime:      now.UTC(),
		StatsIntervalS:  int(statsInterval / time.Second),
		MinAgentVersion: minAgentVersion,
		CmdPubKeys:      g.CmdKeys,
	}); err != nil {
		return
	}
	if cert.NotAfter.Sub(now) < pki.RenewBeforeEnd {
		if _, err := g.SendCommand(ctx, nodeID, proto.CmdCertRenew, nil); err != nil {
			log.Warn("agentgw: request certificate renewal", "err", err)
		}
	}

	go g.pingLoop(ctx, ws, cancel)
	g.readLoop(ctx, ws, nodeID, c, log)
}

func (g *Gateway) readHello(ctx context.Context, ws *websocket.Conn, nodeID string) (proto.Hello, error) {
	var hello proto.Hello
	hctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	_, data, err := ws.Read(hctx)
	if err != nil {
		return hello, err
	}
	env, err := proto.Decode(data)
	if err != nil {
		_ = ws.Close(websocket.StatusCode(proto.CloseProtocolUnsupported), "expected hello")
		return hello, fmt.Errorf("decode first frame: %w", err)
	}
	if env.Type != proto.TypeHello {
		_ = ws.Close(websocket.StatusCode(proto.CloseProtocolUnsupported), "expected hello")
		return hello, fmt.Errorf("first frame is %q, not hello", env.Type)
	}
	if err := env.Unmarshal(&hello); err != nil {
		_ = ws.Close(websocket.StatusCode(proto.CloseProtocolUnsupported), "invalid hello")
		return hello, err
	}
	if hello.Protocol != proto.Version {
		_ = ws.Close(websocket.StatusCode(proto.CloseProtocolUnsupported), fmt.Sprintf("protocol %d not supported", hello.Protocol))
		return hello, fmt.Errorf("protocol %d not supported", hello.Protocol)
	}
	if hello.NodeID != nodeID {
		_ = ws.Close(websocket.StatusCode(proto.CloseAuthFailed), "node id does not match certificate")
		return hello, fmt.Errorf("hello for node %q over certificate of %s", hello.NodeID, nodeID)
	}
	return hello, nil
}

func (g *Gateway) readLoop(ctx context.Context, ws *websocket.Conn, nodeID string, c *agentConn, log *slog.Logger) {
	for {
		rctx, cancel := context.WithTimeout(ctx, idleTimeout)
		_, data, err := ws.Read(rctx)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				log.Info("agentgw: agent disconnected", "reason", err)
			}
			return
		}
		env, err := proto.Decode(data)
		if err != nil {
			log.Warn("agentgw: invalid frame", "err", err)
			continue
		}
		switch env.Type {
		case proto.TypeStats:
			g.handleStats(ctx, nodeID, c, env, log)
		case proto.TypeCmdAck, proto.TypeCmdReject, proto.TypeCmdResult:
			log.Info("agentgw: command update", "type", env.Type, "data", string(env.Data))
		default:
			log.Warn("agentgw: unknown frame type", "type", env.Type)
		}
	}
}

func (g *Gateway) handleStats(ctx context.Context, nodeID string, c *agentConn, env proto.Envelope, log *slog.Logger) {
	now := g.Now()
	if now.Sub(c.lastStats) < minStatsSpacing {
		return // protect the database from a misbehaving agent
	}
	c.lastStats = now
	var st proto.Stats
	if err := env.Unmarshal(&st); err != nil {
		log.Warn("agentgw: invalid stats", "err", err)
		return
	}
	if err := g.Store.InsertStats(ctx, nodeID, now, st); err != nil {
		log.Error("agentgw: store stats", "err", err)
	}
	g.mu.Lock()
	if g.latest == nil {
		g.latest = map[string]proto.Stats{}
	}
	g.latest[nodeID] = st
	g.mu.Unlock()
	ev := NodeStats{NodeID: nodeID, Stats: st}
	g.Hub.Publish(live.TopicStats, live.TypeNodeStats, ev)
	g.Hub.Publish(live.TopicNodeStats(nodeID), live.TypeNodeStats, ev)
}

func (g *Gateway) pingLoop(ctx context.Context, ws *websocket.Conn, cancel context.CancelFunc) {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, pcancel := context.WithTimeout(ctx, writeTimeout)
			err := ws.Ping(pctx)
			pcancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

func (g *Gateway) register(nodeID string, c *agentConn) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.conns == nil {
		g.conns = map[string]*agentConn{}
	}
	if old := g.conns[nodeID]; old != nil {
		// Close waits for the peer's close frame; never block the new session on it.
		go old.ws.Close(websocket.StatusCode(proto.CloseSuperseded), "superseded by a newer connection")
	}
	g.conns[nodeID] = c
}

func (g *Gateway) unregister(ctx context.Context, nodeID string, c *agentConn) {
	g.mu.Lock()
	current := g.conns[nodeID] == c
	if current {
		delete(g.conns, nodeID)
	}
	g.mu.Unlock()
	if !current {
		return // a newer connection took over; the node is still online
	}
	now := g.Now()
	if err := g.Store.SetNodeOffline(ctx, nodeID, now); err != nil {
		g.Log.Error("agentgw: mark offline", "node", nodeID, "err", err)
	}
	g.Hub.Publish(live.TopicNodes, live.TypeNodeStatus, NodeStatus{NodeID: nodeID, Status: store.StatusOffline, LastSeenAt: now})
}

func (g *Gateway) write(ctx context.Context, ws *websocket.Conn, typ string, data any) error {
	frame, err := proto.Encode(typ, "", g.Now(), data)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return ws.Write(wctx, websocket.MessageText, frame)
}

// SendCommand signs a command for a connected node and sends it. It returns
// the command id.
func (g *Gateway) SendCommand(ctx context.Context, nodeID, typ string, payload any) (string, error) {
	g.mu.Lock()
	c := g.conns[nodeID]
	g.mu.Unlock()
	if c == nil {
		return "", ErrOffline
	}
	cmd, err := g.Signer.Sign(nodeID, typ, payload)
	if err != nil {
		return "", err
	}
	frame, err := proto.Encode(proto.TypeCmd, cmd.CmdID, g.Now(), cmd)
	if err != nil {
		return "", err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	if err := c.ws.Write(wctx, websocket.MessageText, frame); err != nil {
		return "", fmt.Errorf("agentgw: send command: %w", err)
	}
	g.Log.Info("agentgw: command sent", "node", nodeID, "type", typ, "cmd", cmd.CmdID)
	return cmd.CmdID, nil
}

// ServeRenew issues a new client certificate for the node identified by its
// current certificate.
func (g *Gateway) ServeRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	nodeID, _, ok := verifiedNode(r)
	if !ok {
		http.Error(w, "client certificate required", http.StatusUnauthorized)
		return
	}
	var req enroll.RenewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRenewBytes)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	now := g.Now()
	cert, err := g.Issuer.SignClientCSR(req.CSR, nodeID, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := g.Store.UpdateNodeCert(r.Context(), nodeID, cert.SerialNumber.Text(16), cert.NotAfter); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNotFound) {
			status = http.StatusForbidden
		}
		http.Error(w, "cannot renew", status)
		return
	}
	g.Log.Info("agentgw: certificate renewed", "node", nodeID, "not_after", cert.NotAfter)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(enroll.RenewResponse{ClientCert: cert.Raw})
}
