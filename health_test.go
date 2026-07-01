package podos

import (
	"testing"

	"github.com/PointOfData/pod-os-go-client/message"
)

func TestBuildStatusHealthReply(t *testing.T) {
	c := &Client{
		gatewayActorName: "zeroth.pod-os.com",
		clientName:       "socket-actor",
	}
	inbound := &message.Message{
		Envelope: message.Envelope{
			From:      "probe-client@zeroth.pod-os.com",
			MessageId: "probe-msg-1",
			Intent:    message.IntentType.StatusRequest,
		},
	}

	reply := BuildStatusHealthReply(c, inbound)
	if reply.Intent.Name != message.IntentType.Status.Name {
		t.Fatalf("intent = %q, want Status", reply.Intent.Name)
	}
	if reply.MessageId != "probe-msg-1" {
		t.Fatalf("MessageId = %q, want probe-msg-1", reply.MessageId)
	}
	if reply.To != "probe-client@zeroth.pod-os.com" {
		t.Fatalf("To = %q", reply.To)
	}
	if reply.From != "socket-actor@zeroth.pod-os.com" {
		t.Fatalf("From = %q", reply.From)
	}
	if reply.Response == nil || reply.Response.Status != "OK" {
		t.Fatalf("Response.Status = %+v", reply.Response)
	}

	socketMsg, err := message.EncodeMessage(reply, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	decoded, err := message.DecodeMessage(socketMsg.MessageBytes)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if decoded.Intent.MessageType != message.IntentType.Status.MessageType {
		t.Fatalf("decoded messageType = %d, want %d", decoded.Intent.MessageType, message.IntentType.Status.MessageType)
	}
	if decoded.MessageId != "probe-msg-1" {
		t.Fatalf("decoded MessageId = %q", decoded.MessageId)
	}
	if decoded.Response == nil || decoded.Response.Status != "OK" {
		t.Fatalf("decoded Response = %+v", decoded.Response)
	}
}

func TestBuildStatusHealthProbeRequest(t *testing.T) {
	msg := &message.Message{
		Envelope: message.Envelope{
			To:         "socket-actor@gateway.pod-os.com",
			From:       "dashboard@zeroth.pod-os.com",
			Intent:     message.IntentType.StatusRequest,
			ClientName: "dashboard",
			MessageId:  "health-1",
		},
	}
	socketMsg, err := message.EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage: %v", err)
	}
	decoded, err := message.DecodeMessage(socketMsg.MessageBytes)
	if err != nil {
		t.Fatalf("DecodeMessage: %v", err)
	}
	if decoded.Intent.Name != message.IntentType.StatusRequest.Name {
		t.Fatalf("decoded intent = %q", decoded.Intent.Name)
	}
	if decoded.MessageId != "health-1" {
		t.Fatalf("decoded MessageId = %q", decoded.MessageId)
	}
}
