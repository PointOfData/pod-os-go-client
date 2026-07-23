package podos

import (
	"testing"
	"time"

	"github.com/PointOfData/pod-os-go-client/config"
	"github.com/PointOfData/pod-os-go-client/message"
)

func TestConfigGetKeepaliveInterval(t *testing.T) {
	t.Parallel()

	defaultInterval := config.DefaultKeepaliveInterval()
	if defaultInterval != 30*time.Second {
		t.Fatalf("DefaultKeepaliveInterval() = %v, want 30s", defaultInterval)
	}

	var cfg config.Config
	if got := cfg.GetKeepaliveInterval(); got != 30*time.Second {
		t.Fatalf("unset GetKeepaliveInterval() = %v, want 30s", got)
	}

	cfg.KeepaliveInterval = 15 * time.Second
	if got := cfg.GetKeepaliveInterval(); got != 15*time.Second {
		t.Fatalf("custom GetKeepaliveInterval() = %v, want 15s", got)
	}

	cfg.KeepaliveInterval = -1
	if got := cfg.GetKeepaliveInterval(); got != 0 {
		t.Fatalf("disabled GetKeepaliveInterval() = %v, want 0", got)
	}
}

func TestBuildKeepaliveMessage(t *testing.T) {
	t.Parallel()

	c := &Client{
		gatewayActorName: "zeroth.pod-os.com",
		clientName:       "my-client",
	}
	msg := c.buildKeepaliveMessage()
	if msg.To != "$system@zeroth.pod-os.com" {
		t.Fatalf("To = %q", msg.To)
	}
	if msg.From != "my-client@zeroth.pod-os.com" {
		t.Fatalf("From = %q", msg.From)
	}
	if msg.Intent.MessageType != 18 {
		t.Fatalf("MessageType = %d, want 18", msg.Intent.MessageType)
	}
	if msg.Event != nil || msg.Payload != nil || msg.NeuralMemory != nil {
		t.Fatalf("keepalive must be envelope-only")
	}

	wire, err := c.encodeKeepalive()
	if err != nil {
		t.Fatalf("encodeKeepalive: %v", err)
	}
	if err := message.ValidateRawMessage(wire); err != nil {
		t.Fatalf("ValidateRawMessage: %v", err)
	}
}

func TestStartKeepaliveLoopDisabled(t *testing.T) {
	t.Parallel()

	c := &Client{
		cfg: config.Config{KeepaliveInterval: -1},
	}
	c.startKeepaliveLoop()
	c.keepaliveWg.Wait() // would deadlock if goroutine started
}

func TestConfigGetConnectionLivenessTimeout(t *testing.T) {
	t.Parallel()

	defaultTimeout := config.DefaultConnectionLivenessTimeout()
	if defaultTimeout != 90*time.Second {
		t.Fatalf("DefaultConnectionLivenessTimeout() = %v, want 90s", defaultTimeout)
	}

	var cfg config.Config
	if got := cfg.GetConnectionLivenessTimeout(); got != 90*time.Second {
		t.Fatalf("unset GetConnectionLivenessTimeout() = %v, want 90s", got)
	}

	cfg.ConnectionLivenessTimeout = 15 * time.Second
	if got := cfg.GetConnectionLivenessTimeout(); got != 15*time.Second {
		t.Fatalf("custom GetConnectionLivenessTimeout() = %v, want 15s", got)
	}

	cfg.ConnectionLivenessTimeout = -1
	if got := cfg.GetConnectionLivenessTimeout(); got != 0 {
		t.Fatalf("disabled GetConnectionLivenessTimeout() = %v, want 0", got)
	}
}
