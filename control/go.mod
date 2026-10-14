module github.com/eronbubi/smartcluster/control

go 1.27

require (
	github.com/coder/websocket v1.8.15
	github.com/eronbubi/smartcluster/pkg v0.0.0
	github.com/fergusstrange/embedded-postgres v1.34.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/libp2p/zeroconf/v2 v2.2.0
	github.com/pressly/goose/v3 v3.29.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/lib/pq v1.10.9 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/miekg/dns v1.1.43 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/eronbubi/smartcluster/pkg => ../pkg
