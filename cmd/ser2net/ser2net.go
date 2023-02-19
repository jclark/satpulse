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

	port, err := serio.Open(serialDev)
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	b := newBcast()
	wg.Add(1)
	go b.run(ctx, &wg)
	wg.Add(1)
	go readWorker(ctx, &wg, port, b.msg)
	chLock, ch := makeChanLock()
	wg.Add(1)
	go writeWorker(ctx, &wg, port, ch)
	wg.Add(1)
	go handleListen(ctx, &wg, listen, b, chLock)
	wg.Wait()
	return nil
}

func handleListen(ctx context.Context, wg *sync.WaitGroup, listen net.Listener, b *bcast, chLock chan chan<- []byte) {
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
		startConnWrite(ctx, wg, conn, b)
		wg.Add(1)
		go handleConnRead(ctx, wg, conn, chLock)
	}
}

func startConnWrite(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *bcast) {
	wg.Add(1)
	// subscribe in the goroutine that closes the subscribe channel to avoid a race
	ch := make(chan []byte)
	b.subscribe <- ch
	go handleConnWrite(ctx, wg, conn, b, ch)
}

func handleConnWrite(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *bcast, ch chan []byte) {
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

// The concept here is that if a connection starts writing it gets exclusive access
// until it doesn't write for this duration. This is to help reduce conflicts between writers.
const writeLockTimeout = 2 * time.Second

func handleConnRead(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, chLock chan chan<- []byte) {
	defer conn.Close()
	defer logctx.FromContext(ctx).Debug("connReadDone")
	defer wg.Done()
	var ch chan<- []byte
	defer func() {
		if ch != nil {
			chLock <- ch
		}
	}()
	for {
		buf := make([]byte, 1024)
		var deadline time.Time
		if ch != nil {
			deadline = time.Now().Add(writeLockTimeout)
		}
		conn.SetReadDeadline(deadline)
		n, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				chLock <- ch
				logctx.FromContext(ctx).Debug("lockRelease", "conn", conn)
				ch = nil
				continue
			}
			logConnErr(ctx, "connReadErr", err)
			return
		}
		if n == 0 {
			continue
		}
		// acquire channel lock
		if ch == nil {
			var ok bool
			select {
			case <-ctx.Done():
				return
			case ch, ok = <-chLock:
				if !ok {
					return
				}
				logctx.FromContext(ctx).Debug("lockAcquire", "conn", conn)
			}
		}
		select {
		case <-ctx.Done():
			return
		case ch <- buf[:n]:
		}
	}
}

func makeChanLock() (chan chan<- []byte, <-chan []byte) {
	chLock := make(chan chan<- []byte, 1)
	ch := make(chan []byte)
	chLock <- ch
	return chLock, ch
}

func logConnErr(ctx context.Context, msg string, err error) {
	if !errors.Is(err, net.ErrClosed) {
		logctx.FromContext(ctx).Error(msg, err)
	}
}

func readWorker(ctx context.Context, wg *sync.WaitGroup, port *serio.Port, ch chan<- []byte) {
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
		buf := make([]byte, 1024)
		nRead, err := port.Read(buf)
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

func writeWorker(ctx context.Context, wg *sync.WaitGroup, port *serio.Port, ch <-chan []byte) {
	defer logctx.FromContext(ctx).Debug("writeWorkerDone")
	defer wg.Done()
	lg := logctx.FromContext(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, err := port.Write(ctx, msg)
			if err != nil {
				lg.Error("serialWriteErr", err)
				return
			}
			lg.Debug("serialWrite", "nBytes", len(msg))
		}
	}
}
