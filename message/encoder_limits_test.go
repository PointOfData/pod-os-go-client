package message

import (
	"errors"
	"testing"
)

func TestEncodeMessage_RejectsOversizePayload(t *testing.T) {
	originalMax := MaxMessageSizeBytes
	defer func() { MaxMessageSizeBytes = originalMax }()

	// Use a small max size to keep the test fast and memory-safe.
	MaxMessageSizeBytes = 64

	largePayload := make([]byte, 128)

	msg := &Message{
		Envelope: Envelope{
			To:      "mem@gateway.example.com",
			From:    "client@gateway.example.com",
			Intent:  IntentType.ActorEcho,
			MessageId: "msg-oversize-payload",
		},
		Payload: &PayloadFields{
			MimeType: "application/octet-stream",
			Data:     largePayload,
		},
	}

	encoded, err := EncodeMessage(msg, "conv-uuid")
	if err == nil {
		t.Fatalf("expected EncodeMessage to fail for oversize payload, got nil error and encoded message %+v", encoded)
	}

	if !IsEncodeError(err) {
		t.Fatalf("expected EncodeMessage error to be EncodeError, got %T: %v", err, err)
	}
	var encErr *EncodeError
	if !errors.As(err, &encErr) {
		t.Fatalf("expected EncodeError, got %T: %v", err, err)
	}
	if encErr.Code != ErrCodeEncodePayloadTooLarge || encErr.Field != "PayloadData" {
		t.Errorf("EncodeError = {Code:%d, Field:%q}, want Code ErrCodeEncodePayloadTooLarge and Field \"PayloadData\"", encErr.Code, encErr.Field)
	}
}

func TestEncodeMessage_RejectsOversizeTotalMessage(t *testing.T) {
	originalMax := MaxMessageSizeBytes
	defer func() { MaxMessageSizeBytes = originalMax }()

	// Force a very small max so even a minimal header will exceed it.
	MaxMessageSizeBytes = 16

	msg := &Message{
		Envelope: Envelope{
			To:        "mem@gateway.example.com",
			From:      "client@gateway.example.com",
			Intent:    IntentType.ActorEcho,
			MessageId: "msg-oversize-message",
		},
		Payload: &PayloadFields{
			MimeType: "text/plain",
			Data:     "",
		},
	}

	encoded, err := EncodeMessage(msg, "conv-uuid")
	if err == nil {
		t.Fatalf("expected EncodeMessage to fail for oversize total message length, got nil error and encoded message %+v", encoded)
	}

	if !IsEncodeError(err) {
		t.Fatalf("expected EncodeMessage error to be EncodeError, got %T: %v", err, err)
	}
	var encErr *EncodeError
	if !errors.As(err, &encErr) {
		t.Fatalf("expected EncodeError, got %T: %v", err, err)
	}
	if encErr.Code != ErrCodeEncodePayloadTooLarge || encErr.Field != "message" {
		t.Errorf("EncodeError = {Code:%d, Field:%q}, want Code ErrCodeEncodePayloadTooLarge and Field \"message\"", encErr.Code, encErr.Field)
	}
}

