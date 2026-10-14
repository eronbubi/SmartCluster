// Package testenv gates tests that start local servers (Postgres, TLS and
// WebSocket listeners, the control center). They run in CI and on test
// machines with SC_TEST_SERVERS=1, never by default on a developer PC.
package testenv

import (
	"os"
	"testing"
)

// ServersVar is the environment variable that enables server tests.
const ServersVar = "SC_TEST_SERVERS"

// RequireServers skips the test unless SC_TEST_SERVERS=1.
func RequireServers(t testing.TB) {
	t.Helper()
	if os.Getenv(ServersVar) != "1" {
		t.Skip("starts local servers; runs with " + ServersVar + "=1 (CI, test machines)")
	}
}
