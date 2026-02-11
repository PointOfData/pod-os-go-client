package message

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// buildMinimalMessage creates a minimal valid AIP message for testing
// Format: 9-char fields for: total_len, to_len, from_len, header_len, msg_type, data_type, payload_len
// Followed by: to, from, header, payload
func buildMinimalMessage(to, from string, header string, messageType int, dataType int, payload string) []byte {
	toLen := len(to)
	fromLen := len(from)
	headerLen := len(header)
	payloadLen := len(payload)

	// Calculate total length: 7 * 9 (lengths) + to + from + header + payload
	totalLen := 7*9 + toLen + fromLen + headerLen + payloadLen

	// Build the message with 9-char padded fields
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("%9d", totalLen))
	msg.WriteString(fmt.Sprintf("%9d", toLen))
	msg.WriteString(fmt.Sprintf("%9d", fromLen))
	msg.WriteString(fmt.Sprintf("%9d", headerLen))
	msg.WriteString(fmt.Sprintf("%9d", messageType))
	msg.WriteString(fmt.Sprintf("%9d", dataType))
	msg.WriteString(fmt.Sprintf("%9d", payloadLen))
	msg.WriteString(to)
	msg.WriteString(from)
	msg.WriteString(header)
	msg.WriteString(payload)

	return []byte(msg.String())
}

func TestDecodeMessage_IntentDetermination(t *testing.T) {
	tests := []struct {
		name             string
		messageType      int
		header           string
		expectIntentName string
	}{
		// MEM_REPLY (1001) with _type header
		{
			name:             "MEM_REPLY with _type=store",
			messageType:      1001,
			header:           "_type=store\t_status=OK",
			expectIntentName: "StoreEventResponse",
		},
		{
			name:             "MEM_REPLY with _type=get",
			messageType:      1001,
			header:           "_type=get\t_status=OK",
			expectIntentName: "GetEventResponse",
		},
		{
			name:             "MEM_REPLY with _type=events_for_tag",
			messageType:      1001,
			header:           "_type=events_for_tag\t_status=OK",
			expectIntentName: "GetEventsForTagsResponse",
		},
		// MEM_REPLY with _command header
		{
			name:             "MEM_REPLY with _command=store_batch",
			messageType:      1001,
			header:           "_command=store_batch\t_status=OK",
			expectIntentName: "StoreBatchEventsResponse",
		},
		// MEM_REPLY with _db_cmd header
		{
			name:             "MEM_REPLY with _db_cmd=link",
			messageType:      1001,
			header:           "_db_cmd=link\t_status=OK",
			expectIntentName: "LinkEventResponse",
		},
		// MEM_REQ (1000) with _type header
		{
			name:             "MEM_REQ with _type=store",
			messageType:      1000,
			header:           "_type=store\t_status=OK",
			expectIntentName: "StoreEvent",
		},
		{
			name:             "MEM_REQ with _db_cmd=get",
			messageType:      1000,
			header:           "_db_cmd=get",
			expectIntentName: "GetEvent",
		},
		// Non-Neural Memory message types
		{
			name:             "ActorEcho message",
			messageType:      2,
			header:           "_status=OK",
			expectIntentName: "ActorEcho",
		},
		{
			name:             "ActorStart message",
			messageType:      1,
			header:           "_status=OK",
			expectIntentName: "ActorStart",
		},
		{
			name:             "GatewayStatus message",
			messageType:      3,
			header:           "_status=OK",
			expectIntentName: "GatewayStatus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildMinimalMessage(
				"actor@gateway.example.com",
				"client@gateway.example.com",
				tt.header,
				tt.messageType,
				0,
				"",
			)

			decoded, err := DecodeMessage(msg)
			if err != nil {
				t.Fatalf("DecodeMessage() error = %v", err)
			}

			if decoded.Intent.Name != tt.expectIntentName {
				t.Errorf("DecodeMessage() Intent.Name = %q, want %q",
					decoded.Intent.Name, tt.expectIntentName)
			}
		})
	}
}

func TestDecodeMessage_ResponseIntentHasCorrectMessageType(t *testing.T) {
	// Test that when we decode a MEM_REPLY message, the intent has MessageType 1001
	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		"_type=store\t_status=OK\t_msg=Success",
		1001, // MEM_REPLY
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.MessageType != 1001 {
		t.Errorf("Decoded intent MessageType = %d, want 1001", decoded.Intent.MessageType)
	}
	if decoded.Intent.RoutingMessageType != "MEM_REPLY" {
		t.Errorf("Decoded intent RoutingMessageType = %q, want MEM_REPLY", decoded.Intent.RoutingMessageType)
	}
}

