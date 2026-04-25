package logdir

import (
	"os"
	"path/filepath"
)

// Path returns the per-user application log directory.
func Path() (string, error) {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "satpulse", "log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "satpulse", "log"), nil
}
