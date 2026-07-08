package logdir

import (
	"os"
	"path/filepath"
)

// Path returns the per-user application log directory.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Logs", "SatPulse"), nil
}
