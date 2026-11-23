package sockrefclock

import "net"

// listenUnixgramUnbound returns a unixgram socket with no local filesystem path.
// An empty name uses Linux autobind (abstract namespace), so there is no inode
// to trip AppArmor (#165) or to clean up. chrony does not reply, so the socket
// only needs to send.
func listenUnixgramUnbound() (*net.UnixConn, error) {
	return net.ListenUnixgram("unixgram", &net.UnixAddr{Name: "", Net: "unixgram"})
}
