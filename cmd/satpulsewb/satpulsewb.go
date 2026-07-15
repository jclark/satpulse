// Command satpulsewb serves SatPulse Workbench, a browser GUI for
// interactive GPS receiver configuration and monitoring. It wraps
// gps/app/session in an HTTP server: session methods become POST
// endpoints, snapshots become GET endpoints, and session events are
// streamed over SSE to the embedded single-page frontend.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jclark/satpulse/gps/app/cmd"
	"github.com/jclark/satpulse/gps/app/session"
	"github.com/jclark/satpulse/gps/gpsreg"
	"github.com/spf13/pflag"
)

// defaultPort is the canonical port (from the GPS L1 frequency,
// 1575.42 MHz), falling back to an OS-picked port when taken.
const defaultPort = 15754

const summary = `[-h|--help] [-L|--listen host:port] [-T|--token]
       [-d|--serial-device path [-s|--device-speed bps]] [--vendor name]
       [--packet-log path]`

type flagVars struct {
	listen      string
	token       bool
	device      string
	speed       int
	vendor      gpsreg.Vendor
	packetLog   string
	logLevel    slog.Level
	showVersion bool
}

func main() {
	progName := os.Args[0]
	v, usage, err := parseFlags(os.Args[1:])
	if err != nil {
		cmd.ErrPrintln(progName, err)
		fmt.Fprint(os.Stderr, usage(progName))
		os.Exit(2)
	}
	if v == nil {
		fmt.Fprint(os.Stderr, usage(progName))
		return
	}
	if v.showVersion {
		fmt.Fprintln(os.Stderr, cmd.VersionInfo())
		return
	}
	if err := run(v); err != nil {
		cmd.ErrPrintlnWithDetail(progName, err)
		os.Exit(1)
	}
}

// parseFlags parses and validates the command line. It returns a nil
// flagVars and a nil error when help was requested.
func parseFlags(args []string) (*flagVars, func(string) string, error) {
	var v flagVars
	var vendorStr string
	var verbose int
	var help bool
	flags := pflag.NewFlagSet("satpulsewb", pflag.ContinueOnError)
	flags.StringVarP(&v.listen, "listen", "L", "", "listen on `host:port` and disable the access token")
	flags.BoolVarP(&v.token, "token", "T", false, "require a generated access token even with --listen")
	flags.StringVarP(&v.device, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.IntVarP(&v.speed, "device-speed", "s", 0, "serial device baud-rate in `bps`")
	flags.StringVar(&vendorStr, "vendor", "", "GPS receiver `vendor` name")
	flags.StringVar(&v.packetLog, "packet-log", "", "log packets to `path`")
	flags.CountVarP(&verbose, "verbose", "v", "increase verbosity")
	flags.BoolVarP(&help, "help", "h", false, "show help")
	flags.BoolVarP(&v.showVersion, "version", "V", false, "show version information")
	usage := func(progName string) string {
		return fmt.Sprintf("Usage: %s %s\nOptions:\n%s", progName, summary, flags.FlagUsages())
	}
	if err := flags.Parse(args); err != nil {
		return nil, usage, err
	}
	if help {
		return nil, usage, nil
	}
	if flags.NArg() != 0 {
		return nil, usage, fmt.Errorf("satpulsewb must not have non-option arguments")
	}
	if flags.Lookup("device-speed").Changed && v.speed == 0 {
		return nil, usage, fmt.Errorf("0 is not a valid value for --device-speed")
	}
	if v.device == "" && flags.Lookup("device-speed").Changed {
		return nil, usage, fmt.Errorf("--device-speed requires --serial-device")
	}
	var err error
	if v.vendor, err = gpsreg.ParseVendor(vendorStr); err != nil {
		return nil, usage, err
	}
	// Default Info so connect and configure progress reaches the UI
	// activity log (and the terminal).
	v.logLevel = slog.LevelInfo
	if verbose > 0 {
		v.logLevel = slog.LevelDebug
	}
	return &v, usage, nil
}

func run(v *flagVars) error {
	hub := newSSEHub()
	base := cmd.NewDefaultLogger(os.Stderr, v.logLevel)
	lg := slog.New(session.NewLogHandler(hub, base.Handler()))
	ctx, cancel := cmd.CancelOnSignal(context.Background(), lg)
	defer cancel()
	opts := session.Options{}
	if v.packetLog != "" {
		f, err := os.Create(v.packetLog)
		if err != nil {
			return err
		}
		defer f.Close()
		opts.PacketLog = f
	}
	sess := session.New(lg, hub, opts)
	ln, err := listen(v.listen)
	if err != nil {
		return err
	}
	var token string
	if v.listen == "" || v.token {
		token = newToken()
	}
	printURLs(os.Stdout, ln, v.listen, token)
	if v.device != "" {
		go func() {
			if err := sess.Connect(session.SerialOpener{Device: v.device, Speed: v.speed}, v.vendor); err != nil {
				lg.Error("could not connect to the GPS", "device", v.device, "err", err)
			}
		}()
	}
	srv := newServer(ctx, sess, hub, lg, token, v.vendor, msgDirs())
	// --listen is expert mode and never opens a browser. In guided mode the
	// opened URL normally carries the multi-use token; on platforms whose argv
	// leaks it, mint a single-use token for the launch instead so the value
	// visible to other users is worthless once redeemed.
	if v.listen == "" && canOpenBrowser() {
		launchToken := token
		if launchBrowserLeaksURL {
			launchToken = srv.newSingleUseToken()
		}
		openBrowser(lg, ln, launchToken)
	}
	httpServer := &http.Server{Handler: srv.mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if httpServer.Shutdown(shutdownCtx) != nil {
			httpServer.Close()
		}
	}()
	err = httpServer.Serve(ln)
	sess.Disconnect()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// listen binds the server socket. With no --listen, it binds all
// interfaces on the default port, falling back to an OS-picked port;
// with an explicit --listen a bind failure is an error, since the
// address may be the target of an ssh tunnel.
func listen(listenFlag string) (net.Listener, error) {
	if listenFlag != "" {
		return net.Listen("tcp", listenFlag)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", defaultPort))
	if err == nil {
		return ln, nil
	}
	return net.Listen("tcp", ":0")
}

// newToken returns the per-run access token: 128 bits from crypto/rand,
// base64url encoded.
func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// printURLs prints one URL per address the server is reachable on:
// the explicit --listen host if one was given, otherwise every
// non-loopback interface address (or localhost when there is none).
func printURLs(w io.Writer, ln net.Listener, listenFlag, token string) {
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	var hosts []string
	if host, _, err := net.SplitHostPort(listenFlag); err == nil && host != "" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
			hosts = []string{host}
		}
	}
	if hosts == nil {
		hosts = nonLoopbackAddrs()
		if len(hosts) == 0 {
			hosts = []string{"localhost"}
		}
	}
	fmt.Fprintln(w, "Serving SatPulse Workbench at:")
	insecure := false
	for _, h := range hosts {
		u := "http://" + net.JoinHostPort(h, port) + "/"
		if token != "" {
			u += "?t=" + token
		}
		fmt.Fprintln(w, "  "+u)
		if token == "" && !isLoopbackHost(h) {
			insecure = true
		}
	}
	if insecure {
		fmt.Fprintln(w, "Warning: no access token on a non-loopback address; browsers will be refused, but clients can still control the receiver by sending a loopback Host (use -T to require a token and allow browser access).")
	}
}

func nonLoopbackAddrs() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var hosts []string
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.IsGlobalUnicast() {
			hosts = append(hosts, ipn.IP.String())
		}
	}
	return hosts
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") || strings.EqualFold(host, "localhost.") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
