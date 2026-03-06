//go:build integration

// Integration tests for message validation against a live Pod-OS gateway.
//
// Run with:
//
//	PODOS_VALIDATE=1 PODOS_TEST_HOST=<gateway-ip> go test -v -tags integration ./message/...
//
// Environment variables:
//
//	PODOS_TEST_HOST    IP address (or hostname) of the test gateway. Defaults to "zeroth.pod-os.com".
//	PODOS_TEST_PORT    Gateway port. Defaults to "62312".
//	PODOS_VALIDATE     Must be set to "1", "true", or "yes" to enable validation.
//
// The test actor address is built as: test@<PODOS_TEST_HOST>
// The gateway actor name is:          <PODOS_TEST_HOST>
package message

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// INTEGRATION TEST HELPERS
// =============================================================================

// integrationHost returns the gateway host for integration tests.
func integrationHost() string {
	if h := os.Getenv("PODOS_TEST_HOST"); h != "" {
		return h
	}
	return "zeroth.pod-os.com"
}

// integrationPort returns the gateway port for integration tests.
func integrationPort() string {
	if p := os.Getenv("PODOS_TEST_PORT"); p != "" {
		return p
	}
	return "62312"
}

// integrationAddr returns "host:port".
func integrationAddr() string {
	return net.JoinHostPort(integrationHost(), integrationPort())
}

// integrationTo returns the NeuralMemory actor address for the test gateway.
// Format: mem@<gatewayHost>
func integrationTo() string {
	return "mem@" + integrationHost()
}

// integrationFrom returns the test client address.
// Format: test@<gatewayHost>
func integrationFrom() string {
	return "test@" + integrationHost()
}

// dialGateway opens a TCP connection to the test gateway and returns a simple
// send/receive helper. The caller is responsible for closing the connection.
func dialGateway(t *testing.T) net.Conn {
	t.Helper()
	addr := integrationAddr()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Skipf("integration: cannot reach gateway at %s: %v", addr, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// sendRaw writes raw bytes to conn and reads a response.
func sendRaw(t *testing.T, conn net.Conn, raw []byte) []byte {
	t.Helper()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := conn.Write(raw); err != nil {
		t.Fatalf("sendRaw: write error: %v", err)
	}
	buf := make([]byte, 64*1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("sendRaw: read error: %v", err)
	}
	return buf[:n]
}

// encodeOrFail encodes a message and returns the wire bytes, failing the test on error.
func encodeOrFail(t *testing.T, msg *Message) []byte {
	t.Helper()
	socket, err := EncodeMessage(msg, "integ-conv")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}
	return socket.MessageBytes
}

// enableValidation ensures validation is on for the duration of the test.
func enableValidation(t *testing.T) {
	t.Helper()
	orig := validationEnabled
	validationEnabled = true
	t.Cleanup(func() { validationEnabled = orig })
}

// =============================================================================
// INTEGRATION: GatewayId
// =============================================================================

func TestIntegration_Validate_GatewayId(t *testing.T) {
	enableValidation(t)

	msg := &Message{
		Envelope: Envelope{
			To:         integrationTo(),
			From:       integrationFrom(),
			Intent:     IntentType.GatewayId,
			ClientName: "test",
		},
	}

	// Struct validation must pass.
	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() errors before encode:\n%s", errs.Error())
	}

	raw := encodeOrFail(t, msg)

	// Wire validation on the encoded bytes.
	if errs := ValidateRawMessage(raw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GatewayId wire bytes:\n%s", errs.Error())
	}

	// Send to the live gateway and validate the response wire format.
	conn := dialGateway(t)
	responseRaw := sendRaw(t, conn, raw)

	if errs := ValidateRawMessage(responseRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GatewayId response:\n%s", errs.Error())
	}

	// Decode and check response intent.
	decoded, decErr := DecodeMessage(responseRaw)
	if decErr != nil {
		t.Fatalf("DecodeMessage() failed on GatewayId response: %v", decErr)
	}
	if decoded.Response == nil || decoded.Response.Status == "" {
		t.Errorf("GatewayId response has no status: intent=%s", decoded.Intent.Name)
	}
}

// =============================================================================
// INTEGRATION: StoreEvent → validate wire message and response
// =============================================================================

