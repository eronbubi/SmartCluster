package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kardianos/service"

	"github.com/eronbubi/smartcluster/agent/internal/session"
	"github.com/eronbubi/smartcluster/agent/internal/state"
	"github.com/eronbubi/smartcluster/agent/internal/stats"
	"github.com/eronbubi/smartcluster/agent/internal/svc"
	"github.com/eronbubi/smartcluster/agent/internal/sysinfo"
	"github.com/eronbubi/smartcluster/pkg/version"
)

const maxLogBytes = 10 << 20

// runAgent runs the agent in the foreground or under the service manager.
func runAgent(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	dir := fs.String("state-dir", state.DefaultDir, "directory with the node identity")
	if err := fs.Parse(args); err != nil {
		return err
	}
	logOut, err := logWriter(*dir)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(logOut, nil))

	runner := &session.Runner{
		StateDir: *dir, Version: version.Version, Collector: stats.New(stats.System{}),
		Inventory: sysinfo.Collect, Log: log,
	}
	s, err := svc.New(*dir, runner.Run, log)
	if err != nil {
		return err
	}
	log.Info("agent starting", "version", version.Version, "state_dir", *dir)
	return s.Run()
}

// logWriter is stderr when interactive or under systemd (journald collects
// it). A Windows service has no console, so it logs to a file next to the
// state directory, restarted when it grows beyond 10 MB.
func logWriter(stateDir string) (io.Writer, error) {
	if service.Interactive() || runtime.GOOS != "windows" {
		return os.Stderr, nil
	}
	path := filepath.Join(filepath.Dir(stateDir), "logs", "agent.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		_ = os.Rename(path, path+".1")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
}

func runService(args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: smartcluster-agent service install|uninstall|start|stop|status [--state-dir dir]")
	}
	action := args[0]
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	dir := fs.String("state-dir", state.DefaultDir, "directory with the node identity")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, err := svc.New(*dir, func(context.Context) error { return nil }, slog.Default())
	if err != nil {
		return err
	}

	switch action {
	case "install":
		if _, err := state.Load(*dir); err != nil {
			return fmt.Errorf("join a cluster before installing the service: %w", err)
		}
		if err := s.Install(); err != nil {
			return adminHint(err)
		}
		if err := s.Start(); err != nil {
			return adminHint(err)
		}
		fmt.Fprintln(out, "Service installed and started.")
	case "uninstall":
		_ = s.Stop()
		if err := s.Uninstall(); err != nil {
			return adminHint(err)
		}
		fmt.Fprintln(out, "Service removed. The node identity is kept in", *dir)
	case "start", "stop":
		if err := service.Control(s, action); err != nil {
			return adminHint(err)
		}
	case "status":
		st, err := s.Status()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, map[service.Status]string{
			service.StatusRunning: "running", service.StatusStopped: "stopped", service.StatusUnknown: "not installed",
		}[st])
	default:
		return fmt.Errorf("unknown service action %q", action)
	}
	return nil
}

func adminHint(err error) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("%w (run the command in an administrator console)", err)
	}
	return fmt.Errorf("%w (run the command with sudo)", err)
}
