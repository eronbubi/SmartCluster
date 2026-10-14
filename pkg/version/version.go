// Package version holds the build version of agent and control center.
package version

// Version is set at build time:
//
//	go build -ldflags "-X github.com/eronbubi/smartcluster/pkg/version.Version=1.2.3"
var Version = "0.0.0-dev"
