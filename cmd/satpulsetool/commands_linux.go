package main

import (
	"github.com/jclark/satpulse/internal/gpiocmd"
	"github.com/jclark/satpulse/internal/sdpcmd"
)

func init() {
	commands["sdp"] = sdpcmd.Cmd
	commands["gpio"] = gpiocmd.Cmd
}