func TestDecodeMessage_HeaderPriorityForCommand(t *testing.T) {
	// Test that _type takes priority over _command and _db_cmd
	msg := buildMinimalMessage(
		"actor@gateway.example.com",
		"client@gateway.example.com",
		"_type=get\t_command=store\t_db_cmd=link\t_status=OK",
		1001,
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	// _type should take priority, so intent should be GetEventResponse
	if decoded.Intent.Name != "GetEventResponse" {
		t.Errorf("DecodeMessage() Intent.Name = %q, want GetEventResponse (from _type)", decoded.Intent.Name)
	}
}

// =============================================================================
// LIMIT & BAD-ACTOR TESTS
// =============================================================================

// buildRawMessageWithCustomLengths builds a raw message where the declared
// length fields can be intentionally inconsistent with the actual body, to
// simulate bad-actor or malformed inputs.
func buildRawMessageWithCustomLengths(
	totalLen, toLen, fromLen, headerLen, messageType, dataType, payloadLen int,
	to, from, header, payload string,
) []byte {
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("%09d", totalLen))
	msg.WriteString(fmt.Sprintf("%09d", toLen))
	msg.WriteString(fmt.Sprintf("%09d", fromLen))
	msg.WriteString(fmt.Sprintf("%09d", headerLen))
	msg.WriteString(fmt.Sprintf("%09d", messageType))
	msg.WriteString(fmt.Sprintf("%09d", dataType))
	msg.WriteString(fmt.Sprintf("%09d", payloadLen))
	msg.WriteString(to)
	msg.WriteString(from)
	msg.WriteString(header)
	msg.WriteString(payload)
	return []byte(msg.String())
}

func TestDecodeMessage_RejectsOversizeMessageByLength(t *testing.T) {
	originalMax := MaxMessageSizeBytes
	defer func() { MaxMessageSizeBytes = originalMax }()

	// Use a small max size to keep the test fast and memory-safe.
	MaxMessageSizeBytes = 64

	// Build a valid minimal message that exceeds the artificial 64-byte limit.
	payload := strings.Repeat("x", 80)
	msg := buildMinimalMessage(
		"actor@gateway.example.com",
		"client@gateway.example.com",
		"_status=OK",
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err == nil {
		t.Fatalf("expected DecodeMessage to fail for oversize message, got nil error and message %+v", decoded)
	}

	if !IsDecodeError(err) {
		t.Fatalf("expected DecodeMessage error to be DecodeError, got %T: %v", err, err)
	}
	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected DecodeError, got %T: %v", err, err)
	}
	if decErr.Code != ErrCodeDecodePayloadTooLarge {
		t.Errorf("DecodeError.Code = %d, want ErrCodeDecodePayloadTooLarge (%d)", decErr.Code, ErrCodeDecodePayloadTooLarge)
	}
}

func TestDecodeMessage_HeaderLengthBeyondAvailableBytes(t *testing.T) {
	// Use the default MaxMessageSizeBytes; message is tiny.

	to := "actor@gateway.example.com"
	from := "client@gateway.example.com"
	header := "_status=OK"
	payload := ""

	toLen := len(to)
	fromLen := len(from)
	declaredHeaderLen := 100 // Deliberately larger than actual header length
	payloadLen := len(payload)

	// totalLen is declared based on the (too-large) header length field.
	totalLen := 7*9 + toLen + fromLen + declaredHeaderLen + payloadLen

	msg := buildRawMessageWithCustomLengths(
		totalLen,
		toLen,
		fromLen,
		declaredHeaderLen,
		1001,
		0,
		payloadLen,
		to,
		from,
		header,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err == nil {
		t.Fatalf("expected DecodeMessage to fail for truncated header, got nil error and message %+v", decoded)
	}

	if !IsDecodeError(err) {
		t.Fatalf("expected DecodeMessage error to be DecodeError, got %T: %v", err, err)
	}
	var decErr *DecodeError
	if !errors.As(err, &decErr) {
		t.Fatalf("expected DecodeError, got %T: %v", err, err)
	}
	if decErr.Code != ErrCodeDecodeMessageTooShort || decErr.Field != "header" {
		t.Errorf("DecodeError = {Code:%d, Field:%q}, want Code ErrCodeDecodeMessageTooShort and Field \"header\"", decErr.Code, decErr.Field)
	}
}
