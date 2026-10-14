package svc

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	cfg := Config("/var/lib/smartcluster/agent")
	if cfg.Name != Name || strings.Join(cfg.Arguments, " ") != "run --state-dir /var/lib/smartcluster/agent" {
		t.Errorf("config %+v", cfg)
	}
	unit, _ := cfg.Option["SystemdScript"].(string)
	if !strings.Contains(unit, "RestartSec=5") || strings.Contains(unit, "RestartSec=120") {
		t.Error("systemd unit does not restart quickly")
	}
}

func TestProgramStartStop(t *testing.T) {
	stopped := make(chan struct{})
	p := &program{log: slog.New(slog.NewTextHandler(io.Discard, nil)), run: func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return nil
	}}
	if err := p.Start(nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Error("run did not observe the stop")
	}
}