func TestIntegration_Validate_StoreEvent(t *testing.T) {
	enableValidation(t)
	conn := dialGateway(t)

	// Authenticate first.
	idMsg := &Message{
		Envelope: Envelope{
			To: integrationTo(), From: integrationFrom(),
			Intent: IntentType.GatewayId, ClientName: "test",
		},
	}
	sendRaw(t, conn, encodeOrFail(t, idMsg))

	// Build StoreEvent.
	msg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.StoreEvent,
		},
		Event: &EventFields{
			Owner:             "$sys",
			Location:          "TERRA|47.6|-122.5",
			LocationSeparator: "|",
			Type:              "validate_test",
		},
	}

	// Struct validation.
	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() errors:\n%s", errs.Error())
	}

	raw := encodeOrFail(t, msg)

	// Wire validation on the outbound message.
	if errs := ValidateRawMessage(raw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on StoreEvent request:\n%s", errs.Error())
	}

	// Send and validate the response.
	responseRaw := sendRaw(t, conn, raw)
	if errs := ValidateRawMessage(responseRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on StoreEvent response:\n%s", errs.Error())
	}

	decoded, decErr := DecodeMessage(responseRaw)
	if decErr != nil {
		t.Fatalf("DecodeMessage() on StoreEvent response: %v", decErr)
	}
	if decoded.Response == nil {
		t.Fatalf("StoreEvent response has nil Response field")
	}
	if decoded.Response.Status != "OK" {
		t.Errorf("StoreEvent response Status = %q, want 'OK'; message: %s", decoded.Response.Status, decoded.Response.Message)
	}

	// Struct validation on the decoded response must produce no errors.
	if errs := decoded.Validate(); hasErrors(errs) {
		t.Errorf("Validate() on decoded StoreEventResponse:\n%s", errs.Error())
	}
}

// =============================================================================
// INTEGRATION: GetEvent (by ID returned from StoreEvent)
// =============================================================================

func TestIntegration_Validate_StoreAndGetEvent(t *testing.T) {
	enableValidation(t)
	conn := dialGateway(t)

	// Auth.
	idMsg := &Message{
		Envelope: Envelope{
			To: integrationTo(), From: integrationFrom(),
			Intent: IntentType.GatewayId, ClientName: "test",
		},
	}
	sendRaw(t, conn, encodeOrFail(t, idMsg))

	// Store.
	storeMsg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.StoreEvent,
		},
		Event: &EventFields{
			Owner:             "$sys",
			Location:          "TERRA|47.6|-122.5",
			LocationSeparator: "|",
			Type:              "validate_get_test",
		},
	}
	if errs := storeMsg.Validate(); hasErrors(errs) {
		t.Fatalf("StoreEvent Validate():\n%s", errs.Error())
	}
	storeResp := sendRaw(t, conn, encodeOrFail(t, storeMsg))
	storedMsg, err := DecodeMessage(storeResp)
	if err != nil {
		t.Fatalf("DecodeMessage() on store response: %v", err)
	}
	if storedMsg.Response == nil || storedMsg.Response.Status != "OK" {
		t.Fatalf("StoreEvent failed: status=%v", storedMsg.Response)
	}
	eventId := storedMsg.Event.Id
	if eventId == "" {
		t.Fatalf("StoreEvent response did not return an event ID")
	}

	// GetEvent.
	getMsg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.GetEvent,
		},
		Event: &EventFields{Id: eventId},
	}
	if errs := getMsg.Validate(); hasErrors(errs) {
		t.Fatalf("GetEvent Validate():\n%s", errs.Error())
	}
	getRaw := encodeOrFail(t, getMsg)
	if errs := ValidateRawMessage(getRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GetEvent request:\n%s", errs.Error())
	}

	getRespRaw := sendRaw(t, conn, getRaw)
	if errs := ValidateRawMessage(getRespRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GetEvent response:\n%s", errs.Error())
	}
	getResp, decErr := DecodeMessage(getRespRaw)
	if decErr != nil {
		t.Fatalf("DecodeMessage() on GetEvent response: %v", decErr)
	}
	if getResp.Response == nil || getResp.Response.Status != "OK" {
		t.Errorf("GetEvent status = %v", getResp.Response)
	}
	if getResp.Event == nil || getResp.Event.Id != eventId {
		t.Errorf("GetEvent returned wrong event ID: got %q, want %q", getResp.Event.Id, eventId)
	}
}

// =============================================================================
// INTEGRATION: LinkEvent → UnlinkEvent → response validation
// =============================================================================

