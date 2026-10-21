package main

import (
	"github.com/jclark/satpulse/internal/ppscmd"
	"github.com/jclark/satpulse/internal/sdpcmd"
)

func init() {
	commands["sdp"] = sdpcmd.Cmd
	commands["pps"] = ppscmd.Cmd
}
