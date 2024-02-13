package main

import (
	"os"

	"github.com/jclark/satpulse/internal/daemon"
)

func main() {
	daemon.Cmd(os.Args[0], os.Args[1:])
}