func TestIntegration_Validate_LinkAndUnlinkEvent(t *testing.T) {
	enableValidation(t)
	conn := dialGateway(t)

	// Auth.
	idMsg := &Message{
		Envelope: Envelope{
			To: integrationTo(), From: integrationFrom(),
			Intent: IntentType.GatewayId, ClientName: "test",
		},
	}
	sendRaw(t, conn, encodeOrFail(t, idMsg))

	// Store two events to link.
	storeEvent := func(label string) string {
		m := &Message{
			Envelope: Envelope{
				To:     integrationTo(),
				From:   integrationFrom(),
				Intent: IntentType.StoreEvent,
			},
			Event: &EventFields{
				Owner:             "$sys",
				Location:          "TERRA|47.6|-122.5",
				LocationSeparator: "|",
				Type:              fmt.Sprintf("link_test_%s", label),
			},
		}
		resp, err := DecodeMessage(sendRaw(t, conn, encodeOrFail(t, m)))
		if err != nil || resp.Response == nil || resp.Response.Status != "OK" {
			t.Fatalf("store %s failed: %v", label, err)
		}
		if resp.Event == nil || resp.Event.Id == "" {
			t.Fatalf("store %s: no event ID in response", label)
		}
		return resp.Event.Id
	}

	idA := storeEvent("A")
	idB := storeEvent("B")

	// LinkEvent.
	ts := fmt.Sprintf("+%d.000000", time.Now().Unix())
	linkMsg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.LinkEvent,
		},
		Event: &EventFields{Owner: "$sys"},
		NeuralMemory: &NeuralMemoryFields{
			Link: &LinkFields{
				EventA:            idA,
				EventB:            idB,
				Category:          "validate_test_link",
				StrengthA:         1.0,
				StrengthB:         1.0,
				Timestamp:         ts,
				OwnerID:           "$sys",
				Location:          "TERRA|47.6|-122.5",
				LocationSeparator: "|",
			},
		},
	}

	if errs := linkMsg.Validate(); hasErrors(errs) {
		t.Fatalf("LinkEvent Validate():\n%s", errs.Error())
	}
	linkRaw := encodeOrFail(t, linkMsg)
	if errs := ValidateRawMessage(linkRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on LinkEvent request:\n%s", errs.Error())
	}

	linkRespRaw := sendRaw(t, conn, linkRaw)
	if errs := ValidateRawMessage(linkRespRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on LinkEvent response:\n%s", errs.Error())
	}
	linkResp, err := DecodeMessage(linkRespRaw)
	if err != nil {
		t.Fatalf("DecodeMessage() on LinkEvent response: %v", err)
	}
	if linkResp.Response == nil || linkResp.Response.Status != "OK" {
		t.Fatalf("LinkEvent failed: %v", linkResp.Response)
	}
	linkId := linkResp.Response.LinkId
	if linkId == "" {
		t.Fatalf("LinkEvent response did not return a link ID")
	}

	// UnlinkEvent.
	unlinkMsg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.UnlinkEvent,
		},
		NeuralMemory: &NeuralMemoryFields{
			Link: &LinkFields{Id: linkId},
		},
	}

	if errs := unlinkMsg.Validate(); hasErrors(errs) {
		t.Fatalf("UnlinkEvent Validate():\n%s", errs.Error())
	}
	unlinkRaw := encodeOrFail(t, unlinkMsg)
	if errs := ValidateRawMessage(unlinkRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on UnlinkEvent request:\n%s", errs.Error())
	}

	unlinkRespRaw := sendRaw(t, conn, unlinkRaw)
	if errs := ValidateRawMessage(unlinkRespRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on UnlinkEvent response:\n%s", errs.Error())
	}
	unlinkResp, err := DecodeMessage(unlinkRespRaw)
	if err != nil {
		t.Fatalf("DecodeMessage() on UnlinkEvent response: %v", err)
	}
	if unlinkResp.Response == nil || unlinkResp.Response.Status != "OK" {
		t.Errorf("UnlinkEvent failed: %v", unlinkResp.Response)
	}
}

// =============================================================================
// INTEGRATION: StoreBatchTags → StoreBatchTagsResponse validation
// =============================================================================

func TestIntegration_Validate_StoreBatchTags(t *testing.T) {
	enableValidation(t)
	conn := dialGateway(t)

	// Auth.
	idMsg := &Message{
		Envelope: Envelope{
			To: integrationTo(), From: integrationFrom(),
			Intent: IntentType.GatewayId, ClientName: "test",
		},
	}
	sendRaw(t, conn, encodeOrFail(t, idMsg))

	// Store a target event.
	storeM := &Message{
		Envelope: Envelope{To: integrationTo(), From: integrationFrom(), Intent: IntentType.StoreEvent},
		Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|", Type: "tag_target"},
	}
	resp, _ := DecodeMessage(sendRaw(t, conn, encodeOrFail(t, storeM)))
	eventId := resp.Event.Id
	if eventId == "" {
		t.Fatalf("could not get event ID from StoreEvent response")
	}

	// StoreBatchTags.
	tagMsg := &Message{
		Envelope: Envelope{To: integrationTo(), From: integrationFrom(), Intent: IntentType.StoreBatchTags},
		Event:    &EventFields{Id: eventId, Owner: "$sys"},
		NeuralMemory: &NeuralMemoryFields{
			Tags: TagList{
				{Key: "language", Value: "go", Frequency: 1},
				{Key: "framework", Value: "podos", Frequency: 1},
			},
		},
	}

	if errs := tagMsg.Validate(); hasErrors(errs) {
		t.Fatalf("StoreBatchTags Validate():\n%s", errs.Error())
	}
	tagRaw := encodeOrFail(t, tagMsg)
	if errs := ValidateRawMessage(tagRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on StoreBatchTags request:\n%s", errs.Error())
	}

	tagRespRaw := sendRaw(t, conn, tagRaw)
	if errs := ValidateRawMessage(tagRespRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on StoreBatchTagsResponse:\n%s", errs.Error())
	}
	tagResp, err := DecodeMessage(tagRespRaw)
	if err != nil {
		t.Fatalf("DecodeMessage() on StoreBatchTags response: %v", err)
	}
	if tagResp.Response == nil || tagResp.Response.Status != "OK" {
		t.Errorf("StoreBatchTags failed: %v", tagResp.Response)
	}
}

