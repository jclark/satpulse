package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/jclark/gps2phc/logctx"
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
	ctx, cancel := context.WithCancel(ctx)

	err := run(ctx, cancel)
	if err != nil {
		fmt.Fprintln(os.Stderr, os.Args[0]+":", err)
		os.Exit(1)
	}
}

func cancelOnSignal(lg *slog.Logger, cancel context.CancelFunc) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, unix.SIGTERM)
	go func() {
		<-sig
		lg.Debug("cancelling")
		cancel()
	}()
}

func run(ctx context.Context, cancel context.CancelFunc) error {
	cfg := net.ListenConfig{}
	listen, err := cfg.Listen(ctx, "tcp", ":2006")
	if err != nil {
		return err
	}
	lg := logctx.FromContext(ctx)
	cancelOnSignal(lg, func() {
		listen.Close()
		cancel()
	})
	defer listen.Close()

	t, err := serio.OpenTerm(serialDev)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	b := newBcast()
	wg.Add(1)
	go b.run(ctx, &wg)
	wg.Add(1)
	go readWorker(ctx, &wg, t, b.msg)
	portLock := make(chan serio.OutPort, 1)
	portLock <- t
	wg.Add(1)
	go handleListen(ctx, &wg, listen, b, portLock)
	wg.Wait()
	return nil
}

func handleListen(ctx context.Context, wg *sync.WaitGroup, listen net.Listener, b *bcast, portLock chan serio.OutPort) {
	defer listen.Close()
	defer logctx.FromContext(ctx).Debug("listenDone")
	defer wg.Done()
	for {
		conn, err := listen.Accept()
		if err != nil {
			logConnErr(ctx, "acceptErr", err)
			close(b.subscribe)
			return
		}
		handleConn(ctx, wg, conn, b, portLock)
	}
}

func handleConn(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *bcast, portLock chan serio.OutPort) {
	wg.Add(1)
	// subscribe in the goroutine that closes the subscribe channel to avoid a race
	ch := make(chan []byte)
	b.subscribe <- ch
	go connWriteWorker(ctx, wg, conn, b, ch)
	go connReadWorker(ctx, wg, conn, portLock)
}

// connWriteWorker reads from a channel and write to the connection.
func connWriteWorker(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *bcast, ch chan []byte) {
	defer conn.Close()
	defer logctx.FromContext(ctx).Debug("connWriteDone")
	defer wg.Done()

	defer func() {
		b.unsubscribe <- ch
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err := conn.Write(msg)
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
	defer conn.Close()
	defer lg.Debug("connReadDone")
	defer wg.Done()
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

const serReadBufSize = 1024

func readWorker(ctx context.Context, wg *sync.WaitGroup, r io.Reader, ch chan<- []byte) {
	defer close(ch)
	defer logctx.FromContext(ctx).Debug("readWorkerDone")
	defer wg.Done()
	lg := logctx.FromContext(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		buf := make([]byte, serReadBufSize)
		nRead, err := r.Read(buf)
		msg := buf[:nRead]
		if err != nil {
			lg.Error("serialReadErr", err)
			return
		}
		if len(msg) > 0 {
			ch <- msg
		}
	}
}
