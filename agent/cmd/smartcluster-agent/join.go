package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/eronbubi/smartcluster/agent/internal/discover"
	"github.com/eronbubi/smartcluster/agent/internal/state"
	"github.com/eronbubi/smartcluster/pkg/enroll"
)

const (
	joinTimeout  = time.Minute
	discoverWait = 3 * time.Second
)

func runJoin(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	control := fs.String("control", "", "address of the control center, host or host:port (port defaults to "+enroll.DefaultAgentPort+"); found via mDNS if empty")
	dir := fs.String("state-dir", state.DefaultDir, "directory for the node identity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: smartcluster-agent join [--control host[:port]] [--state-dir dir] <JOIN-CODE>")
	}
	code, err := enroll.ParseCode(fs.Arg(0))
	if err != nil {
		return err
	}
	address := *control
	if address == "" {
		found, err := discover.Browse(ctx, discoverWait)
		if err != nil {
			return err
		}
		switch len(found) {
		case 0:
			return errors.New("no control center found in the local network; pass its address with --control")
		case 1:
			address = found[0].Address
			fmt.Fprintf(out, "Found %s at %s\n", found[0].Instance, address)
		default:
			var list []string
			for _, f := range found {
				list = append(list, fmt.Sprintf("  %s  %s", f.Address, f.Instance))
			}
			return fmt.Errorf("several control centers found, choose one with --control:\n%s", strings.Join(list, "\n"))
		}
	}
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, enroll.DefaultAgentPort)
	}

	if existing, err := state.Load(*dir); err == nil {
		return fmt.Errorf("already enrolled as node %s; delete %s to join again", existing.NodeID, *dir)
	} else if !errors.Is(err, state.ErrNotEnrolled) {
		return err
	}

	hostname, _ := os.Hostname()
	ctx, cancel := context.WithTimeout(ctx, joinTimeout)
	defer cancel()
	id, err := enroll.Enroll(ctx, address, code, hostname)
	switch {
	case errors.Is(err, enroll.ErrRefused):
		return errors.New("the control center refused the code: it is wrong, expired (30 minutes) or already used. Create a new one")
	case errors.Is(err, enroll.ErrServerProof):
		return fmt.Errorf("%s is not the control center that created this code (or someone is intercepting the connection)", address)
	case err != nil:
		return err
	}
	if err := state.Save(*dir, address, id); err != nil {
		return err
	}
	fmt.Fprintf(out, "Joined as node %s\nCluster CA fingerprint: %s\n", id.NodeID, enroll.Fingerprint(id.CA))
	return nil
}

func runStatus(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	dir := fs.String("state-dir", state.DefaultDir, "directory for the node identity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := state.Load(*dir)
	if err != nil {
		return err
	}
	leaf := s.Cert.Leaf
	fmt.Fprintf(out, "Node:            %s\nControl center:  %s\nCertificate:     valid until %s\nCA fingerprint:  %s\n",
		s.NodeID, s.Control, leaf.NotAfter.Format(time.DateOnly), enroll.Fingerprint(s.CA))
	return nil
}
