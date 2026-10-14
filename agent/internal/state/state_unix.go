//go:build !windows

package state

import "os"

// DefaultDir is where the agent service keeps its state.
const DefaultDir = "/var/lib/smartcluster/agent"

func secureDir(dir string) error {
	return os.Chmod(dir, 0o700)
}
