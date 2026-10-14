// Package server wires the control center together: database, cluster
// identity, agent listener (enrollment, agent WebSocket, renewal), API
// listener and background maintenance.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/agentgw"
	"github.com/eronbubi/smartcluster/control/internal/api"
	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/control/internal/enroll"
	"github.com/eronbubi/smartcluster/control/internal/live"
	"github.com/eronbubi/smartcluster/control/internal/pki"
	"github.com/eronbubi/smartcluster/control/internal/secrets"
	"github.com/eronbubi/smartcluster/control/internal/store"
	pkgenroll "github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/ids"
	"github.com/eronbubi/smartcluster/pkg/proto"
	"github.com/eronbubi/smartcluster/pkg/sign"
	"github.com/eronbubi/smartcluster/pkg/version"
)

// Config is the control center configuration.
type Config struct {
	DatabaseURL string
	DataDir     string
	AgentAddr   string
	APIAddr     string
	// Hosts are extra IP addresses or .local names for the server certificate.
	Hosts       []string
	ClusterName string
	// UI serves the web app; nil serves the API only.
	UI http.Handler
}

const shutdownTimeout = 5 * time.Second

// Run starts the control center and blocks until ctx is done.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	key, err := secrets.LoadOrCreateKey(filepath.Join(cfg.DataDir, "master.key"))
	if err != nil {
		return err
	}
	box, err := secrets.NewBox(key)
	if err != nil {
		return err
	}
	st := store.New(pool, box)
	cluster, err := loadOrCreateCluster(ctx, st, cfg.ClusterName, log)
	if err != nil {
		return err
	}

	hosts, err := serverHosts(cfg.Hosts)
	if err != nil {
		return err
	}
	certs := &certHolder{ca: cluster.CA, hosts: hosts}
	if err := certs.renew(time.Now()); err != nil {
		return err
	}

	now := time.Now()
	if err := st.MarkAllOffline(ctx); err != nil {
		return err
	}
	if err := st.EnsurePartitions(ctx, now); err != nil {
		return err
	}
	if _, err := st.DropOldPartitions(ctx, now); err != nil {
		return err
	}

	cmdKeys := []proto.PubKey{{KeyID: cluster.CmdKeyID, Key: sign.EncodeKey(cluster.CmdKey.Public().(ed25519.PublicKey))}}
	// Same origin (web app) and the Electron renderer; nothing else.
	hub := &live.Hub{OriginPatterns: []string{api.AppOriginHost}, Log: log}
	gw := &agentgw.Gateway{
		Store: st, Hub: hub, Signer: sign.NewSigner(cluster.CmdKeyID, cluster.CmdKey), CmdKeys: cmdKeys,
		Issuer: cluster.CA, Now: time.Now, Log: log,
	}
	agentMux := http.NewServeMux()
	agentMux.Handle(pkgenroll.Path, &enroll.Handler{
		Store: st, Issuer: cluster.CA, CACert: cluster.CA.Cert,
		ServerCert: func() *x509.Certificate { return certs.current().Leaf },
		CmdKeys:    func() []proto.PubKey { return cmdKeys },
		Now:        time.Now, Log: log,
	})
	agentMux.Handle(agentgw.WSPath, gw)
	agentMux.HandleFunc(pkgenroll.RenewPath, gw.ServeRenew)

	caPEM, _, err := cluster.CA.MarshalPEM()
	if err != nil {
		return err
	}
	agentPort, err := portOf(cfg.AgentAddr)
	if err != nil {
		return err
	}
	apiHandler := (&api.API{
		Store: st, Live: gw, Events: hub, CAPEM: caPEM, Version: version.Version, Now: time.Now, Log: log,
		Cluster: api.Cluster{
			ID: cluster.ID, Name: cluster.Name, CAFingerprint: cluster.CA.Fingerprint(),
			AgentPort: agentPort, Addresses: lanAddresses(),
		},
	}).Handler()
	if cfg.UI != nil {
		mux := http.NewServeMux()
		mux.Handle("/api/", apiHandler)
		mux.Handle("/", cfg.UI)
		apiHandler = mux
	}

	errLog := slog.NewLogLogger(log.Handler(), slog.LevelDebug) // TLS handshake noise from browsers
	agentSrv := &http.Server{
		Addr: cfg.AgentAddr, Handler: agentMux, ReadHeaderTimeout: 10 * time.Second, ErrorLog: errLog,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, GetCertificate: certs.get,
			ClientAuth: tls.VerifyClientCertIfGiven, ClientCAs: cluster.CA.Pool(),
		},
	}
	apiSrv := &http.Server{
		Addr: cfg.APIAddr, Handler: apiHandler, ReadHeaderTimeout: 10 * time.Second, ErrorLog: errLog,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, GetCertificate: certs.get},
	}

	go maintain(ctx, st, certs, log)
	errs := make(chan error, 2)
	for _, srv := range []*http.Server{agentSrv, apiSrv} {
		go func() {
			if err := srv.ListenAndServeTLS("", ""); !errors.Is(err, http.ErrServerClosed) {
				errs <- fmt.Errorf("listen on %s: %w", srv.Addr, err)
			}
		}()
	}
	stopAdvertising := advertise(cluster, agentPort, log)
	defer stopAdvertising()
	log.Info("control center running",
		"agents", cfg.AgentAddr, "api", cfg.APIAddr, "cluster", cluster.Name,
		"ca_fingerprint", cluster.CA.Fingerprint(), "certificate_hosts", strings.Join(hosts, ","))
	if !isLoopback(cfg.APIAddr) {
		log.Warn("the API has no login before milestone b and is reachable from the network", "addr", cfg.APIAddr)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errs:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = agentSrv.Shutdown(shutdownCtx)
	_ = apiSrv.Shutdown(shutdownCtx)
	return runErr
}

