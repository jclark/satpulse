package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/satpulse/internal/bcast"
	"github.com/jclark/satpulse/internal/scan"
	"github.com/jclark/satpulse/internal/serio"
)

type TCPConfig struct {
	Listen           string  `toml:"listen"`
	ReadOnly         bool    `toml:"readOnly"`
	NMEAOnly         bool    `toml:"nmeaOnly"`
	WriteLockTimeout float64 `toml:"writeLockTimeout"`
}

type tcpConnConfig struct {
	readOnly         bool
	nmeaOnly         bool
	writeLockTimeout time.Duration
}

func startTCP(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg []TCPConfig, b *bcast.Bcast[scan.Packet], port serio.OutPort) error {
	if len(cfg) == 0 {
		return nil
	}
	allReadOnly := true
	connConfigs := make([]tcpConnConfig, len(cfg))
	for i, c := range cfg {
		if c.Listen == "" {
			return errors.New("must specify listen option for each TCP element")
		}
		if !c.ReadOnly {
			allReadOnly = false
		}
		d, err := convertWriteLockTimeout(c.WriteLockTimeout)
		if err != nil {
			return err
		}
		connConfigs[i] = tcpConnConfig{readOnly: c.ReadOnly, nmeaOnly: c.NMEAOnly, writeLockTimeout: d}
	}
	listenCfg := net.ListenConfig{}
	listeners := make([]net.Listener, len(cfg))
	for i, c := range cfg {
		listen, err := listenCfg.Listen(ctx, "tcp", c.Listen)
		if err != nil {
			return err
		}
		listeners[i] = listen
	}
	var portLock chan serio.OutPort
	if !allReadOnly {
		portLock = make(chan serio.OutPort, 1)
		portLock <- port
	}
	for i, listen := range listeners {
		connConfig := connConfigs[i]
		listen := listen
		waitGroupGo(wg, func() {
			tcpHandleListen(ctx, lg, wg, connConfig, listen, b, portLock)
		})
	}
	go func() {
		<-ctx.Done()
		for _, listen := range listeners {
			listen.Close()
		}
	}()
	return nil
}

func convertWriteLockTimeout(secs float64) (time.Duration, error) {
	const secondsMax = float64(math.MaxInt64 / int64(time.Second))
	if secs > 0 && secs <= secondsMax {
		return time.Duration(secs * float64(time.Second)), nil
	}
	if secs == 0.0 {
		return writeLockTimeoutDefault, nil
	}
	// this handles NaN, negative, infinite, and too large
	return 0, fmt.Errorf("writeLockTimeout %f out of range", secs)
}

func tcpHandleListen(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg tcpConnConfig, listen net.Listener, b *bcast.Bcast[scan.Packet], portLock chan serio.OutPort) {
	defer lg.Debug("about to exit TCP listening goroutine")
	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			logConnErr(lg, "error accepting TCP connection", err)
			return
		}
		tcpHandleConn(ctx, lg, wg, cfg, conn, b, portLock)
	}
}

func tcpHandleConn(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg tcpConnConfig, conn net.Conn, b *bcast.Bcast[scan.Packet], portLock chan serio.OutPort) {
	// XXX both the read and write workers are closing the connection.
	// Not sure if it would better for just one of them to do so.
	waitGroupGo(wg, func() { tcpConnWriteWorker(ctx, lg, cfg, conn, b) })
	// The connReadWorker reads from the connection and writes to the serial port.
	// The readOnly config option says not to write to the serial port.
	if !cfg.readOnly {
		waitGroupGo(wg, func() { tcpConnReadWorker(ctx, lg, cfg, conn, portLock) })
	}
}

// tcpConnWriteWorker reads from a channel and write to the connection.
func tcpConnWriteWorker(ctx context.Context, lg *slog.Logger, cfg tcpConnConfig, conn net.Conn, b *bcast.Bcast[scan.Packet]) {
	defer lg.Debug("about to exit TCP connection writing worker goroutine")
	defer conn.Close()
	ch := b.Subscribe()
	defer b.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if cfg.nmeaOnly && msg.Kind != scan.NMEA {
				continue
			}
			_, err := conn.Write(([]byte)(msg.Data))
			if err != nil {
				logConnErr(lg, "error writing to TCP connection", err)
				return
			}
		}
	}
}

// The concept here is that when a connection starts writing it gets exclusive access
// until it doesn't write for this duration. This is to help reduce conflicts between writers.
const writeLockTimeoutDefault = 2 * time.Second

// tcpConnReadWorker reads from the connection and writes to the serial port.
func tcpConnReadWorker(ctx context.Context, lg *slog.Logger, cfg tcpConnConfig, conn net.Conn, portLock chan serio.OutPort) {
	defer lg.Debug("about to exit TCP connection reading worker goroutine")
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
			deadline = time.Now().Add(cfg.writeLockTimeout)
		}
		conn.SetReadDeadline(deadline)
		nRead, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				portLock <- port
				lg.Info("released the write lock on the serial port", "conn", conn)
				port = nil
				continue
			}
			logConnErr(lg, "error reading from TCP connection", err)
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
				lg.Info("acquired a write lock on the serial port", "conn", conn)
			}
		}
		nWritten, err := port.Write(buf[:nRead])
		if nWritten > 0 {
			lg.Debug("wrote to serial port", "nBytes", nWritten)
		}
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("error writing to serial port", "err", err)
			}
			return
		}
		err = serio.Drain(ctx, lg, port, nWritten)
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("error draining serial port", "err", err)
			}
			return
		}
	}
}

func logConnErr(lg *slog.Logger, msg string, err error) {
	if !errors.Is(err, net.ErrClosed) {
		lg.Error(msg, "err", err)
	}
}
