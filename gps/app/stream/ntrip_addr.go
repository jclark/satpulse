package stream

import (
	"net"
	"strings"

	"github.com/jclark/satpulse/gps/app/ntrip"
)

// NtripNormalizeAddress returns addr with the default Ntrip port
// when no port is supplied.
func NtripNormalizeAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		if port != "" {
			return addr
		}
		return net.JoinHostPort(host, ntrip.DefaultPort)
	}
	if strings.HasPrefix(addr, "[") && strings.HasSuffix(addr, "]") {
		addr = strings.TrimSuffix(strings.TrimPrefix(addr, "["), "]")
	}
	return net.JoinHostPort(addr, ntrip.DefaultPort)
}
