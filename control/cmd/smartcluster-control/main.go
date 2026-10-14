// Command smartcluster-control is the SmartCluster control center.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/control/internal/server"
	"github.com/eronbubi/smartcluster/control/internal/store"
	"github.com/eronbubi/smartcluster/pkg/enroll"
	"github.com/eronbubi/smartcluster/pkg/version"
)

const usage = `Usage: smartcluster-control <command> [flags]

Commands:
  serve          run the control center
  token create   create a join code for a new node
  version        print the version

Configuration can also come from SC_DATABASE_URL and SC_DATA_DIR.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch os.Args[1] {
	case "serve":
		err = runServe(ctx, os.Args[2:])
	case "token":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		err = runTokenCreate(ctx, os.Args[3:], os.Stdout)
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func defaultDataDir() string {
	if v := os.Getenv("SC_DATA_DIR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\SmartCluster\control`
	}
	return "/var/lib/smartcluster/control"
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runServe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	cfg := server.Config{}
	fs.StringVar(&cfg.DatabaseURL, "database-url", os.Getenv("SC_DATABASE_URL"), "Postgres connection URL")
	fs.StringVar(&cfg.DataDir, "data-dir", defaultDataDir(), "directory for the master key")
	fs.StringVar(&cfg.AgentAddr, "agent-addr", ":"+enroll.DefaultAgentPort, "listen address for agents (enrollment, WebSocket)")
	fs.StringVar(&cfg.APIAddr, "api-addr", "127.0.0.1:8443", "listen address for the API and the web app (no login before milestone b)")
	fs.StringVar(&cfg.ClusterName, "cluster-name", "SmartCluster", "name of a new cluster")
	var hosts stringList
	fs.Var(&hosts, "host", "extra private IP or .local name for the server certificate (repeatable)")
	level := fs.String("log-level", "info", "debug, info, warn or error")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("--database-url or SC_DATABASE_URL is required")
	}
	cfg.Hosts = hosts
	cfg.UI = webUI()

	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(*level)); err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
	return server.Run(ctx, cfg, log)
}

func runTokenCreate(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	dbURL := fs.String("database-url", os.Getenv("SC_DATABASE_URL"), "Postgres connection URL")
	ttl := fs.Duration("ttl", 30*time.Minute, "how long the code stays valid")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbURL == "" {
		return errors.New("--database-url or SC_DATABASE_URL is required")
	}
	if *ttl <= 0 || *ttl > 24*time.Hour {
		return errors.New("--ttl must be between 1s and 24h")
	}
	if err := db.Migrate(ctx, *dbURL); err != nil {
		return err
	}
	pool, err := db.Open(ctx, *dbURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	code := enroll.NewCode()
	expires := time.Now().Add(*ttl)
	// Join codes need no secrets, so no master key is required here.
	if _, err := store.New(pool, nil).CreateJoinCode(ctx, code.Key(), expires); err != nil {
		return err
	}
	fmt.Fprintf(out, "Join code: %s\nValid until %s, single use.\n\nOn the new machine run:\n  smartcluster-agent join --control <address of this machine> %s\n",
		code, expires.Format("15:04"), code)
	if addrs := privateIPv4(); len(addrs) > 0 {
		fmt.Fprintf(out, "\nAddresses of this machine: %s\n", strings.Join(addrs, ", "))
	}
	return nil
}

func privateIPv4() []string {
	addrs, _ := net.InterfaceAddrs()
	var out []string
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil && ipnet.IP.IsPrivate() {
			out = append(out, ipnet.IP.String())
		}
	}
	return out
}
