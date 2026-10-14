// Package session keeps the agent connected to the control center: mTLS
// WebSocket, hello, periodic stats, signed commands and reconnects with
// backoff (ARCHITECTURE.md 8.2).
package session

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/eronbubi/smartcluster/agent/internal/state"
	"github.com/eronbubi/smartcluster/agent/internal/stats"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
)

const (
	dialTimeout     = 15 * time.Second
	helloOKTimeout  = 10 * time.Second
	writeTimeout    = 10 * time.Second
	maxClockSkew    = 5 * time.Minute
	errorLogEvery   = time.Hour
	stableAfter     = time.Minute
	defaultInterval = 5 * time.Second
)

// Inventory returns OS and hardware for hello.
type Inventory func(ctx context.Context) (proto.OSInfo, proto.Hardware, []error)

// Runner keeps one session alive at a time.
type Runner struct {
	StateDir  string
	Version   string
	Collector *stats.Collector
	Inventory Inventory
	Log       *slog.Logger
	// MinBackoff and MaxBackoff bound the reconnect delay (defaults 1 s and 60 s).
	MinBackoff, MaxBackoff time.Duration

	errMu    sync.Mutex
	errsSeen map[string]time.Time
}

// Run connects, and reconnects after every failure, until ctx is done. It
// returns early only if the agent is not enrolled.
func (r *Runner) Run(ctx context.Context) error {
	minB, maxB := r.MinBackoff, r.MaxBackoff
	if minB == 0 {
		minB = time.Second
	}
	if maxB == 0 {
		maxB = time.Minute
	}
	backoff := minB
	for {
		st, err := state.Load(r.StateDir)
		if err != nil {
			return err
		}
		started := time.Now()
		err = r.session(ctx, st)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > stableAfter {
			backoff = minB
		}
		// Jitter keeps a whole cluster from reconnecting in lockstep after a restart.
		delay := backoff/2 + rand.N(backoff/2+1)
		r.Log.Warn("disconnected from control center", "err", err, "retry_in", delay.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
		backoff = min(backoff*2, maxB)
	}
}

func (r *Runner) session(ctx context.Context, st *state.State) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := mtlsClient(st)
	dctx, dcancel := context.WithTimeout(ctx, dialTimeout)
	ws, _, err := websocket.Dial(dctx, "wss://"+st.Control+"/agent/v1/ws", &websocket.DialOptions{
		HTTPClient: client, Subprotocols: []string{proto.Subprotocol},
	})
	dcancel()
	if err != nil {
		return fmt.Errorf("connect to %s: %w", st.Control, err)
	}
	defer ws.CloseNow()
	ws.SetReadLimit(proto.MaxMessageBytes)

	osInfo, hw, errs := r.Inventory(ctx)
	r.logErrors(errs)
	if err := write(ctx, ws, proto.TypeHello, proto.Hello{
		Protocol: proto.Version, AgentVersion: r.Version, NodeID: st.NodeID,
		OS: osInfo, Hardware: hw, Capabilities: []string{},
	}); err != nil {
		return err
	}

	ok, err := readHelloOK(ctx, ws)
	if err != nil {
		return err
	}
	offset := time.Until(ok.ServerTime)
	if offset > maxClockSkew || offset < -maxClockSkew {
		r.Log.Warn("clock differs from the control center; check time synchronization", "offset", offset.Round(time.Second))
	}
	keys, err := r.commandKeys(st, ok.CmdPubKeys)
	if err != nil {
		return err
	}
	verifier := sign.NewVerifier(st.NodeID, keys, func() time.Time { return time.Now().Add(offset) })
	interval := time.Duration(ok.StatsIntervalS) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}
	r.Log.Info("connected to control center", "control", st.Control, "node", st.NodeID)

	go r.statsLoop(ctx, cancel, ws, interval)
	h := &commands{st: st, client: client, log: r.Log}
	for {
		_, data, err := ws.Read(ctx)
		if err != nil {
			if status := websocket.CloseStatus(err); status != -1 {
				return fmt.Errorf("closed by control center: %d", status)
			}
			return err
		}
		env, err := proto.Decode(data)
		if err != nil {
			r.Log.Warn("invalid frame from control center", "err", err)
			continue
		}
		if env.Type == proto.TypeCmd {
			go h.handle(ctx, ws, verifier, env.Data)
		}
	}
}

