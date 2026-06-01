package podos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/PointOfData/pod-os-go-client/connection"
	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
)

// newTestConnClient creates a connection.Client connected to a local TCP listener.
// Returns the client and a cleanup function that closes the listener.
func newTestConnClient(t *testing.T) (*connection.Client, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	retry := connection.NewRetry(connection.Retry{Retries: 1, Backoff: time.Millisecond})
	cc := connection.NewClient(
		context.Background(),
		connection.ClientConfig{Logger: log.NoOpLogger{}},
		"tcp", "127.0.0.1", fmt.Sprintf("%d", addr.Port),
		"test-actor", retry,
	)
	if cc == nil {
		ln.Close()
		t.Fatal("connection.NewClient returned nil")
	}
	return cc, func() { ln.Close() }
}

// newTestClient builds a minimal podos.Client suitable for unit-testing state
// and reconnect primitives without a real gateway.
func newTestClient(t *testing.T, conn *connection.Client) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		conn:           conn,
		logger:         log.NoOpLogger{},
		receiverCtx:    ctx,
		receiverCancel: cancel,
		pending:        make(map[string]chan *message.Message),
		pendingWithRaw: make(map[string]chan *responseWithRaw),
	}
	c.reconnectCond = sync.NewCond(&c.reconnectingMu)
	return c
}

// ---------------------------------------------------------------------------
// ConnectionState.String()
// ---------------------------------------------------------------------------

