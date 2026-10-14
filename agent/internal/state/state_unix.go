//go:build !windows

package state

import "os"

// DefaultDir is where the agent service keeps its state.
const DefaultDir = "/var/lib/smartcluster/agent"

func secureDir(dir string) error {
	return os.Chmod(dir, 0o700) //nolint:gosec // G302: a directory needs the execute bit; 0700 is owner-only
}
