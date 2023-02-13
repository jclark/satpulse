package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"

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
	listen, err := net.Listen("tcp", ":2006")
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

	b := newBcast()
	go b.run(ctx)
	go readWorker(ctx, port, b.msg)

	for {
		conn, err := listen.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go handleConn(ctx, b, conn)
	}
}

func handleConn(ctx context.Context, b *bcast, conn net.Conn) {
	defer conn.Close()
	ch := make(chan []byte)
	b.subscribe <- ch
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
				return
			}
		}
	}
}

func readWorker(ctx context.Context, port *serio.Port, ch chan<- []byte) {
	defer close(ch)
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
			lg.Error("readErr", err)
			return
		}
		if len(msg) > 0 {
			ch <- msg
		}
	}
}