func readHelloOK(ctx context.Context, ws *websocket.Conn) (proto.HelloOK, error) {
	var ok proto.HelloOK
	hctx, cancel := context.WithTimeout(ctx, helloOKTimeout)
	defer cancel()
	_, data, err := ws.Read(hctx)
	if err != nil {
		if status := websocket.CloseStatus(err); status != -1 {
			return ok, fmt.Errorf("control center refused the session: %d", status)
		}
		return ok, err
	}
	env, err := proto.Decode(data)
	if err != nil {
		return ok, err
	}
	if env.Type != proto.TypeHelloOK {
		return ok, fmt.Errorf("expected hello.ok, got %s", env.Type)
	}
	if err := env.Unmarshal(&ok); err != nil {
		return ok, err
	}
	if ok.Protocol != proto.Version {
		return ok, fmt.Errorf("control center speaks protocol %d, agent %d", ok.Protocol, proto.Version)
	}
	return ok, nil
}

// commandKeys returns the keys to verify commands with. The keys from
// hello.ok arrive over the mTLS channel to the pinned cluster CA and may
// rotate; changes are written back to the state.
func (r *Runner) commandKeys(st *state.State, announced []proto.PubKey) (map[string]ed25519.PublicKey, error) {
	use := st.CmdKeys
	if len(announced) > 0 {
		use = announced
	}
	keys := make(map[string]ed25519.PublicKey, len(use))
	for _, k := range use {
		pub, err := sign.DecodeKey(k.Key)
		if err != nil {
			return nil, fmt.Errorf("command key %s: %w", k.KeyID, err)
		}
		keys[k.KeyID] = pub
	}
	if len(announced) > 0 && !sameKeys(st.CmdKeys, announced) {
		if err := state.SaveCmdKeys(r.StateDir, announced); err != nil {
			r.Log.Warn("cannot store new command keys", "err", err)
		} else {
			r.Log.Info("command keys updated", "keys", len(announced))
		}
	}
	return keys, nil
}

func sameKeys(a, b []proto.PubKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *Runner) statsLoop(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, interval time.Duration) {
	r.Collector.Sample(ctx, time.Now()) // prime: rates need a previous reading
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s, errs := r.Collector.Sample(ctx, time.Now())
			r.logErrors(errs)
			if err := write(ctx, ws, proto.TypeStats, s); err != nil {
				cancel()
				return
			}
		}
	}
}

// logErrors logs each distinct reading error at most once per hour; a
// sensor that is never readable must not flood the log every 5 seconds.
func (r *Runner) logErrors(errs []error) {
	if len(errs) == 0 {
		return
	}
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if r.errsSeen == nil {
		r.errsSeen = map[string]time.Time{}
	}
	now := time.Now()
	for _, e := range errs {
		msg := e.Error()
		if last, ok := r.errsSeen[msg]; ok && now.Sub(last) < errorLogEvery {
			continue
		}
		r.errsSeen[msg] = now
		r.Log.Warn("reading not available", "err", msg)
	}
}

func mtlsClient(st *state.State) *http.Client {
	host, _, _ := net.SplitHostPort(st.Control)
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{st.Cert},
		RootCAs:      st.CAPool,
		ServerName:   host,
	}}}
}

func write(ctx context.Context, ws *websocket.Conn, typ string, data any) error {
	frame, err := proto.Encode(typ, "", time.Now(), data)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return ws.Write(wctx, websocket.MessageText, frame)
}

var errUnknownType = errors.New("unknown command type")
