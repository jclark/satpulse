package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/gps4ptp/internal/logctx"
	"github.com/jclark/gps4ptp/internal/scan"
	"github.com/jclark/gps4ptp/internal/serio"
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

func startTCP(ctx context.Context, wg *sync.WaitGroup, cfg []TCPConfig, b *serio.Bcast, port serio.OutPort) error {
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
		wg.Add(1)
		go handleListen(ctx, wg, connConfigs[i], listen, b, portLock)
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

func handleListen(ctx context.Context, wg *sync.WaitGroup, cfg tcpConnConfig, listen net.Listener, b *serio.Bcast, portLock chan serio.OutPort) {
	defer wg.Done()
	defer logctx.FromContext(ctx).Debug("listenDone")
	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			logConnErr(ctx, "acceptErr", err)
			return
		}
		handleConn(ctx, wg, cfg, conn, b, portLock)
	}
}

func handleConn(ctx context.Context, wg *sync.WaitGroup, cfg tcpConnConfig, conn net.Conn, b *serio.Bcast, portLock chan serio.OutPort) {
	// XXX both the read and write workers are closing the connection.
	// Not sure if it would better for just one of them to do so.
	wg.Add(1)
	go connWriteWorker(ctx, wg, cfg, conn, b)
	// The connReadWorker reads from the connection and writes to the serial port.
	// The readOnly config option says not to write to the serial port.
	if !cfg.readOnly {
		wg.Add(1)
		go connReadWorker(ctx, wg, cfg, conn, portLock)
	}
}

// connWriteWorker reads from a channel and write to the connection.
func connWriteWorker(ctx context.Context, wg *sync.WaitGroup, cfg tcpConnConfig, conn net.Conn, b *serio.Bcast) {
	defer wg.Done()
	defer logctx.FromContext(ctx).Debug("connWriteDone")
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
				logConnErr(ctx, "connWriteErr", err)
				return
			}
		}
	}
}

// The concept here is that when a connection starts writing it gets exclusive access
// until it doesn't write for this duration. This is to help reduce conflicts between writers.
const writeLockTimeoutDefault = 2 * time.Second

// connReadWorker reads from the connection and writes to the serial port.
func connReadWorker(ctx context.Context, wg *sync.WaitGroup, cfg tcpConnConfig, conn net.Conn, portLock chan serio.OutPort) {
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
			deadline = time.Now().Add(cfg.writeLockTimeout)
		}
		conn.SetReadDeadline(deadline)
		nRead, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				portLock <- port
				lg.Info("serLockRelease", "conn", conn)
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
				lg.Info("serLockAcquire", "conn", conn)
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
