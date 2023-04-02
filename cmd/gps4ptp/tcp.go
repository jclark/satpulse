package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/jclark/gps4ptp/internal/logctx"
	"github.com/jclark/gps4ptp/internal/serio"
)

func startTCP(ctx context.Context, wg *sync.WaitGroup, cfg TCPConfig, b *serio.Bcast, port serio.OutPort) error {
	if cfg.Port == 0 {
		return nil
	}
	listenCfg := net.ListenConfig{}
	listen, err := listenCfg.Listen(ctx, "tcp", fmt.Sprintf("%s:%d", cfg.Address, cfg.Port))
	if err != nil {
		return err
	}
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
	go connWriteWorker(ctx, wg, conn, b)
	wg.Add(1)
	go connReadWorker(ctx, wg, conn, portLock)
}

// connWriteWorker reads from a channel and write to the connection.
func connWriteWorker(ctx context.Context, wg *sync.WaitGroup, conn net.Conn, b *serio.Bcast) {
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
