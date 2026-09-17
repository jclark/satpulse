//go:build linux || darwin

package main

import (
	"github.com/jclark/satpulse/internal/ubxsimcmd"
)

func init() {
	commands["ubxsim"] = ubxsimcmd.Cmd
}
