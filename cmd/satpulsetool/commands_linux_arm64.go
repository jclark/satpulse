package main

import "github.com/jclark/satpulse/internal/gpiocmd"

func init() {
	commands["gpio"] = gpiocmd.Cmd
}