func loadOrCreateCluster(ctx context.Context, st *store.Store, name string, log *slog.Logger) (*store.Cluster, error) {
	cluster, err := st.LoadCluster(ctx)
	if err == nil {
		return cluster, nil
	}
	if !errors.Is(err, store.ErrNoCluster) {
		return nil, err
	}
	ca, err := pki.NewCA(name, time.Now())
	if err != nil {
		return nil, err
	}
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(pub)
	cluster = &store.Cluster{ID: ids.NewV7(), Name: name, CA: ca, CmdKeyID: "cmd-" + hex.EncodeToString(sum[:4]), CmdKey: key}
	if err := st.CreateCluster(ctx, cluster); err != nil {
		return nil, err
	}
	log.Info("new cluster created", "name", name, "ca_fingerprint", ca.Fingerprint())
	return cluster, nil
}

// certHolder serves the current server certificate and replaces it before
// it expires.
type certHolder struct {
	ca    *pki.CA
	hosts []string
	cert  atomic.Pointer[tls.Certificate]
}

func (h *certHolder) current() *tls.Certificate { return h.cert.Load() }

func (h *certHolder) get(*tls.ClientHelloInfo) (*tls.Certificate, error) { return h.current(), nil }

func (h *certHolder) renew(now time.Time) error {
	c, err := h.ca.IssueServer(h.hosts, now)
	if err != nil {
		return err
	}
	h.cert.Store(&c)
	return nil
}

func maintain(ctx context.Context, st *store.Store, certs *certHolder, log *slog.Logger) {
	minute := time.NewTicker(time.Minute)
	hour := time.NewTicker(time.Hour)
	defer minute.Stop()
	defer hour.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-minute.C:
			if err := st.Rollup(ctx, now); err != nil {
				log.Error("rollup", "err", err)
			}
		case now := <-hour.C:
			if err := st.EnsurePartitions(ctx, now); err != nil {
				log.Error("create partitions", "err", err)
			}
			if dropped, err := st.DropOldPartitions(ctx, now); err != nil {
				log.Error("drop partitions", "err", err)
			} else if len(dropped) > 0 {
				log.Info("old stats removed", "partitions", dropped)
			}
			if certs.current().Leaf.NotAfter.Sub(now) < pki.RenewBeforeEnd {
				if err := certs.renew(now); err != nil {
					log.Error("renew server certificate", "err", err)
				}
			}
		}
	}
}

// serverHosts lists every name and address agents and browsers may use:
// localhost, <hostname>.local and the private addresses of this machine.
func serverHosts(extra []string) ([]string, error) {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if name, err := os.Hostname(); err == nil {
		short := strings.ToLower(strings.Split(name, ".")[0])
		if pki.Allowed(short + ".local") {
			hosts = append(hosts, short+".local")
		}
	}
	hosts = append(hosts, lanAddresses()...)
	for _, h := range extra {
		if !pki.Allowed(h) {
			return nil, fmt.Errorf("--host %s: only private addresses and .local names are allowed (ADR-0015)", h)
		}
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	return slices.Compact(hosts), nil
}

// lanAddresses returns the private, non-link-local addresses of this machine.
func lanAddresses() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{}
	}
	out := []string{}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ip := ipnet.IP.String(); pki.Allowed(ip) {
			out = append(out, ip)
		}
	}
	return out
}

func portOf(addr string) (int, error) {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("address %q: %w", addr, err)
	}
	return strconv.Atoi(p)
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}
