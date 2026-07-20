package podos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/PointOfData/pod-os-go-client/connection"
	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
)

func TestBuildDisconnectMessage(t *testing.T) {
	t.Parallel()

	c := &Client{
		gatewayActorName: "zeroth.pod-os.com",
		clientName:       "my-client",
	}
	msg := c.buildDisconnectMessage()
	if msg.To != "$system@zeroth.pod-os.com" {
		t.Fatalf("To = %q", msg.To)
	}
	if msg.From != "my-client@zeroth.pod-os.com" {
		t.Fatalf("From = %q", msg.From)
	}
	if msg.Intent.MessageType != 6 {
		t.Fatalf("MessageType = %d, want 6", msg.Intent.MessageType)
	}
	if msg.Event != nil || msg.Payload != nil || msg.NeuralMemory != nil {
		t.Fatalf("disconnect must be envelope-only")
	}

	wire, err := c.encodeDisconnect()
	if err != nil {
		t.Fatalf("encodeDisconnect: %v", err)
	}
	if !strings.Contains(string(wire), "000000006") {
		t.Fatalf("wire missing message type 6: %q", wire)
	}
	if err := message.ValidateRawMessage(wire); err != nil {
		t.Fatalf("ValidateRawMessage: %v", err)
	}
}

func TestClose_SendsDisconnectBeforeTCP(t *testing.T) {
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
	retry := connection.NewRetry(connection.Retry{Retries: 1, Backoff: time.Millisecond})
	cc := connection.NewClient(
		context.Background(),
		connection.ClientConfig{Logger: log.NoOpLogger{}},
		"tcp", "127.0.0.1", fmt.Sprintf("%d", addr.Port),
		"test-actor", retry,
	)
	if cc == nil {
		t.Fatal("connection.NewClient returned nil")
	}

	c := newTestClient(t, cc)
	c.gatewayActorName = "zeroth.pod-os.com"
	c.clientName = "close-test-client"

	select {
	case serverConn := <-accepted:
		defer serverConn.Close()
		done := make(chan struct{})
		var got []byte
		go func() {
			defer close(done)
			buf := make([]byte, 4096)
			n, readErr := serverConn.Read(buf)
			if readErr != nil && readErr != io.EOF {
				t.Errorf("server read: %v", readErr)
				return
			}
			got = append(got, buf[:n]...)
		}()

		if err := c.Close(); err != nil {
			t.Fatalf("Close(): %v", err)
		}

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for disconnect frame on server")
		}

		if !bytes.Contains(got, []byte("000000006")) {
			t.Fatalf("server did not receive GatewayDisconnect frame; got %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for accepted connection")
	}
}
