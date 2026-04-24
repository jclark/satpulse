//go:build !windows

package main

import (
	"github.com/jclark/satpulse/internal/pmccmd"
	"github.com/jclark/satpulse/time/app/syncsimcmd"
)

func init() {
	commands["pmc"] = pmccmd.Cmd
	commands["syncsim"] = syncsimcmd.Cmd
}
