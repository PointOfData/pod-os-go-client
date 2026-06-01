//go:build !linux

package connection

import "net"

// setTCPUserTimeout is a no-op on platforms without TCP_USER_TIMEOUT.
// Dead-connection detection there relies on the keepalive probe settings and
// the receive-loop liveness deadline.
func setTCPUserTimeout(conn net.Conn, ms int) error {
	return nil
}
