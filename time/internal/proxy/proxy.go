package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"

	"github.com/jclark/satpulse/gps/app/bcast"
	"github.com/jclark/satpulse/gps/app/gpsio"
	"github.com/jclark/satpulse/gps/gpsprot"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/jclark/satpulse/gps/scan"
)

type Config struct {
	TCP    []TCPService    `toml:"tcp"`
	Socket []SocketService `toml:"socket"`
}

type Options struct {
	ReadOnly         *bool           `toml:"readOnly"`
	Protocol         gpsreg.Protocol `toml:"protocol"`
	WriteLockTimeout float64         `toml:"writeLockTimeout"`
}

type TCPService struct {
	Options
	Listen string `toml:"listen"`
}

type SocketService struct {
	Options
	Path  string `toml:"path"`
	Group string `toml:"group"`
}

type svcConfig struct {
	network          string
	address          string
	readOnly         bool
	tag              gpsprot.Tag
	writeLockTimeout time.Duration
}

func Start(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg Config, b *bcast.Bcast[scan.Packet], portLock gpsio.OutPortLock) error {
	nSocket := len(cfg.Socket)
	nSvc := nSocket + len(cfg.TCP)
	if nSvc == 0 {
		return nil
	}
	allReadOnly := true
	svcConfigs := make([]svcConfig, nSvc)
	for i := range svcConfigs {
		sc := &svcConfigs[i]
		if i < nSocket {
			svc := cfg.Socket[i]
			if svc.Path == "" {
				return errors.New("must specify path for each socket proxy")
			}
			sc.address = svc.Path
			// this is generally good practice, I believe
			_ = os.Remove(svc.Path)
			sc.network = "unix"
			sc.setOptions(svc.Options)
		} else {
			svc := cfg.TCP[i-nSocket]
			if svc.Listen == "" {
				return errors.New("must specify listen for each tcp proxy")
			}
			sc.address = svc.Listen
			sc.network = "tcp"
			sc.setOptions(svc.Options)
		}
		if !sc.readOnly {
			allReadOnly = false
		}
	}
	listenCfg := net.ListenConfig{}
	listeners := make([]net.Listener, nSvc)
	for i, sc := range svcConfigs {
		listen, err := listenCfg.Listen(ctx, sc.network, sc.address)
		if err != nil {
			return err
		}
		if i < nSocket {
			err = setSocketPerms(lg, cfg.Socket[i], sc)
			if err != nil {
				return err
			}
		}
		listeners[i] = listen
	}
	if allReadOnly {
		portLock = nil
	} else if portLock == nil {
		panic("proxy.Start: non-nil portLock required when any proxy service is writable")
	}
	for i, listen := range listeners {
		sc := svcConfigs[i]
		listen := listen
		wg.Go(func() {
			handleListen(ctx, lg, wg, sc, listen, b, portLock)
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

func (s *svcConfig) setOptions(opts Options) error {
	d, err := convertWriteLockTimeout(opts.WriteLockTimeout)
	if err != nil {
		return err
	}
	s.writeLockTimeout = d
	s.tag = opts.Protocol.Tag()
	if opts.ReadOnly != nil {
		s.readOnly = *opts.ReadOnly
	} else {
		// default to readOnly for protocol-specific services
		s.readOnly = !s.tag.IsZero()
	}
	return nil
}

func setSocketPerms(lg *slog.Logger, sock SocketService, sc svcConfig) error {
	mode := os.FileMode(0660)
	if sc.readOnly {
		mode = os.FileMode(0666)
	}
	err := os.Chmod(sock.Path, mode)
	if err != nil {
		return err
	}
	if sc.readOnly {
		return nil
	}
	groupName := sock.Group
	if groupName == "" {
		groupName = defaultGroupName
	}
	group, err := user.LookupGroup(groupName)
	if err != nil {
		if sock.Group == "" {
			lg.Info("default group does not exist", "name", defaultGroupName)
			return nil
		}
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	return os.Chown(sock.Path, -1, gid)
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

func handleListen(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg svcConfig, listen net.Listener, b *bcast.Bcast[scan.Packet], portLock gpsio.OutPortLock) {
	defer lg.Debug("about to exit proxy listening goroutine")
	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			logConnErr(lg, "error accepting proxy network connection", err)
			return
		}
		handleConn(ctx, lg, wg, cfg, conn, b, portLock)
	}
}

func handleConn(ctx context.Context, lg *slog.Logger, wg *sync.WaitGroup, cfg svcConfig, conn net.Conn, b *bcast.Bcast[scan.Packet], portLock gpsio.OutPortLock) {
	lg.Info("accepted proxy connection", "remoteAddr", conn.RemoteAddr())
	// XXX both the read and write workers are closing the connection.
	// Not sure if it would better for just one of them to do so.
	wg.Go(func() { connWriteWorker(ctx, lg, cfg, conn, b) })
	// The connReadWorker reads from the connection and writes to the serial port.
	// The readOnly config option says not to write to the serial port.
	if !cfg.readOnly {
		wg.Go(func() { connReadWorker(ctx, lg, cfg, conn, portLock) })
	}
}

// connWriteWorker reads from a channel and write to the connection.
func connWriteWorker(ctx context.Context, lg *slog.Logger, cfg svcConfig, conn net.Conn, b *bcast.Bcast[scan.Packet]) {
	defer lg.Debug("about to exit proxy connection writing worker goroutine")
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
			// when forwarding is restricted to a specific protocol, don't forward packets with an invalid checksum;
			// forwarding in this case is semantic, so there's no point in forwarding a packeet that will be discarded
			if !cfg.tag.IsZero() && (!msg.HasTag(cfg.tag) || !msg.ChecksumValid) {
				continue
			}
			_, err := conn.Write(([]byte)(msg.Data))
			if err != nil {
				logConnErr(lg, "error writing to proxy connection", err)
				return
			}
		}
	}
}

// The concept here is that when a connection starts writing it gets exclusive access
// until it doesn't write for this duration. This is to help reduce conflicts between writers.
const writeLockTimeoutDefault = 2 * time.Second

// connReadWorker reads from the connection and writes to the serial port.
func connReadWorker(ctx context.Context, lg *slog.Logger, cfg svcConfig, conn net.Conn, portLock gpsio.OutPortLock) {
	defer lg.Debug("about to exit proxy connection reading worker goroutine")
	defer conn.Close()
	var port gpsio.OutPort
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
				lg.Debug("released the write lock on the serial port", "conn", conn)
				port = nil
				continue
			}
			if err, ok := err.(net.Error); ok && err.Timeout() {
				continue
			}
			if !errors.Is(err, io.EOF) {
				logConnErr(lg, "error reading from proxy connection", err)
			}
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
				lg.Debug("acquired a write lock on the serial port", "conn", conn)
			}
		}
		nWritten, err := port.Write(buf[:nRead])
		if nWritten > 0 {
			lg.Debug("wrote to serial port", "nBytes", nWritten, "data", buf[:nWritten])
		}
		if err != nil {
			if ctx.Err() == nil {
				lg.Error("error writing to serial port", "err", err)
			}
			return
		}
		err = gpsio.Drain(ctx, lg, port, nWritten)
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
