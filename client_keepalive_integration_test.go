//go:build integration

package podos

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/PointOfData/pod-os-go-client/config"
	"github.com/PointOfData/pod-os-go-client/message"
	"github.com/google/uuid"
)

func integrationHost() string {
	if h := os.Getenv("PODOS_TEST_HOST"); h != "" {
		return h
	}
	return "zeroth.pod-os.com"
}

func integrationPort() string {
	if p := os.Getenv("PODOS_TEST_PORT"); p != "" {
		return p
	}
	return "62312"
}

// TestKeepaliveLiveVerify confirms the gateway accepts an envelope-only Keepalive
// (message_type 18) with SDK addressing after GatewayId handshake.
func TestKeepaliveLiveVerify(t *testing.T) {
	addr := net.JoinHostPort(integrationHost(), integrationPort())
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Skipf("integration: cannot reach gateway at %s: %v", addr, err)
	}

	gateway := integrationHost()
	clientName := "keepalive-verify-" + uuid.New().String()[:8]

	// GatewayId handshake (required before other messages).
	idMsg := &message.Message{
		Envelope: message.Envelope{
			To:         "$system@" + gateway,
			From:       clientName + "@" + gateway,
			Intent:     message.IntentType.GatewayId,
			ClientName: clientName,
			MessageId:  uuid.New().String(),
		},
		Event: &message.EventFields{
			Owner:             "$sys",
			Timestamp:         message.GetTimestamp(),
			LocationSeparator: "|",
		},
	}
	idWire, err := message.EncodeMessage(idMsg, uuid.New().String())
	if err != nil {
		t.Fatalf("encode GatewayId: %v", err)
	}
	if _, err := conn.Write(idWire.MessageBytes); err != nil {
		t.Fatalf("send GatewayId: %v", err)
	}
	idResp := readFrame(t, conn)
	idDecoded, err := message.DecodeMessage(idResp)
	if err != nil {
		t.Fatalf("decode GatewayId response: %v", err)
	}
	if idDecoded.ProcessingStatus() == "ERROR" {
		t.Fatalf("GatewayId rejected: %s", idDecoded.ProcessingMessage())
	}

	// Keepalive: envelope-only, SDK From convention.
	keepalive := &message.Message{
		Envelope: message.Envelope{
			To:         "$system@" + gateway,
			From:       clientName + "@" + gateway,
			Intent:     message.IntentType.Keepalive,
			ClientName: clientName,
			MessageId:  uuid.New().String(),
		},
	}
	keepaliveWire, err := message.EncodeMessage(keepalive, uuid.New().String())
	if err != nil {
		t.Fatalf("encode Keepalive: %v", err)
	}
	if err := message.ValidateRawMessage(keepaliveWire.MessageBytes); err != nil {
		t.Fatalf("validate Keepalive wire: %v", err)
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write(keepaliveWire.MessageBytes); err != nil {
		t.Fatalf("send Keepalive: %v", err)
	}

	// Gateway may or may not reply; a read timeout is acceptable for fire-and-forget.
	buf := make([]byte, 64*1024)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, readErr := conn.Read(buf)
	if readErr == nil && n > 0 {
		resp, err := message.DecodeMessage(buf[:n])
		if err != nil {
			t.Fatalf("decode Keepalive response: %v", err)
		}
		if resp.ProcessingStatus() == "ERROR" {
			t.Fatalf("Keepalive rejected: %s", resp.ProcessingMessage())
		}
		t.Logf("Keepalive response status=%s", resp.ProcessingStatus())
	} else {
		t.Log("Keepalive accepted (no response within 2s — fire-and-forget OK)")
	}
}

// TestClientKeepaliveLoopLiveVerify uses podos.Client with a short keepalive interval.
func TestClientKeepaliveLoopLiveVerify(t *testing.T) {
	addr := net.JoinHostPort(integrationHost(), integrationPort())
	if _, err := net.DialTimeout("tcp", addr, 5*time.Second); err != nil {
		t.Skipf("integration: cannot reach gateway at %s: %v", addr, err)
	}

	gateway := integrationHost()
	clientName := "keepalive-loop-" + uuid.New().String()[:8]
	host, port, _ := net.SplitHostPort(addr)

	cfg := config.Config{
		Network:              "tcp",
		Host:                 host,
		Port:                 port,
		GatewayActorName:     gateway,
		ClientName:           clientName,
		KeepaliveInterval:    2 * time.Second,
		EnableConcurrentMode: true,
		ReceiveTimeout:       10 * time.Second,
	}
	reconnectDisabled := false
	cfg.ReconnectConfig.Enabled = &reconnectDisabled

	client, err := NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	time.Sleep(2500 * time.Millisecond)
	if !client.IsConnected() {
		t.Fatal("client disconnected after keepalive loop tick")
	}
}

func readFrame(t *testing.T, conn net.Conn) []byte {
	t.Helper()
	buf := make([]byte, 64*1024)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return buf[:n]
}
