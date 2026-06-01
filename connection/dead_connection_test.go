package connection

import (
	"context"
	"net"
	"testing"
	"time"

	gatewayerrors "github.com/PointOfData/pod-os-go-client/errors"
)

// startTestServer starts a local TCP listener and invokes handle for each
// accepted connection. It returns the host, port, and a cleanup func.
func startTestServer(t *testing.T, handle func(conn net.Conn)) (string, string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handle(conn)
		}
	}()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return host, port, func() { _ = ln.Close() }
}

func newTestClient(t *testing.T, host, port string) *Client {
	t.Helper()
	retry := NewRetry(Retry{Retries: 0})
	c := NewClient(context.Background(), ClientConfig{
		ReceiveTimeout: 500 * time.Millisecond,
	}, "tcp", host, port, "test-actor", retry)
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	return c
}

// A corrupt length prefix can never be resynced on a framed stream, so it must
// be reported as a fatal connection-lost error and clear the connected flag.
func TestReceive_CorruptPrefix_IsFatal(t *testing.T) {
	host, port, cleanup := startTestServer(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte("!!garbage")) // 9 invalid prefix bytes
		time.Sleep(2 * time.Second)
		_ = conn.Close()
	})
	defer cleanup()

	c := newTestClient(t, host, port)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := c.Receive(ctx)
	if !gatewayerrors.IsConnectionLost(err) {
		t.Fatalf("expected ErrConnectionLost, got %v", err)
	}
	if c.IsConnected() {
		t.Fatal("expected connected=false after fatal framing error")
	}
}

// A peer that sends a valid length prefix but then closes (RST/EOF) mid-body
// must surface a fatal connection-lost error rather than hanging or returning a
// benign timeout.
func TestReceive_RstMidResponse_IsFatal(t *testing.T) {
	host, port, cleanup := startTestServer(t, func(conn net.Conn) {
		// Claim a 100-byte message, send only the prefix + a few body bytes,
		// then abruptly close.
		_, _ = conn.Write([]byte("x00000064")) // hex 0x64 = 100 total bytes
		_, _ = conn.Write([]byte("partial"))
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetLinger(0) // force RST on close
		}
		_ = conn.Close()
	})
	defer cleanup()

	c := newTestClient(t, host, port)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, _, err := c.Receive(ctx)
	if !gatewayerrors.IsConnectionLost(err) {
		t.Fatalf("expected ErrConnectionLost, got %v", err)
	}
	if c.IsConnected() {
		t.Fatal("expected connected=false after mid-frame RST")
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("detection too slow: %v", elapsed)
	}
}

// An idle connection (no bytes sent) must time out benignly and keep the
// connection marked as alive.
func TestReceive_IdleTimeout_IsBenign(t *testing.T) {
	host, port, cleanup := startTestServer(t, func(conn net.Conn) {
		time.Sleep(3 * time.Second) // stay silent but connected
		_ = conn.Close()
	})
	defer cleanup()

	c := newTestClient(t, host, port)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, _, err := c.Receive(ctx)
	if !gatewayerrors.IsIdleTimeout(err) {
		t.Fatalf("expected ErrReceiveIdleTimeout, got %v", err)
	}
	if !c.IsConnected() {
		t.Fatal("expected connected=true after benign idle timeout")
	}
}

// A write to a peer that has gone away must clear connected and return a fatal
// connection-lost error so caller retry loops engage.
func TestSend_AfterPeerClose_IsFatal(t *testing.T) {
	connClosed := make(chan struct{})
	host, port, cleanup := startTestServer(t, func(conn net.Conn) {
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetLinger(0)
		}
		_ = conn.Close()
		close(connClosed)
	})
	defer cleanup()

	c := newTestClient(t, host, port)
	defer c.Close()

	<-connClosed
	// The first write may succeed (buffered) before the RST is observed; retry a
	// few times to reliably surface the dead socket.
	var err *gatewayerrors.GatewayDError
	for i := 0; i < 50; i++ {
		_, err = c.Send([]byte("x00000009"))
		if err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !gatewayerrors.IsConnectionLost(err) {
		t.Fatalf("expected ErrConnectionLost from Send, got %v", err)
	}
	if c.IsConnected() {
		t.Fatal("expected connected=false after failed send")
	}
}
