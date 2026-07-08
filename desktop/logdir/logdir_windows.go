package logdir

import (
	"os"
	"path/filepath"
)

// Path returns the per-user application log directory.
func Path() (string, error) {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		return filepath.Join(localAppData, "SatPulse", "Logs"), nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "SatPulse", "Logs"), nil
}