// =============================================================================
// INTEGRATION: GetEventsForTags — validate request and buffered response
// =============================================================================

func TestIntegration_Validate_GetEventsForTags(t *testing.T) {
	enableValidation(t)
	conn := dialGateway(t)

	// Auth.
	idMsg := &Message{
		Envelope: Envelope{
			To: integrationTo(), From: integrationFrom(),
			Intent: IntentType.GatewayId, ClientName: "test",
		},
	}
	sendRaw(t, conn, encodeOrFail(t, idMsg))

	// GetEventsForTags.
	searchMsg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.GetEventsForTags,
		},
		NeuralMemory: &NeuralMemoryFields{
			GetEventsForTags: &GetEventsForTagsOptions{
				EventPattern:  "type=validate_test",
				BufferResults: true,
				CountOnly:     true,
			},
		},
	}

	if errs := searchMsg.Validate(); hasErrors(errs) {
		t.Fatalf("GetEventsForTags Validate():\n%s", errs.Error())
	}
	searchRaw := encodeOrFail(t, searchMsg)
	if errs := ValidateRawMessage(searchRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GetEventsForTags request:\n%s", errs.Error())
	}

	respRaw := sendRaw(t, conn, searchRaw)
	if errs := ValidateRawMessage(respRaw); hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on GetEventsForTags response:\n%s", errs.Error())
	}
	decoded, err := DecodeMessage(respRaw)
	if err != nil {
		t.Fatalf("DecodeMessage() on GetEventsForTags response: %v", err)
	}
	if decoded.Response == nil {
		t.Fatalf("GetEventsForTags response has nil Response")
	}
}

// =============================================================================
// INTEGRATION: LLMJson output round-trip (validates JSON is parseable by tools)
// =============================================================================

func TestIntegration_Validate_LLMJsonOutput(t *testing.T) {
	enableValidation(t)

	// Deliberately invalid LinkEvent — should collect multiple errors.
	msg := &Message{
		Envelope: Envelope{
			To:     integrationTo(),
			From:   integrationFrom(),
			Intent: IntentType.LinkEvent,
		},
		NeuralMemory: &NeuralMemoryFields{
			Link: &LinkFields{
				// Missing Category, StrengthA, StrengthB, Timestamp, OwnerID, Location, LocationSeparator
				EventA: "a", EventB: "b",
			},
		},
	}

	errs := msg.Validate()
	if len(errs) == 0 {
		t.Fatalf("expected validation errors for incomplete LinkEvent")
	}

	// Engineer format: each error must have [ERROR].
	engineerOut := errs.Error()
	if !strings.Contains(engineerOut, "[ERROR]") {
		t.Errorf("engineer output must contain [ERROR]: %s", engineerOut)
	}

	// LLM JSON: must be parseable and each error must have all required fields.
	llmOut := errs.LLMJson()
	var parsed []llmValidationError
	if jsonErr := json.Unmarshal([]byte(llmOut), &parsed); jsonErr != nil {
		t.Fatalf("LLMJson() produced invalid JSON: %v\n%s", jsonErr, llmOut)
	}
	if len(parsed) != len(errs) {
		t.Errorf("LLMJson item count = %d, want %d", len(parsed), len(errs))
	}
	for i, e := range errs {
		if e.Intent == "" {
			t.Errorf("errs[%d] missing Intent: %+v", i, e)
		}
		if e.Rule == "" {
			t.Errorf("errs[%d] missing Rule: %+v", i, e)
		}
		if e.Message == "" {
			t.Errorf("errs[%d] missing Message: %+v", i, e)
		}
		if e.Fix == "" {
			t.Errorf("errs[%d] missing Fix: %+v", i, e)
		}
	}
}
