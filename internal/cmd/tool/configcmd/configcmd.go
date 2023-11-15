package configcmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/jclark/satpulse/internal/cmd"
	"github.com/jclark/satpulse/internal/cmd/tool"
	"github.com/jclark/satpulse/internal/gpscfg"
	"github.com/jclark/satpulse/internal/gpsmsg"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/serio"

	"github.com/spf13/pflag"
)

func Cmd(lg *slog.Logger, progName string, cmdName string, args []string) error {
	var help bool
	var sane bool
	var speed cmd.IntFlag

	cm := &gpsmsg.ConfigMap{}
	flags := pflag.NewFlagSet("config", pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")

	flags.VarP(&speed, "speed", "s", "serial device `baud-rate`")
	flags.BoolVarP(&sane, "sane", "S", false, "configure the receiver defaults that are sane for timing")
	err := flags.Parse(args)
	if err != nil {
		cmd.ErrPrintln(progName, err)
		os.Exit(2)
	}
	const summary = "[options] serial-device"
	if flags.NArg() != 1 {
		cmd.ErrPrintln(progName, "config command must have argument giving serial device")
		tool.Usage(progName, cmdName, summary, flags)
		os.Exit(2)
	}
	device := flags.Arg(0)

	if sane {
		cm.SetSane()
		gpsmsg.CfgNMEAEnabled.Set(cm, false)
	}

	ctx := context.Background()
	ctx, cancel := cmd.CancelOnSignal(ctx, lg)
	return run(ctx, lg, cancel, cm, device, speed.Value)
}

func run(ctx context.Context, lg *slog.Logger, cancel context.CancelFunc, cm *gpsmsg.ConfigMap, serialDev string, speed *int) error {

	t, err := serio.OpenTerm(serialDev, speed)
	if err != nil {
		return err
	}

	defer func() {
		lg.Debug("restoring the serial port settings", "path", serialDev)
		e := t.Restore()
		if e != nil {
			lg.Error("error while restoring the serial port settings", e, "path", serialDev)
		} else {
			lg.Debug("successfully restored the serial port settings", "path", serialDev)
		}
		lg.Debug("closing the serial port", "path", serialDev)
		t.Close()
		lg.Debug("successfully closed the serial port", "path", serialDev)
	}()

	lg.Debug("detecting kind of serial port", "devKind", t.DevKind())

	scanner := serio.NewScanner(t)
	var wg sync.WaitGroup

	pCh := startScan(ctx, lg, &wg, scanner)

	// Let the compiler check that TermError implements the SerialError interface
	// gpsInit relies on this
	var _ gpscfg.SerialError = serio.TermError{}
	info, err := gpscfg.Configure(ctx, lg, cm, pCh, t)
	if err == nil {
		fmt.Printf("set config to: %s\n", fmt.Sprint(info.ConfigMap))
	}

	lg.Debug("about to wait")

	// stop the scan worker
	cancel()
	// need to keep reading here to avoid deadlock
	for range pCh {
	}
	wg.Wait()
	return err
}

func startScan(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, scanner *scan.Scanner) <-chan scan.Packet {
	msg := make(chan scan.Packet, 1)
	cmd.WaitGroupGo(wg, func() { serio.ScanWorker(ctx, lg, scanner, msg) })
	return msg
}
