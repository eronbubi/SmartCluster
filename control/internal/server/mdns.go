package server

import (
	"log/slog"

	"github.com/libp2p/zeroconf/v2"

	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/pkg/enroll"
)

// advertise announces the agent listener via mDNS so that "join" works
// without typing an address (ADR-0009). Failure is not fatal: agents can
// always be given the address explicitly.
func advertise(cluster *store.Cluster, agentPort int, log *slog.Logger) func() {
	srv, err := zeroconf.Register("SmartCluster "+cluster.Name, enroll.ServiceType, "local.", agentPort,
		[]string{"v=1", "cluster=" + cluster.ID}, nil)
	if err != nil {
		log.Warn("mDNS announcement not available; agents need --control", "err", err)
		return func() {}
	}
	return srv.Shutdown
}
