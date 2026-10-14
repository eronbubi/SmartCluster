// Package svc runs the agent as a system service: a Windows service as
// LocalSystem or a systemd unit as root (kardianos/service).
package svc

import (
	"context"
	"log/slog"
	"time"

	"github.com/kardianos/service"
)

// Name is the service name on both platforms.
const Name = "smartcluster-agent"

const stopTimeout = 10 * time.Second

// systemdUnit replaces the library default, which waits 120 s before a
// restart; a server should be back within seconds.
const systemdUnit = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// Config describes the service. The service manager starts
// "smartcluster-agent run --state-dir <dir>".
func Config(stateDir string) *service.Config {
	return &service.Config{
		Name:        Name,
		DisplayName: "SmartCluster Agent",
		Description: "Connects this machine to its SmartCluster control center.",
		Arguments:   []string{"run", "--state-dir", stateDir},
		Option: service.KeyValue{
			"SystemdScript":          systemdUnit,
			"OnFailure":              "restart",
			"OnFailureDelayDuration": "5s",
			"OnFailureResetPeriod":   60,
		},
	}
}

// program adapts a run function to the service interface.
type program struct {
	run    func(context.Context) error
	log    *slog.Logger
	cancel context.CancelFunc
	done   chan struct{}
}

func (p *program) Start(service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	go func() {
		defer close(p.done)
		if err := p.run(ctx); err != nil {
			p.log.Error("agent stopped", "err", err)
		}
	}()
	return nil
}

func (p *program) Stop(service.Service) error {
	p.cancel()
	select {
	case <-p.done:
	case <-time.After(stopTimeout):
	}
	return nil
}

// New returns the service for stateDir. run is executed when the service
// manager (or an interactive "run") starts it.
func New(stateDir string, run func(context.Context) error, log *slog.Logger) (service.Service, error) {
	return service.New(&program{run: run, log: log}, Config(stateDir))
}