func TestConnectionStateString(t *testing.T) {
	tests := []struct {
		state ConnectionState
		want  string
	}{
		{StateConnected, "connected"},
		{StateDisconnected, "disconnected"},
		{StateReconnecting, "reconnecting"},
		{StateReconnectFailed, "reconnect_failed"},
		{ConnectionState(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.state.String(); got != tt.want {
				t.Errorf("ConnectionState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// emitState / OnConnectionStateChange
// ---------------------------------------------------------------------------

func TestEmitState_NoHandler(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)
	// Should not panic when no handler is registered.
	c.emitState(StateDisconnected, errors.New("boom"))
}

func TestOnConnectionStateChange(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	var mu sync.Mutex
	var transitions []ConnectionState
	var errs []error

	c.OnConnectionStateChange(func(state ConnectionState, err error) {
		mu.Lock()
		transitions = append(transitions, state)
		errs = append(errs, err)
		mu.Unlock()
	})

	trigger := errors.New("EOF")
	c.emitState(StateDisconnected, trigger)
	c.emitState(StateReconnecting, trigger)
	c.emitState(StateConnected, nil)

	mu.Lock()
	defer mu.Unlock()

	if len(transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(transitions))
	}
	if transitions[0] != StateDisconnected {
		t.Errorf("transitions[0] = %v, want StateDisconnected", transitions[0])
	}
	if transitions[1] != StateReconnecting {
		t.Errorf("transitions[1] = %v, want StateReconnecting", transitions[1])
	}
	if transitions[2] != StateConnected {
		t.Errorf("transitions[2] = %v, want StateConnected", transitions[2])
	}
	if errs[0] != trigger {
		t.Errorf("errs[0] = %v, want %v", errs[0], trigger)
	}
	if errs[2] != nil {
		t.Errorf("errs[2] = %v, want nil", errs[2])
	}
}

func TestOnConnectionStateChange_Replace(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	called1 := false
	called2 := false
	c.OnConnectionStateChange(func(ConnectionState, error) { called1 = true })
	c.OnConnectionStateChange(func(ConnectionState, error) { called2 = true })

	c.emitState(StateDisconnected, nil)

	if called1 {
		t.Error("first handler should have been replaced")
	}
	if !called2 {
		t.Error("second handler should have been called")
	}
}

// ---------------------------------------------------------------------------
// waitForReconnect
// ---------------------------------------------------------------------------

func TestWaitForReconnect_AlreadyConnected(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	if !c.waitForReconnect(context.Background()) {
		t.Error("waitForReconnect should return true when already connected")
	}
}

func TestWaitForReconnect_ContextCancelled(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	// Disconnect the underlying conn so IsConnected returns false.
	cc.Close()
	// Simulate an in-progress reconnect so the waiter enters the cond loop.
	c.reconnectingMu.Lock()
	c.reconnecting = true
	c.reconnectingMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := c.waitForReconnect(ctx)
	elapsed := time.Since(start)

	if result {
		t.Error("waitForReconnect should return false when context is cancelled")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForReconnect took %v, expected ~50ms", elapsed)
	}
}

func TestWaitForReconnect_NotReconnecting(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)
	cc.Close()

	// reconnecting=false, not connected, not closed → should return false immediately
	result := c.waitForReconnect(context.Background())
	if result {
		t.Error("waitForReconnect should return false when not reconnecting and not connected")
	}
}

func TestWaitForReconnect_Closed(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)
	cc.Close()

	c.reconnectingMu.Lock()
	c.closed = true
	c.reconnectingMu.Unlock()

	result := c.waitForReconnect(context.Background())
	if result {
		t.Error("waitForReconnect should return false when client is closed")
	}
}

// ---------------------------------------------------------------------------
// attemptReconnection — closed guard
// ---------------------------------------------------------------------------

func TestAttemptReconnection_ClosedReturnsImmediately(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	c.reconnectingMu.Lock()
	c.closed = true
	c.reconnectingMu.Unlock()

	if c.attemptReconnection(nil) {
		t.Error("attemptReconnection should return false when client is closed")
	}
}

func TestAttemptReconnection_EmitsStates(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	// Close the connection so reconnect will fail.
	cc.Close()

	// Set up a minimal reconnect config with 1 retry and very short backoff
	// so the test doesn't hang. Connection will fail because the conn is closed
	// and the underlying Reconnect() will error.
	enabled := true
	c.cfg.ReconnectConfig.Enabled = &enabled
	c.cfg.ReconnectConfig.MaxRetries = 1
	c.cfg.ReconnectConfig.InitialBackoff = 10 * time.Millisecond
	c.cfg.ReconnectConfig.MaxBackoff = 10 * time.Millisecond

	var mu sync.Mutex
	var states []ConnectionState

	c.OnConnectionStateChange(func(state ConnectionState, _ error) {
		mu.Lock()
		states = append(states, state)
		mu.Unlock()
	})

	trigger := errors.New("EOF")
	c.attemptReconnection(trigger)

	mu.Lock()
	defer mu.Unlock()

	// Should see at least StateReconnecting and StateReconnectFailed
	if len(states) < 2 {
		t.Fatalf("expected at least 2 state transitions, got %d: %v", len(states), states)
	}
	if states[0] != StateReconnecting {
		t.Errorf("states[0] = %v, want StateReconnecting", states[0])
	}
	last := states[len(states)-1]
	if last != StateReconnectFailed && last != StateConnected {
		t.Errorf("last state = %v, want StateReconnectFailed or StateConnected", last)
	}
}

// ---------------------------------------------------------------------------
// Close sets closed flag and broadcasts
// ---------------------------------------------------------------------------

func TestClose_SetsClosedFlag(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)

	if err := c.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	c.reconnectingMu.Lock()
	closed := c.closed
	c.reconnectingMu.Unlock()

	if !closed {
		t.Error("Close() should set closed=true")
	}
}

// ---------------------------------------------------------------------------
// Server reset — full receiveLoop integration
// ---------------------------------------------------------------------------

// TestServerReset_ReceiveLoopEmitsDisconnected verifies that when a remote
// server issues a TCP RST (connection reset by peer), the receiveLoop detects
// the fatal error, calls handleConnectionLost, and emits StateDisconnected
// through the state observer callback.
func TestServerReset_ReceiveLoopEmitsDisconnected(t *testing.T) {
	accepted := make(chan net.Conn, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	retry := connection.NewRetry(connection.Retry{Retries: 0})
	cc := connection.NewClient(
		context.Background(),
		connection.ClientConfig{
			Logger:         log.NoOpLogger{},
			ReceiveTimeout: 500 * time.Millisecond,
		},
		"tcp", "127.0.0.1", fmt.Sprintf("%d", addr.Port),
		"test-actor", retry,
	)
	if cc == nil {
		t.Fatal("connection.NewClient returned nil")
	}

	c := newTestClient(t, cc)
	c.cfg.ReceiveTimeout = 500 * time.Millisecond

	stateCh := make(chan ConnectionState, 10)
	c.OnConnectionStateChange(func(state ConnectionState, _ error) {
		stateCh <- state
	})

	c.sendMu.Lock()
	c.receiverActive = true
	c.sendMu.Unlock()
	c.receiverWg.Add(1)
	go func() {
		defer c.receiverWg.Done()
		c.receiveLoop()
	}()

	// Wait for the server-side connection, then RST it.
	select {
	case serverConn := <-accepted:
		if tcp, ok := serverConn.(*net.TCPConn); ok {
			_ = tcp.SetLinger(0) // force RST on close
		}
		serverConn.Close()
	case <-time.After(2 * time.Second):
		t.Fatal("server never accepted a connection")
	}

	// The receiveLoop should detect the RST and emit StateDisconnected.
	select {
	case state := <-stateCh:
		if state != StateDisconnected {
			t.Errorf("first state emission = %v, want StateDisconnected", state)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for StateDisconnected after server reset")
	}

	c.Close()
}

func TestClose_UnblocksWaiters(t *testing.T) {
	cc, cleanup := newTestConnClient(t)
	defer cleanup()
	c := newTestClient(t, cc)
	cc.Close()

	c.reconnectingMu.Lock()
	c.reconnecting = true
	c.reconnectingMu.Unlock()

	done := make(chan bool, 1)
	go func() {
		done <- c.waitForReconnect(context.Background())
	}()

	// Give the goroutine time to block on the cond.
	time.Sleep(20 * time.Millisecond)

	c.Close()

	select {
	case result := <-done:
		if result {
			t.Error("waitForReconnect should return false after Close()")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForReconnect not unblocked after Close()")
	}
}