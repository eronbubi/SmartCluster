// Command smartcluster-agent is the SmartCluster node agent.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/eronbubi/smartcluster/agent/internal/stats"
	"github.com/eronbubi/smartcluster/agent/internal/sysinfo"
	"github.com/eronbubi/smartcluster/pkg/version"
)

const usage = `Usage: smartcluster-agent <command> [flags]

Commands:
  join        join a cluster with a join code ([--control host[:port]] CODE)
  status      show node id, control center and CA fingerprint
  run         run the agent in the foreground (the service runs this)
  service     install|uninstall|start|stop|status the system service
  version     print the agent version
  inventory   print operating system and hardware inventory as JSON
  stats       print stats samples as JSON (-n count, -interval duration)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var err error
	switch os.Args[1] {
	case "join":
		err = runJoin(ctx, os.Args[2:], os.Stdout)
	case "status":
		err = runStatus(os.Args[2:], os.Stdout)
	case "run":
		err = runAgent(os.Args[2:])
	case "service":
		err = runService(os.Args[2:], os.Stdout)
	case "version":
		fmt.Println(version.Version)
	case "inventory":
		err = runInventory(ctx, os.Stdout, os.Stderr)
	case "stats":
		err = runStats(ctx, os.Args[2:], os.Stdout, os.Stderr)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runInventory(ctx context.Context, out, errOut io.Writer) error {
	osInfo, hw, errs := sysinfo.Collect(ctx)
	for _, e := range errs {
		fmt.Fprintln(errOut, "warning:", e)
	}
	return writeJSON(out, map[string]any{"os": osInfo, "hardware": hw})
}

func runStats(ctx context.Context, args []string, out, errOut io.Writer) error {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	count := fs.Int("n", 1, "number of samples to print")
	interval := fs.Duration("interval", 5*time.Second, "time between samples")
	if err := fs.Parse(args); err != nil {
		return err
	}

	c := stats.New(stats.System{})
	c.Sample(ctx, time.Now()) // primes the counters; rates need a previous reading
	for i := 0; i < *count; i++ {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(*interval):
		}
		s, errs := c.Sample(ctx, time.Now())
		for _, e := range errs {
			fmt.Fprintln(errOut, "warning:", e)
		}
		if err := writeJSON(out, s); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
