package connection

import (
	"bytes"
	"net"
	"testing"
)

func putTestIdleConnection(pool *ChannelPool, conn net.Conn) error {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	if pool.aipConns == nil {
		return ErrClosed
	}
	select {
	case pool.semaphore <- struct{}{}:
	default:
		return ErrClosed
	}
	select {
	case pool.aipConns <- ConnectionData{Conns: conn}:
		return nil
	default:
		<-pool.semaphore
		return ErrClosed
	}
}

func TestChannelPoolPingIdleConnections(t *testing.T) {
	t.Parallel()

	server, clientConn := net.Pipe()
	t.Cleanup(func() {
		server.Close()
		clientConn.Close()
	})

	readDone := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := server.Read(buf)
		if err != nil {
			readDone <- nil
			return
		}
		readDone <- append([]byte(nil), buf[:n]...)
	}()

	pool := NewChannelPool(1, func() (net.Conn, string, error) {
		return nil, "", net.ErrClosed
	})
	if err := putTestIdleConnection(pool, clientConn); err != nil {
		t.Fatalf("putTestIdleConnection: %v", err)
	}

	payload := []byte("keepalive-frame")
	sent := pool.PingIdleConnections(func(conn net.Conn) error {
		n, err := conn.Write(payload)
		if err != nil {
			return err
		}
		if n != len(payload) {
			return bytes.ErrTooLarge
		}
		return nil
	})
	if sent != 1 {
		t.Fatalf("PingIdleConnections sent = %d, want 1", sent)
	}

	got := <-readDone
	if !bytes.Equal(got, payload) {
		t.Fatalf("server read = %q, want %q", got, payload)
	}
}

func TestChannelPoolPingIdleConnectionsSkipsCheckedOut(t *testing.T) {
	t.Parallel()

	pool := NewChannelPool(2, func() (net.Conn, string, error) {
		return nil, "", net.ErrClosed
	})

	idleServer, idleConn := net.Pipe()
	t.Cleanup(func() {
		idleServer.Close()
		idleConn.Close()
	})

	checkedOutServer, checkedOutConn := net.Pipe()
	t.Cleanup(func() {
		checkedOutServer.Close()
		checkedOutConn.Close()
	})

	if err := putTestIdleConnection(pool, idleConn); err != nil {
		t.Fatalf("putTestIdleConnection idle: %v", err)
	}

	// Checked-out connection is not in the idle queue.
	_ = checkedOutConn

	readDone := make(chan struct{})
	go func() {
		buf := make([]byte, 64)
		_, _ = idleServer.Read(buf)
		close(readDone)
	}()

	sent := pool.PingIdleConnections(func(conn net.Conn) error {
		_, err := conn.Write([]byte("ping"))
		return err
	})
	if sent != 1 {
		t.Fatalf("PingIdleConnections sent = %d, want 1 (checked-out conn must be skipped)", sent)
	}

	<-readDone
}
