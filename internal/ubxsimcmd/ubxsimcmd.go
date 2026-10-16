//go:build linux || darwin

// Package ubxsimcmd implements the ubxsim subcommand of satpulsetool. It
// hosts the u-blox receiver simulator (gps/app/ubxsim) behind a pty so
// that satpulsetool, satpulsed and satpulsewb can be tested against a
// configurable receiver with no GPS hardware. The pty slave path is
// printed on stdout; the simulator serves until interrupted.
package ubxsimcmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/ubxsim"
	ucv "github.com/jclark/satpulse/gps/lib/ubxcfgval"
	"github.com/spf13/pflag"
	"golang.org/x/sys/unix"
)

type flagVars struct {
	personalityPath string
	replayPath      string
	portName        string
	linkPath        string
	timing          bool
	phcPath         string
	edgePath        string
	leapSeconds     int
}

// Cmd implements the ubxsim subcommand.
func Cmd(logWriter io.Writer, logLevel slog.Level, progName string, cmdName string, args []string) (usage string, err error) {
	v, usageFunc, err := parseFlags(cmdName, args)
	if v == nil {
		if usageFunc != nil {
			usage = usageFunc(progName)
		}
		return
	}
	lg := cmd.NewLogger(logWriter, logLevel)
	return "", run(lg, v)
}

const summary = `[-h|--help] [--port name] [--link path] [-r|--replay file | --timing] personality.ubx`

func parseFlags(cmdName string, args []string) (*flagVars, func(string) string, error) {
	help := false
	v := &flagVars{}
	flags := pflag.NewFlagSet(cmdName, pflag.ContinueOnError)
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.StringVarP(&v.replayPath, "replay", "r", "", "nav replay packet log `file`; without replay or timing the simulator is silent")
	flags.BoolVar(&v.timing, "timing", false, "generate live 1 Hz timing messages and virtual PHC edges (Linux)")
	flags.StringVar(&v.phcPath, "phc", "", "use PHC `path` as timing reference instead of the system clock")
	flags.StringVar(&v.edgePath, "edge", "/sys/devices/platform/virt-phc-extts/extts_schedule", "virtual PHC EXTTS schedule sysfs `path`")
	flags.IntVar(&v.leapSeconds, "leap-seconds", 18, "fixed GPS-UTC offset in seconds")
	flags.StringVar(&v.portName, "port", "uart1", "receiver port to simulate: uart1, uart2, usb or spi")
	flags.StringVar(&v.linkPath, "link", "", "create a symlink at `path` to the pty slave")
	usageFunc := cmd.UsageFunc(cmdName, summary, flags)
	if err := flags.Parse(args); err != nil {
		return nil, usageFunc, err
	}
	if help {
		return nil, usageFunc, nil
	}
	if v.timing && v.replayPath != "" {
		return nil, usageFunc, fmt.Errorf("--timing and --replay cannot be combined")
	}
	if !v.timing && (flags.Changed("phc") || flags.Changed("edge") || flags.Changed("leap-seconds")) {
		return nil, usageFunc, fmt.Errorf("--phc, --edge and --leap-seconds require --timing")
	}
	if v.leapSeconds < 0 || v.leapSeconds > 127 {
		return nil, usageFunc, fmt.Errorf("--leap-seconds must be between 0 and 127")
	}
	if flags.NArg() != 1 {
		return nil, usageFunc, fmt.Errorf("expected one personality file argument")
	}
	v.personalityPath = flags.Arg(0)
	return v, usageFunc, nil
}

func run(lg *slog.Logger, v *flagVars) error {
	port, err := parsePort(v.portName)
	if err != nil {
		return err
	}
	p, err := ubxsim.LoadPersonality(v.personalityPath)
	if err != nil {
		return err
	}
	if v.replayPath != "" {
		if err := p.LoadReplay(v.replayPath); err != nil {
			return err
		}
	}
	lg.Info("personality loaded", "keys", len(p.Defaults), "epochs", len(p.Epochs))
	if p.MonGnss == nil {
		lg.Warn("no MON-GNSS in personality file; MON-GNSS polls will go unanswered")
	}
	if p.Skipped > 0 {
		lg.Info("replay packets with no MSGOUT key skipped", "count", p.Skipped)
	}
	var timing *ubxsim.Timing
	if v.timing {
		var closeTiming func()
		timing, closeTiming, err = openTiming(v)
		if err != nil {
			return err
		}
		defer closeTiming()
	}
	master, slavePath, slave, err := openPty()
	if err != nil {
		return err
	}
	defer master.Close()
	defer slave.Close()
	if v.linkPath != "" {
		if err := os.Symlink(slavePath, v.linkPath); err != nil {
			return err
		}
		defer os.Remove(v.linkPath)
	}
	fmt.Println(slavePath)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		master.Close()
	}()
	sim := ubxsim.New(p, ubxsim.Options{Port: port, Logger: lg, Timing: timing})
	err = sim.Run(ctx, master)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func parsePort(name string) (ucv.Port, error) {
	switch name {
	case "uart1":
		return ucv.UART1, nil
	case "uart2":
		return ucv.UART2, nil
	case "usb":
		return ucv.USB, nil
	case "spi":
		return ucv.SPI, nil
	}
	return 0, fmt.Errorf("invalid port %q (want uart1, uart2, usb or spi)", name)
}

// openPty opens a pty pair and puts the slave in raw mode, so that the
// line discipline does not echo simulator output back into its input
// before the client opens the slave and applies its own settings. The
// returned slave file is held open by the caller for the simulator's
// lifetime so that client disconnects do not read as EOF on the master.
// The master ioctls go through SyscallConn, not Fd, because Fd would
// put the file into blocking mode and Close could then no longer
// unblock the simulator's read loop.
func openPty() (master *os.File, slavePath string, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		return nil, "", nil, err
	}
	defer func() {
		if err != nil {
			master.Close()
		}
	}()
	rc, err := master.SyscallConn()
	if err != nil {
		return nil, "", nil, err
	}
	var ioctlErr error
	err = rc.Control(func(fd uintptr) {
		slavePath, ioctlErr = ptySlavePath(fd)
	})
	if err == nil {
		err = ioctlErr
	}
	if err != nil {
		return nil, "", nil, fmt.Errorf("ptmx ioctl: %w", err)
	}
	slave, err = os.OpenFile(slavePath, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, "", nil, err
	}
	tio, err := unix.IoctlGetTermios(int(slave.Fd()), reqGetTermios)
	if err == nil {
		tio.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
		tio.Oflag &^= unix.OPOST
		tio.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
		tio.Cflag &^= unix.CSIZE | unix.PARENB
		tio.Cflag |= unix.CS8
		err = unix.IoctlSetTermios(int(slave.Fd()), reqSetTermios, tio)
	}
	if err != nil {
		slave.Close()
		return nil, "", nil, fmt.Errorf("slave termios: %w", err)
	}
	return master, slavePath, slave, nil
}
