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

const summary = `[-h|--help] [-v|--verbose] [-L|--listen host:port] [-t|--token]
       [-n|--no-open-browser]
       [-d|--serial-device path] [-s|--device-speed bps] [--vendor name]
       [--packet-log path]`

type flagVars struct {
	listen      string
	token       bool
	noOpen      bool
	device      string
	speed       int
	autoConnect bool
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
	flags.BoolVarP(&v.token, "token", "t", false, "require a generated access token even with --listen")
	flags.BoolVarP(&v.noOpen, "no-open-browser", "n", false, "do not open a web browser at startup")
	flags.StringVarP(&v.device, "serial-device", "d", "", "serial device connected to GPS receiver")
	flags.IntVarP(&v.speed, "device-speed", "s", 9600, "serial device baud-rate in `bps`")
	flags.StringVar(&vendorStr, "vendor", "", "GPS receiver `vendor` name")
	flags.StringVar(&v.packetLog, "packet-log", "", "log packets to `path`")
	flags.BoolFuncP("verbose", "v", "increase logging verbosity", func(string) error {
		verbose++
		return nil
	})
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
	if flags.Lookup("device-speed").Changed && v.speed <= 0 {
		return nil, usage, fmt.Errorf("--device-speed must be greater than zero")
	}
	v.autoConnect = flags.Lookup("serial-device").Changed && flags.Lookup("device-speed").Changed
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
	vendors, err := cmd.ResolveVendors(v.vendor)
	if err != nil {
		return err
	}
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
	printURLs(os.Stdout, lg, ln, v.listen, token)
	srv := newServer(ctx, sess, hub, lg, token, vendors, msgDirs(), v.device, v.speed)
	if v.autoConnect {
		go func() {
			if err := srv.connect(v.device, v.speed); err != nil {
				lg.Error("could not connect to the GPS", "device", v.device, "err", err)
			}
		}()
	}
	// --listen is expert mode and never opens a browser; --no-open-browser
	// suppresses it in guided mode too (headless hosts, scripting, tests). In
	// guided mode the opened URL normally carries the multi-use token; on
	// platforms whose argv leaks it, mint a single-use token for the launch
	// instead so the value visible to other users is worthless once redeemed.
	if v.listen == "" && !v.noOpen && canOpenBrowser() {
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
func listen(listenAddr string) (net.Listener, error) {
	if listenAddr != "" {
		return net.Listen("tcp", listenAddr)
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

// printURLs prints one URL per host from urlHosts, with the access
// token as a query parameter when there is one, warning on lg when a
// tokenless non-loopback URL was printed.
func printURLs(w io.Writer, lg *slog.Logger, ln net.Listener, listenAddr, token string) {
	addr := ln.Addr().(*net.TCPAddr)
	port := strconv.Itoa(addr.Port)
	hosts := urlHosts(addr.IP, listenAddr)
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
		lg.Warn("no access token on a non-loopback address; browsers will be refused, but clients can still control the receiver by sending a loopback Host (use -t to require a token and allow browser access)")
	}
}

// urlHosts returns the hosts, names or address literals, to print
// URLs for: the explicit --listen host if one was given, otherwise
// the loopback hosts (see loopbackHosts) followed by every
// non-loopback interface address the bind serves, skipping IPv6
// link-local addresses, which have no zone-free URL form.
func urlHosts(bind net.IP, listenAddr string) []string {
	if host, _, err := net.SplitHostPort(listenAddr); err == nil && host != "" {
		if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
			return []string{host}
		}
	}
	var nets []*net.IPNet
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				nets = append(nets, ipn)
			}
		}
	}
	v4only := bind.To4() != nil
	hosts := loopbackHosts(nets, v4only)
	for _, n := range nets {
		ip := n.IP
		if ip.IsLoopback() || (ip.To4() == nil && (v4only || ip.IsLinkLocalUnicast())) {
			continue
		}
		hosts = append(hosts, ip.String())
	}
	return hosts
}

// loopbackHosts returns the loopback part of the host list: localhost
// when every address it resolves to on this machine is loopback and
// served, else the canonical served literal, else every served
// loopback interface address. A loopback URL is only usable in a
// browser on this machine, so resolving localhost here tests exactly
// the environment the URL will be used in; a browser may try any
// resolved address, so all of them must reach the server, and all
// must be loopback so that the printed name means what it says.
func loopbackHosts(nets []*net.IPNet, v4only bool) []string {
	// served reports whether the bind covers ip: the address must be
	// local and match the wildcard's family (0.0.0.0 serves only v4;
	// the :: wildcard is dual-stack, since listen() uses network
	// "tcp", for which Go turns IPV6_V6ONLY off). Locality is prefix
	// containment, not equality: the kernel serves all of 127.0.0.1/8
	// on lo, so e.g. 127.0.1.1 is served without being an interface
	// address.
	served := func(ip net.IP) bool {
		if v4only && ip.To4() == nil {
			return false
		}
		for _, n := range nets {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	if ips, err := net.LookupIP("localhost"); err == nil && len(ips) > 0 {
		ok := true
		for _, ip := range ips {
			if !ip.IsLoopback() || !served(ip) {
				ok = false
				break
			}
		}
		if ok {
			return []string{"localhost"}
		}
	}
	if served(net.IPv4(127, 0, 0, 1)) {
		return []string{"127.0.0.1"}
	}
	if served(net.IPv6loopback) {
		return []string{"::1"}
	}
	var hosts []string
	for _, n := range nets {
		if n.IP.IsLoopback() && served(n.IP) {
			hosts = append(hosts, n.IP.String())
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
