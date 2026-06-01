//go:build linux

package connection

import (
	"net"
	"syscall"
)

// tcpUserTimeout is the TCP_USER_TIMEOUT socket option (Linux, IPPROTO_TCP level).
// It bounds how long transmitted data may remain unacknowledged before the
// connection is forcibly closed, so writes to a half-dead peer fail fast instead
// of blocking until the OS default (minutes).
const tcpUserTimeout = 0x12 // 18

// setTCPUserTimeout sets TCP_USER_TIMEOUT (in milliseconds) on the connection.
// A no-op on non-Linux platforms (see tcp_other.go).
func setTCPUserTimeout(conn net.Conn, ms int) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil
	}
	raw, err := tcpConn.SyscallConn()
	if err != nil {
		return err
	}
	var setErr error
	if err := raw.Control(func(fd uintptr) {
		setErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, tcpUserTimeout, ms)
	}); err != nil {
		return err
	}
	return setErr
}
