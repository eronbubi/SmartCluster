// Package api serves the REST API of the control center (ARCHITECTURE.md 7)
// for milestone a: version, health, cluster, nodes and their stats.
//
// There is no login before milestone b, so the API listener binds to
// localhost by default.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/pkg/proto"
)

// Store is the data the API reads.
type Store interface {
	Ping(ctx context.Context) error
	ListNodes(ctx context.Context) ([]store.Node, error)
	GetNode(ctx context.Context, id string) (*store.Node, error)
	QueryStats(ctx context.Context, nodeID string, from, to time.Time, res store.Resolution) ([]store.StatPoint, error)
}

// Live gives the newest in-memory stats of a node.
type Live interface {
	Latest(nodeID string) (proto.Stats, bool)
}

// Cluster is static information about the cluster.
type Cluster struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	CAFingerprint string   `json:"ca_fingerprint"`
	AgentPort     int      `json:"agent_port"`
	Addresses     []string `json:"addresses"`
}

// API holds the handlers.
type API struct {
	Store   Store
	Live    Live
	Events  http.Handler // the live WebSocket
	Cluster Cluster
	CAPEM   []byte
	Version string
	Now     func() time.Time
	Log     *slog.Logger
}

// Handler returns the routes under /api/v1.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range a.routes() {
		mux.Handle(http.MethodGet+" "+rt.path, rt.handler)
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return securityHeaders(cors(mux))
}

type route struct {
	path    string
	handler http.Handler
}

// routes are all GET endpoints. contract_test.go checks them against
// docs/api/openapi.yaml.
func (a *API) routes() []route {
	return []route{
		{"/api/v1/version", http.HandlerFunc(a.version)},
		{"/api/v1/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		})},
		{"/api/v1/readyz", http.HandlerFunc(a.ready)},
		{"/api/v1/ca.crt", http.HandlerFunc(a.caCert)},
		{"/api/v1/cluster", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, a.Cluster)
		})},
		{"/api/v1/nodes", http.HandlerFunc(a.listNodes)},
		{"/api/v1/nodes/{id}", http.HandlerFunc(a.getNode)},
		{"/api/v1/nodes/{id}/stats", http.HandlerFunc(a.nodeStats)},
		{"/api/v1/live", a.Events},
	}
}

func (a *API) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": a.Version, "protocol": proto.Version})
}

func (a *API) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.Store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "database is not reachable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (a *API) caCert(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="smartcluster-ca.crt"`)
	_, _ = w.Write(a.CAPEM)
}

func (a *API) listNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := a.Store.ListNodes(r.Context())
	if err != nil {
		a.internal(w, err)
		return
	}
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, a.toNode(n))
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out})
}

func (a *API) getNode(w http.ResponseWriter, r *http.Request) {
	n, err := a.Store.GetNode(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "node_not_found", "no such node")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.toNode(*n))
}

const (
	defaultStatsRange = time.Hour
	maxStatsRange     = store.RollupRetention
	rawUpTo           = 6 * time.Hour
)

func (a *API) nodeStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := a.Store.GetNode(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "node_not_found", "no such node")
		return
	} else if err != nil {
		a.internal(w, err)
		return
	}
	q := r.URL.Query()
	to := a.Now()
	from := to.Add(-defaultStatsRange)
	var err error
	if v := q.Get("to"); v != "" {
		if to, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_time", "to must be RFC 3339")
			return
		}
	}
	if v := q.Get("from"); v != "" {
		if from, err = time.Parse(time.RFC3339, v); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_time", "from must be RFC 3339")
			return
		}
	}
	if !from.Before(to) || to.Sub(from) > maxStatsRange {
		writeError(w, http.StatusBadRequest, "invalid_range", "from must be before to and the range at most 90 days")
		return
	}
	res := store.Resolution(q.Get("res"))
	switch res {
	case "", "auto":
		res = store.ResolutionFiveMin
		if to.Sub(from) <= rawUpTo {
			res = store.ResolutionRaw
		}
	case store.ResolutionRaw, store.ResolutionFiveMin:
	default:
		writeError(w, http.StatusBadRequest, "invalid_resolution", "res must be raw, 5m or auto")
		return
	}
	points, err := a.Store.QueryStats(r.Context(), id, from, to, res)
	if err != nil {
		a.internal(w, err)
		return
	}
	out := StatsResponse{Resolution: string(res), From: from.UTC(), To: to.UTC(), Points: make([]Point, len(points))}
	for i, p := range points {
		out.Points[i] = Point{TS: p.TS.UTC(), CPUPct: p.CPUPct, MemUsed: p.MemUsed, MemTotal: p.MemTotal, DiskUsedPct: p.DiskUsedPct,
			NetRxBps: p.NetRxBps, NetTxBps: p.NetTxBps, CPUTempC: p.CPUTempC, GPUUtilPct: p.GPUUtilPct}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) toNode(n store.Node) Node {
	out := Node{
		ID: n.ID, Name: n.Name, Hostname: n.Hostname, Status: n.Status, OS: n.OS, Hardware: n.Hardware,
		AgentVersion: n.AgentVersion, IPAddresses: n.IPAddresses, MACAddresses: n.MACAddresses,
		CertNotAfter: n.CertNotAfter.UTC(), CreatedAt: n.CreatedAt.UTC(), LastSeenAt: n.LastSeenAt,
	}
	if out.Hardware != nil {
		// Agents send empty lists, but a JSON null must never reach the app.
		hw := *out.Hardware
		hw.GPUs = nonNil(hw.GPUs)
		hw.NICs = nonNil(hw.NICs)
		for i := range hw.NICs {
			hw.NICs[i].Addrs = nonNil(hw.NICs[i].Addrs)
		}
		out.Hardware = &hw
	}
	out.IPAddresses, out.MACAddresses = nonNil(out.IPAddresses), nonNil(out.MACAddresses)
	if s, ok := a.Live.Latest(n.ID); ok {
		out.Latest = &s
	}
	return out
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (a *API) internal(w http.ResponseWriter, err error) {
	a.Log.Error("api: internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

// AppOrigin is the origin of the bundled renderer of the Electron app
// (ADR-0020). It is the only cross-origin caller the API answers.
const AppOrigin = "app://smartcluster"

// AppOriginHost is AppOrigin as WebSocket origin patterns see it.
const AppOriginHost = "smartcluster"

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") == AppOrigin {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", AppOrigin)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET")
				h.Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorBody is the error format of every endpoint.
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var e ErrorBody
	e.Error.Code, e.Error.Message = code, message
	writeJSON(w, status, e)
}
