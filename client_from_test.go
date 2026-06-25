package podos

import (
	"testing"

	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
)

func TestClientFromAddress(t *testing.T) {
	c := &Client{
		clientName:       "my-client",
		gatewayActorName: "zeroth.pod-os.com",
		logger:           log.NoOpLogger{},
	}
	if got := c.FromAddress(); got != "my-client@zeroth.pod-os.com" {
		t.Fatalf("FromAddress() = %q, want my-client@zeroth.pod-os.com", got)
	}
}

func TestNormalizeMessageFromUsesConnectionGateway(t *testing.T) {
	c := &Client{
		clientName:       "my-client",
		gatewayActorName: "zeroth.pod-os.com",
		logger:           log.NoOpLogger{},
	}

	msg := &message.Message{
		Envelope: message.Envelope{
			To:         "kb@skills.pod-os.com",
			From:       "other@skills.pod-os.com",
			ClientName: "other",
			Intent:     message.IntentType.GetEvent,
		},
	}

	c.normalizeMessageFrom(msg)

	if msg.ClientName != "my-client" {
		t.Fatalf("ClientName = %q, want my-client", msg.ClientName)
	}
	if msg.From != "my-client@zeroth.pod-os.com" {
		t.Fatalf("From = %q, want my-client@zeroth.pod-os.com", msg.From)
	}
}

func TestNormalizeMessageFromEmptyFrom(t *testing.T) {
	c := &Client{
		clientName:       "my-client",
		gatewayActorName: "skills.pod-os.com",
		logger:           log.NoOpLogger{},
	}

	msg := &message.Message{
		Envelope: message.Envelope{
			To:     "kb@skills.pod-os.com",
			Intent: message.IntentType.GetEvent,
		},
	}

	c.normalizeMessageFrom(msg)

	if msg.From != "my-client@skills.pod-os.com" {
		t.Fatalf("From = %q, want my-client@skills.pod-os.com", msg.From)
	}
}
