package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/gps2phc/logctx"
	"github.com/jclark/gps2phc/scan"
	"github.com/jclark/gps2phc/serio"
	"golang.org/x/exp/slog"
	"golang.org/x/sys/unix"
)

var serialDev string
var debugEnable bool

func main() {
	flag.StringVar(&serialDev, "s", "/dev/ttyUSB0", "device for serial connection")
	flag.BoolVar(&debugEnable, "d", false, "log debugging information")
	flag.Parse()
	level := slog.LevelInfo
	if debugEnable {
		level = slog.LevelDebug
	}
	lg := slog.New(slog.HandlerOptions{Level: level}.NewTextHandler(os.Stdout))
	slog.SetDefault(lg)
	ctx := logctx.NewContext(context.Background(), lg)
	ctx, _ = cancelOnSignal(ctx)
	err := run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func cancelOnSignal(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		logctx.FromContext(ctx).Info("gracefulShutdown")
		cancel()
	}()
	return ctx, cancel
}

func run(ctx context.Context) error {
	t, err := serio.OpenTerm(serialDev)
	if err != nil {
		return err
	}
	defer t.Close()
	defer t.Restore()
	var wg sync.WaitGroup
	err = tcpServe(ctx, &wg, ":2006", scan.New(t, 16), t)
	if err != nil {
		return err
	}
	wg.Wait()
	return nil
}

func tcpServe(ctx context.Context, wg *sync.WaitGroup, address string, scanner *scan.Scanner, port serio.OutPort) error {
	cfg := net.ListenConfig{}
	listen, err := cfg.Listen(ctx, "tcp", address)
	if err != nil {
		return err
	}
	msg := make(chan scan.Frame, 1)
	b := serio.NewBcast(msg)
	wg.Add(1)
	go b.Run(ctx, wg)
	wg.Add(1)
	go func() {
		serio.ScanWorker(ctx, scanner, msg)
		wg.Done()
	}()
	portLock := make(chan serio.OutPort, 1)
	portLock <- port
	wg.Add(1)
	go handleListen(ctx, wg, listen, b, portLock)
	return nil
}

func handleListen(ctx context.Context, wg *sync.WaitGroup, listen net.Listener, b *serio.Bcast, portLock chan serio.OutPort) {
	go func() {
		<-ctx.Done()
		listen.Close()
	}()
	defer wg.Done()
	defer logctx.FromContext(ctx).Debug("listenDone")
	defer b.Close()
	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			logConnErr(ctx, "acceptErr", err)
			return
		}
		handleConn(ctx, wg, conn, b, portLock)
	}
}

func handleConn(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *serio.Bcast, portLock chan serio.OutPort) {
	// XXX both the read and write workers are closing the connection.
	// Not sure if it would better for just one of them to do so.
	wg.Add(1)
	// subscribe in the goroutine that closes the subscribe channel to avoid a race
	go connWriteWorker(ctx, wg, conn, b, b.Subscribe())
	wg.Add(1)
	go connReadWorker(ctx, wg, conn, portLock)
}

// connWriteWorker reads from a channel and write to the connection.
func connWriteWorker(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *serio.Bcast, ch <-chan scan.Frame) {
	defer wg.Done()
	defer logctx.FromContext(ctx).Debug("connWriteDone")
	defer conn.Close()
	defer func() {
		b.Unsubscribe(ch)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err := conn.Write(([]byte)(msg.Data))
			if err != nil {
				logConnErr(ctx, "connWriteErr", err)
				return
			}
		}
	}
}

// The concept here is that when a connection starts writing it gets exclusive access
// until it doesn't write for this duration. This is to help reduce conflicts between writers.
const writeLockTimeout = 2 * time.Second

// connReadWorker reads from the connection and writes to the serial port.
func connReadWorker(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, portLock chan serio.OutPort) {
	lg := logctx.FromContext(ctx)
	defer wg.Done()
	defer lg.Debug("connReadDone")
	defer conn.Close()
	var port serio.OutPort
	defer func() {
		if port != nil {
			portLock <- port
		}
	}()
	for {
		buf := make([]byte, 1024)
		var deadline time.Time
		if port != nil {
			deadline = time.Now().Add(writeLockTimeout)
		}
		conn.SetReadDeadline(deadline)
		nRead, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				portLock <- port
				lg.Debug("lockRelease", "conn", conn)
				port = nil
				continue
			}
			logConnErr(ctx, "connReadErr", err)
			return
		}
		if nRead == 0 {
			continue
		}
		// acquire port lock
		if port == nil {
			var ok bool
			select {
			case <-ctx.Done():
				return
			case port, ok = <-portLock:
				if !ok {
					return
				}
				lg.Debug("lockAcquire", "conn", conn)
			}
		}
		nWritten, err := port.Write(buf[:nRead])
		if nWritten > 0 {
			lg.Debug("serialWrite", "nBytes", nWritten)
		}
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("serialWriteErr", err)
			}
			return
		}
		err = serio.Drain(ctx, port, nWritten)
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("serialDrainErr", err)
			}
			return
		}
	}
}

func logConnErr(ctx context.Context, msg string, err error) {
	if !errors.Is(err, net.ErrClosed) {
		logctx.FromContext(ctx).Error(msg, err)
	}
}
