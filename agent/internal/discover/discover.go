// Package discover finds control centers in the local network via mDNS
// (ADR-0009). Discovery is only a convenience: the join code's proofs still
// authenticate the control center, so a spoofed announcement cannot take
// over an enrollment.
package discover

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strconv"
	"time"

	"github.com/libp2p/zeroconf/v2"

	"github.com/eronbubi/smartcluster/pkg/enroll"
)

// Found is one announced control center.
type Found struct {
	Instance string
	Address  string // host:port of the agent listener
}

// Browse listens for announcements for the given time.
func Browse(ctx context.Context, wait time.Duration) ([]Found, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	entries := make(chan *zeroconf.ServiceEntry, 16)
	var found []Found
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			if f, ok := toFound(e); ok && !slices.Contains(found, f) {
				found = append(found, f)
			}
		}
	}()
	err := zeroconf.Browse(ctx, enroll.ServiceType, "local.", entries)
	<-done
	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("discover: %w", err)
	}
	return found, nil
}

// toFound prefers a private IPv4 address; enrollment needs one that the
// control center's certificate covers.
func toFound(e *zeroconf.ServiceEntry) (Found, bool) {
	for _, ip := range e.AddrIPv4 {
		if ip.IsPrivate() || ip.IsLoopback() {
			return Found{Instance: e.Instance, Address: net.JoinHostPort(ip.String(), strconv.Itoa(e.Port))}, true
		}
	}
	return Found{}, false
}
