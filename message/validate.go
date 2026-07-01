package message

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// validationEnabled is set once at package init from PODOS_VALIDATE env var.
// Accepted values: "1", "true", "yes" (case-insensitive). Anything else disables.
// Both Validate() and ValidateRawMessage() return nil immediately when false,
// making the hot path a single bool check with zero allocations.
var validationEnabled bool

func init() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PODOS_VALIDATE")))
	validationEnabled = v == "1" || v == "true" || v == "yes"
}

// =============================================================================
// ERROR TYPES — dual-audience (engineer + LLM)
// =============================================================================

// ValidationError represents a single validation violation.
// It is designed to be consumed by two audiences:
//   - Engineers: via the Error()/String() output on ValidationErrors (terminal-friendly)
//   - LLMs: via the LLMJson() output on ValidationErrors (structured JSON for prompt injection)
type ValidationError struct {
	Severity    string   // "error" or "warn"
	Intent      string   // Intent name, e.g. "LinkEvent"
	Field       string   // Go struct dot-path: "NeuralMemory.Link.Category"
	WireField   string   // Wire protocol key: "category"
	Rule        string   // "required", "one_of_required", "format", "nil_struct",
	             //        "header_missing", "header_value", "payload_type",
	             //        "payload_format", "uncovered"
	Message     string   // Human-readable description of what is wrong
	Fix         string   // Concrete remediation step in plain English
	ExampleCode string   // Minimal Go snippet showing a correct value
	References  []string // Source locations: "message/types.go:LinkFields.Category"
}

// ValidationErrors is a slice of ValidationError.
type ValidationErrors []ValidationError

// Error returns a terminal-friendly, engineer-readable multiline string.
// Empty ValidationErrors produces "".
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, ve := range e {
		prefix := "[ERROR]"
		if strings.EqualFold(ve.Severity, "warn") {
			prefix = "[WARN]"
		}
		// Header line
		wireInfo := ""
		if ve.WireField != "" {
			wireInfo = fmt.Sprintf(" (%s)", ve.WireField)
		}
		fieldInfo := ve.Field
		if fieldInfo == "" {
			fieldInfo = ve.Intent
		}
		fmt.Fprintf(&sb, "%s %s / %s%s: %s\n", prefix, ve.Intent, fieldInfo, wireInfo, ve.Rule)
		if ve.Message != "" {
			fmt.Fprintf(&sb, "  What: %s\n", ve.Message)
		}
		if ve.Fix != "" {
			fmt.Fprintf(&sb, "  Fix:  %s\n", ve.Fix)
		}
		if ve.ExampleCode != "" {
			fmt.Fprintf(&sb, "  Code: %s\n", ve.ExampleCode)
		}
	}
	return sb.String()
}

// llmValidationError is the JSON representation of a ValidationError for LLM consumption.
type llmValidationError struct {
	Severity    string   `json:"severity"`
	Intent      string   `json:"intent"`
	StructPath  string   `json:"struct_path"`
	WireField   string   `json:"wire_field"`
	Rule        string   `json:"rule"`
	Description string   `json:"description"`
	Fix         string   `json:"fix"`
	ExampleCode string   `json:"example_code"`
	References  []string `json:"references"`
}

// LLMJson returns a JSON array of validation errors suitable for injection into an LLM prompt
// or tool-call response. Empty ValidationErrors produces "[]".
func (e ValidationErrors) LLMJson() string {
	items := make([]llmValidationError, len(e))
	for i, ve := range e {
		refs := ve.References
		if refs == nil {
			refs = []string{}
		}
		items[i] = llmValidationError{
			Severity:    ve.Severity,
			Intent:      ve.Intent,
			StructPath:  ve.Field,
			WireField:   ve.WireField,
			Rule:        ve.Rule,
			Description: ve.Message,
			Fix:         ve.Fix,
			ExampleCode: ve.ExampleCode,
			References:  refs,
		}
	}
	b, _ := json.MarshalIndent(items, "", "  ")
	return string(b)
}

// =============================================================================
// HELPERS — shared field check utilities
// =============================================================================

func errorf(severity, intent, field, wireField, rule, msg, fix, code string, refs ...string) ValidationError {
	return ValidationError{
		Severity:    severity,
		Intent:      intent,
		Field:       field,
		WireField:   wireField,
		Rule:        rule,
		Message:     msg,
		Fix:         fix,
		ExampleCode: code,
		References:  refs,
	}
}

func requiredField(intent, field, wireField, fix, code string, refs ...string) ValidationError {
	return errorf("error", intent, field, wireField, "required",
		fmt.Sprintf("%s (%s) is required for %s and is missing.", field, wireField, intent),
		fix, code, refs...)
}

func nilStruct(intent, field, fix, code string, refs ...string) ValidationError {
	return errorf("error", intent, field, "", "nil_struct",
		fmt.Sprintf("%s must be initialized for intent %s.", field, intent),
		fix, code, refs...)
}

func oneOfRequired(intent, fieldA, wireA, fieldB, wireB, fix, code string, refs ...string) ValidationError {
	return errorf("error", intent,
		fmt.Sprintf("%s / %s", fieldA, fieldB),
		fmt.Sprintf("%s / %s", wireA, wireB),
		"one_of_required",
		fmt.Sprintf("One of %s (%s) or %s (%s) is required for %s.", fieldA, wireA, fieldB, wireB, intent),
		fix, code, refs...)
}

func warnUncovered(intent, msg string) ValidationError {
	return errorf("warn", intent, "", "", "uncovered", msg,
		"This case is currently unsupported. Monitor release notes for support updates.", "", "")
}

// isNameAtGateway checks that s contains exactly one '@' and neither part is empty.
func isNameAtGateway(s string) bool {
	idx := strings.IndexByte(s, '@')
	return idx > 0 && idx < len(s)-1
}

// =============================================================================
// VALIDATE — struct-level validation
// =============================================================================

// Validate validates a *Message against the rules for its Intent.
// Returns nil immediately if validation is disabled (PODOS_VALIDATE not set).
// Otherwise returns all violations collected across envelope + intent + payload.
func (m *Message) Validate() ValidationErrors {
	if !validationEnabled {
		return nil
	}
	var errs ValidationErrors
	errs = append(errs, validateEnvelope(m)...)

	intentName := m.Intent.Name
	if fn, ok := intentValidators[intentName]; ok {
		errs = append(errs, fn(m)...)
	}
	return errs
}

// intentValidators maps Intent.Name → per-intent validator function.
var intentValidators = map[string]func(*Message) ValidationErrors{
	// NeuralMemory requests
	"StoreEvent":       validateStoreEvent,
	"StoreBatchEvents": validateStoreBatchEvents,
	"StoreBatchTags":   validateStoreBatchTags,
	"GetEvent":         validateGetEvent,
	"GetEventsForTags": validateGetEventsForTags,
	"LinkEvent":        validateLinkEvent,
	"UnlinkEvent":      validateUnlinkEvent,
	"StoreBatchLinks":  validateStoreBatchLinks,

	// NeuralMemory responses
	"StoreEventResponse":       validateResponseIntent,
	"StoreBatchEventsResponse": validateResponseIntent,
	"StoreBatchTagsResponse":   validateResponseIntent,
	"GetEventResponse":         validateResponseIntent,
	"GetEventsForTagsResponse": validateResponseIntent,
	"LinkEventResponse":        validateResponseIntent,
	"UnlinkEventResponse":      validateResponseIntent,
	"StoreBatchLinksResponse":  validateResponseIntent,

	// Gateway / Actor
	"GatewayId":      validateGatewayId,
	"GatewayStreamOn":  validateGatewayStream,
	"GatewayStreamOff": validateGatewayStream,
	"ActorRequest":   validateActorRequest,
	"ActorResponse":  validateActorResponse,
	"ActorReport":    validateActorReport,
	"Status":         validateStatus,
	"StatusRequest":  validateStatusRequest,
}

// =============================================================================
// ENVELOPE VALIDATOR
// =============================================================================

func validateEnvelope(m *Message) ValidationErrors {
	var errs ValidationErrors
	intent := m.Intent.Name
	if intent == "" {
		intent = "(unknown)"
	}

	// To field
	if m.To == "" {
		errs = append(errs, requiredField(intent, "Envelope.To", "to",
			"Set Envelope.To to the recipient address in name@gateway format.",
			`msg.Envelope.To = "actor@gateway.example.com"`,
			"message/types.go:Envelope.To"))
	} else if !isNameAtGateway(m.To) {
		errs = append(errs, errorf("error", intent, "Envelope.To", "to", "format",
			"Envelope.To must be in name@gateway format.",
			"Set To to a string containing exactly one '@' with non-empty name and gateway parts.",
			`msg.Envelope.To = "actor@gateway.example.com"`,
			"message/types.go:Envelope.To"))
	}

	// From field
	if m.From == "" {
		errs = append(errs, requiredField(intent, "Envelope.From", "from",
			"Set Envelope.From to the sender address in name@gateway format.",
			`msg.Envelope.From = "client@gateway.example.com"`,
			"message/types.go:Envelope.From"))
	} else if !isNameAtGateway(m.From) {
		errs = append(errs, errorf("error", intent, "Envelope.From", "from", "format",
			"Envelope.From must be in name@gateway format.",
			"Set From to a string containing exactly one '@' with non-empty name and gateway parts.",
			`msg.Envelope.From = "client@gateway.example.com"`,
			"message/types.go:Envelope.From"))
	}

	// Intent must be non-zero
	if m.Intent.Name == "" {
		errs = append(errs, requiredField("(unknown)", "Envelope.Intent", "intent",
			"Set Envelope.Intent to a value from IntentType (e.g. IntentType.StoreEvent).",
			`msg.Envelope.Intent = message.IntentType.StoreEvent`,
			"message/intents.go:IntentType"))
	}

	// GatewayId requires ClientName
	if m.Intent.Name == IntentType.GatewayId.Name && m.ClientName == "" {
		errs = append(errs, requiredField(intent, "Envelope.ClientName", "id:name",
			"Set Envelope.ClientName to the unique name for this client connection.",
			`msg.Envelope.ClientName = "MyClient"`,
			"message/types.go:Envelope.ClientName"))
	}

	return errs
}

// =============================================================================
// NEURAL MEMORY REQUEST VALIDATORS
// =============================================================================

func validateStoreEvent(m *Message) ValidationErrors {
	const intent = "StoreEvent"
	var errs ValidationErrors

	if m.Event == nil {
		errs = append(errs, nilStruct(intent, "Event",
			"Initialize Event before building a StoreEvent message.",
			`msg.Event = &message.EventFields{Owner: "owner-id", Location: "TERRA|47.6|-122.5", LocationSeparator: "|"}`,
			"message/types.go:EventFields", "message/header.go:StoreEventMessageHeader"))
		return errs
	}

	if m.Event.Owner == "" && m.Event.OwnerUniqueID == "" {
		errs = append(errs, oneOfRequired(intent,
			"Event.Owner", "owner",
			"Event.OwnerUniqueID", "owner_unique_id",
			"Set Event.Owner or Event.OwnerUniqueID to identify the owning entity.",
			`msg.Event.Owner = "$sys"`,
			"message/types.go:EventFields.Owner", "message/header.go:StoreEventMessageHeader"))
	}
	if m.Event.Location == "" {
		errs = append(errs, requiredField(intent, "Event.Location", "loc",
			"Set Event.Location to a location string (e.g. TERRA|47.6|-122.5).",
			`msg.Event.Location = "TERRA|47.6|-122.5"`,
			"message/types.go:EventFields.Location", "message/header.go:StoreEventMessageHeader"))
	}
	if m.Event.LocationSeparator == "" {
		errs = append(errs, requiredField(intent, "Event.LocationSeparator", "loc_delim",
			"Set Event.LocationSeparator to the delimiter used in Event.Location.",
			`msg.Event.LocationSeparator = "|"`,
			"message/types.go:EventFields.LocationSeparator"))
	}
	return errs
}

func validateStoreBatchEvents(m *Message) ValidationErrors {
	const intent = "StoreBatchEvents"
	var errs ValidationErrors

	errs = append(errs, validatePayload(m)...)
	return errs
}

func validateStoreBatchTags(m *Message) ValidationErrors {
	const intent = "StoreBatchTags"
	var errs ValidationErrors

	if m.Event == nil {
		errs = append(errs, nilStruct(intent, "Event",
			"Initialize Event with the ID or UniqueId of the target event.",
			`msg.Event = &message.EventFields{Id: "event-id"}`,
			"message/types.go:EventFields"))
	} else {
		if m.Event.Id == "" && m.Event.UniqueId == "" {
			errs = append(errs, oneOfRequired(intent,
				"Event.Id", "event_id",
				"Event.UniqueId", "unique_id",
				"Set Event.Id or Event.UniqueId to identify the target event.",
				`msg.Event.Id = "2024.01.15..."`,
				"message/types.go:EventFields"))
		}
		if m.Event.Owner == "" && m.Event.OwnerUniqueID == "" {
			errs = append(errs, oneOfRequired(intent,
				"Event.Owner", "owner",
				"Event.OwnerUniqueID", "owner_unique_id",
				"Set Event.Owner or Event.OwnerUniqueID to identify the owning entity.",
				`msg.Event.Owner = "$sys"`,
				"message/types.go:EventFields.Owner"))
		}
	}

	errs = append(errs, validatePayload(m)...)
	return errs
}

func validateGetEvent(m *Message) ValidationErrors {
	const intent = "GetEvent"
	var errs ValidationErrors

	if m.Event == nil {
		errs = append(errs, nilStruct(intent, "Event",
			"Initialize Event with the ID or UniqueId of the event to retrieve.",
			`msg.Event = &message.EventFields{Id: "2024.01.15..."}`,
			"message/types.go:EventFields"))
		return errs
	}
	if m.Event.Id == "" && m.Event.UniqueId == "" {
		errs = append(errs, oneOfRequired(intent,
			"Event.Id", "event_id",
			"Event.UniqueId", "unique_id",
			"Set Event.Id or Event.UniqueId to identify the event to retrieve.",
			`msg.Event.Id = "2024.01.15..."`,
			"message/types.go:EventFields", "message/header.go:GetEventMessageHeader"))
	}
	return errs
}

func validateGetEventsForTags(m *Message) ValidationErrors {
	const intent = "GetEventsForTags"
	var errs ValidationErrors

	if m.NeuralMemory == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory",
			"Initialize NeuralMemory with GetEventsForTags options.",
			`msg.NeuralMemory = &message.NeuralMemoryFields{GetEventsForTags: &message.GetEventsForTagsOptions{BufferResults: true}}`,
			"message/types.go:NeuralMemoryFields"))
		return errs
	}
	if m.NeuralMemory.GetEventsForTags == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory.GetEventsForTags",
			"Initialize NeuralMemory.GetEventsForTags with search options.",
			`msg.NeuralMemory.GetEventsForTags = &message.GetEventsForTagsOptions{EventPattern: "my-key=my-value", BufferResults: true}`,
			"message/types.go:GetEventsForTagsOptions"))
	}
	return errs
}

func validateLinkEvent(m *Message) ValidationErrors {
	const intent = "LinkEvent"
	var errs ValidationErrors

	if m.NeuralMemory == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory",
			"Initialize NeuralMemory with a Link struct.",
			`msg.NeuralMemory = &message.NeuralMemoryFields{Link: &message.LinkFields{...}}`,
			"message/types.go:NeuralMemoryFields"))
		return errs
	}
	lk := m.NeuralMemory.Link
	if lk == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory.Link",
			"Initialize NeuralMemory.Link with the link definition.",
			`msg.NeuralMemory.Link = &message.LinkFields{EventA: "...", EventB: "...", Category: "related", StrengthA: 1.0, StrengthB: 1.0}`,
			"message/types.go:LinkFields", "message/header.go:LinkEventsMessageHeader"))
		return errs
	}

	// EventA+B or UniqueIdA+B
	hasIdPair := lk.EventA != "" && lk.EventB != ""
	hasUniqueIdPair := lk.UniqueIdA != "" && lk.UniqueIdB != ""
	if !hasIdPair && !hasUniqueIdPair {
		errs = append(errs, errorf("error", intent,
			"NeuralMemory.Link.EventA+EventB / NeuralMemory.Link.UniqueIdA+UniqueIdB",
			"event_id_a+event_id_b / unique_id_a+unique_id_b",
			"one_of_required",
			"Either (EventA AND EventB) or (UniqueIdA AND UniqueIdB) must be set on NeuralMemory.Link.",
			"Set both EventA and EventB, or both UniqueIdA and UniqueIdB.",
			`msg.NeuralMemory.Link.EventA = "a-id"\nmsg.NeuralMemory.Link.EventB = "b-id"`,
			"message/types.go:LinkFields", "message/header.go:LinkEventsMessageHeader"))
	}
	if lk.Category == "" {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.Category", "category",
			"Set NeuralMemory.Link.Category to a non-empty relationship string.",
			`msg.NeuralMemory.Link.Category = "related"`,
			"message/types.go:LinkFields.Category", "message/header.go:LinkEventsMessageHeader"))
	}
	if lk.StrengthA == 0 {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.StrengthA", "strength_a",
			"Set NeuralMemory.Link.StrengthA to the A→B link strength (e.g. 1.0).",
			`msg.NeuralMemory.Link.StrengthA = 1.0`,
			"message/types.go:LinkFields.StrengthA"))
	}
	if lk.StrengthB == 0 {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.StrengthB", "strength_b",
			"Set NeuralMemory.Link.StrengthB to the B→A link strength (e.g. 1.0).",
			`msg.NeuralMemory.Link.StrengthB = 1.0`,
			"message/types.go:LinkFields.StrengthB"))
	}
	if lk.Timestamp == "" {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.Timestamp", "timestamp",
			"Set NeuralMemory.Link.Timestamp to the link creation time (POSIX microseconds string).",
			`msg.NeuralMemory.Link.Timestamp = "+1234567890.123456"`,
			"message/types.go:LinkFields.Timestamp", "message/header.go:LinkEventsMessageHeader"))
	}
	if lk.OwnerID == "" && lk.OwnerUniqueID == "" {
		errs = append(errs, oneOfRequired(intent,
			"NeuralMemory.Link.OwnerID", "owner_event_id",
			"NeuralMemory.Link.OwnerUniqueID", "owner_unique_id",
			"Set NeuralMemory.Link.OwnerID or NeuralMemory.Link.OwnerUniqueID.",
			`msg.NeuralMemory.Link.OwnerID = "owner-event-id"`,
			"message/types.go:LinkFields.OwnerID"))
	}
	if lk.Location == "" {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.Location", "loc",
			"Set NeuralMemory.Link.Location to a location string (moved from Event.Location per pending code change).",
			`msg.NeuralMemory.Link.Location = "TERRA|47.6|-122.5"`,
			"message/types.go:LinkFields.Location", "message/header.go:LinkEventsMessageHeader"))
	}
	if lk.LocationSeparator == "" {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.LocationSeparator", "loc_delim",
			"Set NeuralMemory.Link.LocationSeparator to the delimiter used in Location.",
			`msg.NeuralMemory.Link.LocationSeparator = "|"`,
			"message/types.go:LinkFields.LocationSeparator"))
	}
	return errs
}

func validateUnlinkEvent(m *Message) ValidationErrors {
	const intent = "UnlinkEvent"
	var errs ValidationErrors

	if m.NeuralMemory == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory",
			"Initialize NeuralMemory with a Link struct.",
			`msg.NeuralMemory = &message.NeuralMemoryFields{Link: &message.LinkFields{Id: "link-event-id"}}`,
			"message/types.go:NeuralMemoryFields"))
		return errs
	}
	lk := m.NeuralMemory.Link
	if lk == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory.Link",
			"Initialize NeuralMemory.Link with the Id or UniqueId of the link to remove.",
			`msg.NeuralMemory.Link = &message.LinkFields{Id: "link-event-id"}`,
			"message/types.go:LinkFields", "message/header.go:UnlinkEventsMessageHeader"))
		return errs
	}
	if lk.Id == "" && lk.UniqueId == "" {
		errs = append(errs, oneOfRequired(intent,
			"NeuralMemory.Link.Id", "event_id",
			"NeuralMemory.Link.UniqueId", "unique_id",
			"Set NeuralMemory.Link.Id or NeuralMemory.Link.UniqueId to identify the link event object.",
			`msg.NeuralMemory.Link.Id = "link-event-id"`,
			"message/types.go:LinkFields", "message/header.go:UnlinkEventsMessageHeader"))
	}
	// LocationSeparator required when Location is set
	if lk.Location != "" && lk.LocationSeparator == "" {
		errs = append(errs, requiredField(intent, "NeuralMemory.Link.LocationSeparator", "loc_delim",
			"Set NeuralMemory.Link.LocationSeparator when Location is provided.",
			`msg.NeuralMemory.Link.LocationSeparator = "|"`,
			"message/types.go:LinkFields.LocationSeparator"))
	}
	return errs
}

func validateStoreBatchLinks(m *Message) ValidationErrors {
	const intent = "StoreBatchLinks"
	var errs ValidationErrors

	if m.NeuralMemory == nil {
		errs = append(errs, nilStruct(intent, "NeuralMemory",
			"Initialize NeuralMemory with BatchLinks slice.",
			`msg.NeuralMemory = &message.NeuralMemoryFields{BatchLinks: []message.BatchLinkEventSpec{...}}`,
			"message/types.go:NeuralMemoryFields"))
		return errs
	}
	errs = append(errs, validatePayload(m)...)
	return errs
}

// =============================================================================
// GATEWAY / ACTOR VALIDATORS
// =============================================================================

func validateGatewayId(m *Message) ValidationErrors {
	const intent = "GatewayId"
	var errs ValidationErrors

	if m.ClientName == "" {
		errs = append(errs, requiredField(intent, "Envelope.ClientName", "id:name",
			"Set Envelope.ClientName to the unique name for this client connection.",
			`msg.Envelope.ClientName = "MyClient"`,
			"message/types.go:Envelope.ClientName"))
	}
	// Passcode requires UserName and vice-versa
	if m.Passcode != "" && m.UserName == "" {
		errs = append(errs, requiredField(intent, "Envelope.UserName", "id:user",
			"Set Envelope.UserName when Passcode is provided.",
			`msg.Envelope.UserName = "admin"`,
			"message/types.go:Envelope.UserName"))
	}
	if m.UserName != "" && m.Passcode == "" {
		errs = append(errs, requiredField(intent, "Envelope.Passcode", "id:passcode",
			"Set Envelope.Passcode when UserName is provided.",
			`msg.Envelope.Passcode = "secret"`,
			"message/types.go:Envelope.Passcode"))
	}
	return errs
}

func validateGatewayStream(m *Message) ValidationErrors {
	// GatewayStreamOn / GatewayStreamOff: only envelope fields are required
	return nil
}

func validateActorRequest(m *Message) ValidationErrors {
	// ActorRequest: _type=status is always written by the encoder; no struct fields beyond envelope
	return nil
}

func validateActorResponse(m *Message) ValidationErrors {
	// ActorResponse: response payload; no required struct fields beyond envelope
	return nil
}

func validateActorReport(m *Message) ValidationErrors {
	const intent = "ActorReport"
	var errs ValidationErrors

	if m.Response == nil {
		errs = append(errs, nilStruct(intent, "Response",
			"Initialize Response with Status and Message for an ActorReport.",
			`msg.Response = &message.ResponseFields{Status: "OK", Message: "..."}`,
			"message/types.go:ResponseFields"))
		return errs
	}
	if m.Response.Status == "" {
		errs = append(errs, requiredField(intent, "Response.Status", "_status",
			"Set Response.Status to indicate actor health (e.g. 'OK').",
			`msg.Response.Status = "OK"`,
			"message/types.go:ResponseFields.Status"))
	}
	if m.Response.Message == "" {
		errs = append(errs, requiredField(intent, "Response.Message", "_msg",
			"Set Response.Message to a descriptive status string.",
			`msg.Response.Message = "actor is healthy"`,
			"message/types.go:ResponseFields.Message"))
	}
	return errs
}

func validateStatus(m *Message) ValidationErrors {
	// Status: no required struct fields beyond envelope
	return nil
}

func validateStatusRequest(m *Message) ValidationErrors {
	// StatusRequest: envelope-only; _msg_id is written by StatusRequestHeader when set
	return nil
}

func validateResponseIntent(m *Message) ValidationErrors {
	// Generic response validator: warns if Response is nil or Status empty.
	// Intent-specific responses are validated by the wire validator.
	intent := m.Intent.Name
	var errs ValidationErrors

	if m.Response == nil {
		errs = append(errs, errorf("warn", intent, "Response", "_status", "nil_struct",
			"Response is nil; the decoder should have populated it from the wire message.",
			"Ensure DecodeMessage was called before using the decoded message.",
			"decoded, err := message.DecodeMessage(raw)",
			"message/decoder.go:DecodeMessage"))
		return errs
	}
	if m.Response.Status == "" {
		errs = append(errs, errorf("warn", intent, "Response.Status", "_status", "required",
			"Response.Status is empty; it may not have been decoded correctly.",
			"Check that the raw message contains a _status header field.",
			"", "message/decoder.go:DecodeMessage"))
	}
	return errs
}

// =============================================================================
// PAYLOAD VALIDATOR
// =============================================================================

// validatePayload validates the payload contents for NeuralMemory batch intents.
// It is called from within per-intent validators. The intent is taken from m.Intent.Name.
func validatePayload(m *Message) ValidationErrors {
	intent := m.Intent.Name
	var errs ValidationErrors

	switch intent {
	case "StoreBatchEvents":
		if m.NeuralMemory == nil || len(m.NeuralMemory.BatchEvents) == 0 {
			// Also accept payload-based specification
			payloadData := m.PayloadData()
			if payloadData == nil {
				errs = append(errs, errorf("error", intent,
					"NeuralMemory.BatchEvents", "payload",
					"required",
					"StoreBatchEvents requires a non-empty NeuralMemory.BatchEvents slice.",
					"Populate NeuralMemory.BatchEvents with []BatchEventSpec records.",
					`msg.NeuralMemory = &message.NeuralMemoryFields{BatchEvents: []message.BatchEventSpec{{Event: message.EventFields{...}}}}`,
					"message/types.go:NeuralMemoryFields.BatchEvents",
					"message/encoder.go:FormatBatchEventsPayload"))
				return errs
			}
			// Payload-based: validate type
			if _, ok := payloadData.([]BatchEventSpec); !ok {
				errs = append(errs, errorf("error", intent,
					"Payload.Data", "payload",
					"payload_type",
					fmt.Sprintf("StoreBatchEvents payload must be []BatchEventSpec, got %T.", payloadData),
					"Cast or assign Payload.Data as []BatchEventSpec.",
					`msg.Payload = &message.PayloadFields{Data: []message.BatchEventSpec{{...}}}`,
					"message/types.go:BatchEventSpec"))
			}
			return errs
		}
		for i, spec := range m.NeuralMemory.BatchEvents {
			path := fmt.Sprintf("NeuralMemory.BatchEvents[%d].Event", i)
			if spec.Event.Timestamp == "" {
				errs = append(errs, requiredField(intent, path+".Timestamp", "timestamp",
					"Set a POSIX microsecond timestamp for this batch event.",
					`events[i].Event.Timestamp = "+1234567890.123456"`,
					"message/types.go:BatchEventSpec"))
			}
			if spec.Event.Owner == "" && spec.Event.OwnerUniqueID == "" {
				errs = append(errs, errorf("error", intent,
					fmt.Sprintf("%s.Owner / %s.OwnerUniqueID", path, path),
					"owner / owner_unique_id",
					"one_of_required",
					fmt.Sprintf("BatchEventSpec[%d]: Owner or OwnerUniqueID is required.", i),
					"Set Event.Owner or Event.OwnerUniqueID.",
					`events[i].Event.Owner = "$sys"`,
					"message/types.go:BatchEventSpec"))
			}
			if spec.Event.Location == "" {
				errs = append(errs, errorf("error", intent, path+".Location", "loc", "payload_format",
					fmt.Sprintf("BatchEventSpec[%d]: Location is required.", i),
					"Set Event.Location to a location string.",
					`events[i].Event.Location = "TERRA|47.6|-122.5"`,
					"message/types.go:BatchEventSpec"))
			}
			if spec.Event.LocationSeparator == "" {
				errs = append(errs, errorf("error", intent, path+".LocationSeparator", "loc_delim", "payload_format",
					fmt.Sprintf("BatchEventSpec[%d]: LocationSeparator is required.", i),
					"Set Event.LocationSeparator to match the delimiter in Location.",
					`events[i].Event.LocationSeparator = "|"`,
					"message/types.go:BatchEventSpec"))
			}
		}

	case "StoreBatchTags":
		if m.NeuralMemory == nil || len(m.NeuralMemory.Tags) == 0 {
			// Check Payload.Data for TagList
			payloadData := m.PayloadData()
			if payloadData != nil {
				switch pd := payloadData.(type) {
				case TagList:
					errs = append(errs, validateTagList(intent, pd)...)
				case []Tag:
					errs = append(errs, validateTagList(intent, pd)...)
				default:
					errs = append(errs, errorf("error", intent,
						"NeuralMemory.Tags / Payload.Data", "payload",
						"payload_type",
						fmt.Sprintf("StoreBatchTags payload must be TagList or []Tag, got %T.", payloadData),
						"Set NeuralMemory.Tags or Payload.Data to a TagList.",
						`msg.NeuralMemory.Tags = message.TagList{{Key: "k", Value: "v"}}`,
						"message/types.go:TagList"))
				}
			} else {
				errs = append(errs, errorf("error", intent,
					"NeuralMemory.Tags", "payload",
					"required",
					"StoreBatchTags requires a non-empty NeuralMemory.Tags (TagList).",
					"Populate NeuralMemory.Tags with Tag records.",
					`msg.NeuralMemory.Tags = message.TagList{{Key: "category", Value: "value"}}`,
					"message/types.go:TagList"))
			}
		} else {
			errs = append(errs, validateTagList(intent, m.NeuralMemory.Tags)...)
		}

	case "StoreBatchLinks":
		if m.NeuralMemory == nil || len(m.NeuralMemory.BatchLinks) == 0 {
			errs = append(errs, errorf("error", intent,
				"NeuralMemory.BatchLinks", "payload",
				"required",
				"StoreBatchLinks requires a non-empty NeuralMemory.BatchLinks slice.",
				"Populate NeuralMemory.BatchLinks with []BatchLinkEventSpec records.",
				`msg.NeuralMemory.BatchLinks = []message.BatchLinkEventSpec{{Event: message.EventFields{...}, Link: message.LinkFields{...}}}`,
				"message/types.go:BatchLinkEventSpec"))
			return errs
		}
		for i, spec := range m.NeuralMemory.BatchLinks {
			evPath := fmt.Sprintf("NeuralMemory.BatchLinks[%d].Event", i)
			lkPath := fmt.Sprintf("NeuralMemory.BatchLinks[%d].Link", i)

			if spec.Event.Timestamp == "" {
				errs = append(errs, errorf("error", intent, evPath+".Timestamp", "timestamp", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Event.Timestamp is required.", i),
					"Set a POSIX microsecond timestamp for the link event.",
					`links[i].Event.Timestamp = "+1234567890.123456"`,
					"message/types.go:BatchLinkEventSpec"))
			}
			if spec.Event.Owner == "" && spec.Event.OwnerUniqueID == "" {
				errs = append(errs, errorf("error", intent,
					fmt.Sprintf("%s.Owner / %s.OwnerUniqueID", evPath, evPath),
					"owner / owner_unique_id", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Event.Owner or Event.OwnerUniqueID is required.", i),
					"Set Event.Owner or Event.OwnerUniqueID.",
					`links[i].Event.Owner = "$sys"`,
					"message/types.go:BatchLinkEventSpec"))
			}
			if spec.Link.Timestamp == "" {
				errs = append(errs, errorf("error", intent, lkPath+".Timestamp", "timestamp", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Link.Timestamp is required (NOT auto-generated).", i),
					"Set Link.Timestamp explicitly to the link creation time.",
					`links[i].Link.Timestamp = "+1234567890.123456"`,
					"message/types.go:LinkFields.Timestamp"))
			}
			hasIdPair := spec.Link.EventA != "" && spec.Link.EventB != ""
			hasUidPair := spec.Link.UniqueIdA != "" && spec.Link.UniqueIdB != ""
			if !hasIdPair && !hasUidPair {
				errs = append(errs, errorf("error", intent,
					fmt.Sprintf("%s.EventA+EventB / %s.UniqueIdA+UniqueIdB", lkPath, lkPath),
					"event_id_a+event_id_b / unique_id_a+unique_id_b", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: (EventA AND EventB) or (UniqueIdA AND UniqueIdB) required.", i),
					"Set both EventA and EventB, or both UniqueIdA and UniqueIdB.",
					`links[i].Link.EventA = "a"\nlinks[i].Link.EventB = "b"`,
					"message/types.go:LinkFields"))
			}
			if spec.Link.Category == "" {
				errs = append(errs, errorf("error", intent, lkPath+".Category", "category", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Link.Category is required.", i),
					"Set Link.Category to a relationship string.",
					`links[i].Link.Category = "related"`,
					"message/types.go:LinkFields.Category"))
			}
			if spec.Link.StrengthA == 0 {
				errs = append(errs, errorf("error", intent, lkPath+".StrengthA", "strength_a", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Link.StrengthA is required.", i),
					"Set Link.StrengthA to a non-zero float.",
					`links[i].Link.StrengthA = 1.0`,
					"message/types.go:LinkFields.StrengthA"))
			}
			if spec.Link.StrengthB == 0 {
				errs = append(errs, errorf("error", intent, lkPath+".StrengthB", "strength_b", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Link.StrengthB is required.", i),
					"Set Link.StrengthB to a non-zero float.",
					`links[i].Link.StrengthB = 1.0`,
					"message/types.go:LinkFields.StrengthB"))
			}
			if spec.Link.OwnerID == "" && spec.Link.OwnerUniqueID == "" {
				errs = append(errs, errorf("error", intent,
					fmt.Sprintf("%s.OwnerID / %s.OwnerUniqueID", lkPath, lkPath),
					"owner_event_id / owner_unique_id", "payload_format",
					fmt.Sprintf("BatchLinkEventSpec[%d]: Link.OwnerID or Link.OwnerUniqueID is required.", i),
					"Set Link.OwnerID or Link.OwnerUniqueID.",
					`links[i].Link.OwnerID = "owner-event-id"`,
					"message/types.go:LinkFields.OwnerID"))
			}
		}
	}

	return errs
}

// validateTagList checks each Tag in a TagList for required fields.
func validateTagList(intent string, tags TagList) ValidationErrors {
	var errs ValidationErrors
	for i, tag := range tags {
		if tag.Key == "" {
			errs = append(errs, errorf("error", intent,
				fmt.Sprintf("NeuralMemory.Tags[%d].Key", i), "key", "payload_format",
				fmt.Sprintf("Tag[%d]: Key is required.", i),
				"Set Tag.Key to a non-empty category string.",
				`tags[i].Key = "category"`,
				"message/types.go:Tag.Key"))
		}
		if tag.Value == nil {
			errs = append(errs, errorf("error", intent,
				fmt.Sprintf("NeuralMemory.Tags[%d].Value", i), "value", "payload_format",
				fmt.Sprintf("Tag[%d]: Value must not be nil.", i),
				"Set Tag.Value to a non-nil value.",
				`tags[i].Value = "some-value"`,
				"message/types.go:Tag.Value"))
		}
	}
	return errs
}

// =============================================================================
// WIRE VALIDATOR — ValidateRawMessage
// =============================================================================

// ValidateRawMessage validates a raw wire-format []byte in two stages:
//
//  1. Wire framing: checks length prefixes, To/From format, and messageType.
//  2. Per-intent header fields: checks required header keys for the resolved intent.
//
// Returns nil immediately if validation is disabled (PODOS_VALIDATE not set).
func ValidateRawMessage(raw []byte) ValidationErrors {
	if !validationEnabled {
		return nil
	}

	const ctx = "wire"
	var errs ValidationErrors

	// Stage 1 — nil and size bounds
	if raw == nil {
		errs = append(errs, errorf("error", ctx, "message", "", "nil_struct",
			"Raw message is nil.",
			"Ensure the raw []byte is non-nil before calling ValidateRawMessage.",
			"", "message/decoder.go:DecodeMessage"))
		return errs
	}

	const minSize = 63
	if len(raw) < minSize {
		errs = append(errs, errorf("error", ctx, "message", "", "format",
			fmt.Sprintf("Raw message is %d bytes; minimum is %d bytes (7 × 9-byte length fields).", len(raw), minSize),
			"Check that the wire message was not truncated.",
			"", "message/decoder.go:DecodeMessage"))
		return errs
	}

	if int64(len(raw)) > MaxMessageSizeBytes {
		errs = append(errs, errorf("error", ctx, "message", "", "format",
			fmt.Sprintf("Raw message is %d bytes; maximum is %d bytes.", len(raw), MaxMessageSizeBytes),
			"Reduce payload size or increase MaxMessageSizeBytes.",
			"", "message/constants.go:MaxMessageSizeBytes"))
		return errs
	}

	// Stage 1 — parse length prefix fields
	type sizeField struct {
		name  string
		start int
		end   int
	}
	fields := []sizeField{
		{"totalLength", 0, 9},
		{"toLength", 9, 18},
		{"fromLength", 18, 27},
		{"headerLength", 27, 36},
		{"messageType", 36, 45},
		{"dataType", 45, 54},
		{"payloadDataLength", 54, 63},
	}

	parsed := make([]int64, len(fields))
	for i, f := range fields {
		v, err := decodeMessageSizeParam(raw[f.start:f.end])
		if err != nil {
			errs = append(errs, errorf("error", ctx, f.name, "", "format",
				fmt.Sprintf("Failed to parse length field '%s': %v", f.name, err),
				fmt.Sprintf("Ensure bytes [%d:%d] form a valid hex or decimal integer.", f.start, f.end),
				"", "message/decoder.go:decodeMessageSizeParam"))
			return errs
		}
		parsed[i] = v
	}

	// totalLength=parsed[0], toLength=parsed[1], fromLength=parsed[2],
	// headerLength=parsed[3], messageType=parsed[4], payloadDataLength=parsed[6]
	toLength := parsed[1]
	fromLength := parsed[2]
	headerLength := parsed[3]
	messageType := int(parsed[4])
	payloadDataLength := parsed[6]

	const prefixBytes int64 = 63
	toStart := prefixBytes
	toEnd := toStart + toLength
	fromStart := toEnd
	fromEnd := fromStart + fromLength
	headerStart := fromEnd
	headerEnd := headerStart + headerLength

	// Validate we have enough bytes
	if int64(len(raw)) < toEnd {
		errs = append(errs, errorf("error", ctx, "to", "to", "format",
			fmt.Sprintf("Message too short for 'to' field: need %d bytes, have %d.", toEnd, len(raw)),
			"", "", "message/decoder.go:DecodeMessage"))
		return errs
	}
	if int64(len(raw)) < fromEnd {
		errs = append(errs, errorf("error", ctx, "from", "from", "format",
			fmt.Sprintf("Message too short for 'from' field: need %d bytes, have %d.", fromEnd, len(raw)),
			"", "", "message/decoder.go:DecodeMessage"))
		return errs
	}
	if int64(len(raw)) < headerEnd {
		errs = append(errs, errorf("error", ctx, "header", "header", "format",
			fmt.Sprintf("Message too short for header: need %d bytes, have %d.", headerEnd, len(raw)),
			"", "", "message/decoder.go:DecodeMessage"))
		return errs
	}

	// Stage 1 — To and From format
	toStr := string(raw[toStart:toEnd])
	if toStr == "" {
		errs = append(errs, errorf("error", ctx, "to", "to", "required",
			"'to' field is empty.",
			"Ensure the encoded message has a non-empty 'to' address.",
			`msg.Envelope.To = "actor@gateway.example.com"`,
			"message/types.go:Envelope.To"))
	} else if !isNameAtGateway(toStr) {
		errs = append(errs, errorf("error", ctx, "to", "to", "format",
			fmt.Sprintf("'to' field %q is not in name@gateway format.", toStr),
			"Use a 'to' address containing exactly one '@' with non-empty parts.",
			`msg.Envelope.To = "actor@gateway.example.com"`,
			"message/types.go:Envelope.To"))
	}

	fromStr := string(raw[fromStart:fromEnd])
	// Strip routing suffix (Pod-OS may append |gateway,client,timestamp)
	if pipeIdx := strings.IndexByte(fromStr, '|'); pipeIdx != -1 {
		fromStr = fromStr[:pipeIdx]
	}
	if fromStr == "" {
		errs = append(errs, errorf("error", ctx, "from", "from", "required",
			"'from' field is empty.",
			"Ensure the encoded message has a non-empty 'from' address.",
			`msg.Envelope.From = "client@gateway.example.com"`,
			"message/types.go:Envelope.From"))
	} else if !isNameAtGateway(fromStr) {
		errs = append(errs, errorf("error", ctx, "from", "from", "format",
			fmt.Sprintf("'from' field %q is not in name@gateway format.", fromStr),
			"Use a 'from' address containing exactly one '@' with non-empty parts.",
			`msg.Envelope.From = "client@gateway.example.com"`,
			"message/types.go:Envelope.From"))
	}

	if len(errs) > 0 {
		return errs
	}

	// Stage 1 — messageType must be a known Intent.MessageType
	if !isKnownMessageType(messageType) {
		errs = append(errs, errorf("error", ctx, "messageType", "messageType", "format",
			fmt.Sprintf("messageType %d is not a recognised Intent.MessageType.", messageType),
			"Set Intent to a value from IntentType; the MessageType is derived automatically.",
			`msg.Envelope.Intent = message.IntentType.StoreEvent`,
			"message/intents.go:IntentType"))
		return errs
	}

	// Stage 2 — parse header and validate per-intent fields
	headerStr := string(raw[headerStart:headerEnd])
	headerMap, err := decodeHeader(headerStr)
	if err != nil {
		errs = append(errs, errorf("error", ctx, "header", "header", "format",
			fmt.Sprintf("Header parse error: %v", err),
			"Ensure the header is tab-separated key=value pairs.",
			"", "message/decoder.go:decodeHeader"))
		return errs
	}

	errs = append(errs, validateWireHeader(messageType, headerMap, payloadDataLength)...)
	return errs
}

// isKnownMessageType returns true if t matches any Intent.MessageType defined in IntentType.
var knownMessageTypes = func() map[int]bool {
	m := map[int]bool{}
	it := IntentType
	// Collect all non-zero message types via the exported intentTypes struct
	for _, intent := range []Intent{
		it.StoreEvent, it.StoreBatchEvents, it.StoreBatchTags,
		it.GetEvent, it.GetEventsForTags,
		it.LinkEvent, it.UnlinkEvent, it.StoreBatchLinks,
		it.StoreEventResponse, it.StoreBatchEventsResponse, it.StoreBatchTagsResponse,
		it.GetEventResponse, it.GetEventsForTagsResponse,
		it.LinkEventResponse, it.UnlinkEventResponse, it.StoreBatchLinksResponse,
		it.GatewayId, it.GatewayStatus, it.GatewayDisconnect,
		it.GatewayStreamOn, it.GatewayStreamOff,
		it.GatewayBatchStart, it.GatewayBatchEnd, it.GatewaySendNext, it.GatewayNoSend,
		it.ActorEcho, it.ActorHalt, it.ActorStart,
		it.ActorRequest, it.ActorResponse, it.ActorReport,
		it.ActorRecord, it.ActorUser,
		it.Status, it.Keepalive,
		it.RouteAnyMessage, it.RouteUserOnlyMessage,
		it.QueueNextRequest, it.QueueAllRequest, it.QueueCountRequest, it.QueueEmpty,
		it.ReportRequest, it.InformationReport,
		it.AuthAddUser, it.AuthUpdateUser, it.AuthUserList, it.AuthDisableUser,
	} {
		if intent.MessageType != 0 {
			m[intent.MessageType] = true
		}
	}
	return m
}()

func isKnownMessageType(t int) bool {
	return knownMessageTypes[t]
}

// hasHeader checks for presence and non-empty value of a wire header key.
func hasHeader(h map[string]string, key string) bool {
	v, ok := h[key]
	return ok && v != ""
}

// validateWireHeader performs Stage 2 validation: per-intent header field checks.
func validateWireHeader(messageType int, h map[string]string, payloadLength int64) ValidationErrors {
	const ctx = "wire"
	var errs ValidationErrors

	// All intents: optional _msg_id must be non-empty if present
	if msgId, ok := h["_msg_id"]; ok && msgId == "" {
		errs = append(errs, errorf("warn", ctx, "Envelope.MessageId", "_msg_id", "format",
			"_msg_id header is present but empty; omit it or supply a non-empty value.",
			"Set Envelope.MessageId to a UUID or remove it.",
			`msg.Envelope.MessageId = uuid.New().String()`,
			"message/types.go:Envelope.MessageId"))
	}

	switch messageType {
	// ---- NeuralMemory request (1000) ----
	case 1000:
		dbCmd, ok := h["_db_cmd"]
		if !ok || dbCmd == "" {
			errs = append(errs, errorf("error", ctx, "_db_cmd", "_db_cmd", "header_missing",
				"NeuralMemory request (messageType 1000) is missing _db_cmd header.",
				"Set Intent to a NeuralMemory IntentType; the encoder writes _db_cmd automatically.",
				`msg.Envelope.Intent = message.IntentType.StoreEvent`,
				"message/header.go", "message/intents.go"))
			return errs
		}
		if _, known := commandToIntent[dbCmd]; !known {
			errs = append(errs, errorf("error", ctx, "_db_cmd", "_db_cmd", "header_value",
				fmt.Sprintf("_db_cmd=%q is not a known NeuralMemory command.", dbCmd),
				"Use a command produced by a NeuralMemory IntentType.",
				`msg.Envelope.Intent = message.IntentType.StoreEvent`,
				"message/intents.go:commandToIntent"))
		}
		errs = append(errs, validateNeuralMemoryRequestHeader(dbCmd, h, payloadLength)...)

	// ---- NeuralMemory response (1001) ----
	case 1001:
		errs = append(errs, validateNeuralMemoryResponseHeader(h)...)

	// ---- GatewayId (5) ----
	case 5:
		if !hasHeader(h, "id:name") {
			errs = append(errs, errorf("error", ctx, "Envelope.ClientName", "id:name", "header_missing",
				"GatewayId message is missing required id:name header.",
				"Set Envelope.ClientName; the encoder writes id:name automatically.",
				`msg.Envelope.ClientName = "MyClient"`,
				"message/types.go:Envelope.ClientName", "message/header.go:GatewayIdMessageHeader"))
		}

	// ---- ActorEcho (2) ----
	case 2:
		if !hasHeader(h, "_msg_id") {
			errs = append(errs, errorf("warn", ctx, "Envelope.MessageId", "_msg_id", "header_missing",
				"ActorEcho message is missing _msg_id; echo responses may not be correlatable.",
				"Set Envelope.MessageId to a UUID.",
				`msg.Envelope.MessageId = uuid.New().String()`,
				"message/types.go:Envelope.MessageId"))
		}

	// ---- ActorRequest (4) ----
	case 4:
		if v, ok := h["_type"]; !ok || v != "status" {
			errs = append(errs, errorf("error", ctx, "_type", "_type", "header_missing",
				"ActorRequest message must have _type=status header.",
				"Use IntentType.ActorRequest; the encoder writes _type=status automatically.",
				`msg.Envelope.Intent = message.IntentType.ActorRequest`,
				"message/header.go:ActorRequestMessageHeader"))
		}

	// ---- GatewayStreamOn (10) / GatewayStreamOff (9) ----
	case 9, 10:
		// No required header fields beyond envelope; no action needed.

	// ---- StatusRequest (110) ----
	case 110:
		if !hasHeader(h, "_msg_id") {
			errs = append(errs, errorf("warn", ctx, "Envelope.MessageId", "_msg_id", "header_missing",
				"StatusRequest message is missing _msg_id; health-check responses may not be correlatable.",
				"Set Envelope.MessageId to a UUID.",
				`msg.Envelope.MessageId = uuid.New().String()`,
				"message/types.go:Envelope.MessageId", "message/header.go:StatusRequestHeader"))
		}

	// ---- Status (3) / ActorResponse (30) / ActorReport (19) ----
	case 3, 19, 30:
		// No required header fields; presence of _status is encouraged but not enforced at wire level.

	default:
		errs = append(errs, warnUncovered(ctx,
			fmt.Sprintf("messageType %d is currently uncovered; wire header validation is in development.", messageType)))
	}

	return errs
}

func validateNeuralMemoryRequestHeader(cmd string, h map[string]string, payloadLength int64) ValidationErrors {
	const ctx = "wire"
	var errs ValidationErrors

	switch cmd {
	case "store":
		if !hasHeader(h, "timestamp") {
			errs = append(errs, errorf("warn", ctx, "Event.Timestamp", "timestamp", "header_missing",
				"StoreEvent header is missing 'timestamp'; the encoder should write it automatically.",
				"Verify that EncodeMessage was called and Event.Timestamp was set.",
				`msg.Event.Timestamp = "+1234567890.123456"`,
				"message/header.go:StoreEventMessageHeader"))
		}

	case "store_batch":
		if payloadLength == 0 {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.BatchEvents", "payload", "required",
				"StoreBatchEvents payload is empty; batch events must be encoded in the payload.",
				"Populate NeuralMemory.BatchEvents before encoding.",
				`msg.NeuralMemory.BatchEvents = []message.BatchEventSpec{{...}}`,
				"message/encoder.go:FormatBatchEventsPayload"))
		}

	case "tag_store_batch":
		if !hasHeader(h, "event_id") && !hasHeader(h, "unique_id") {
			errs = append(errs, errorf("error", ctx, "Event.Id / Event.UniqueId", "event_id / unique_id", "header_missing",
				"StoreBatchTags header is missing event_id or unique_id.",
				"Set Event.Id or Event.UniqueId to identify the target event.",
				`msg.Event.Id = "2024.01.15..."`,
				"message/header.go:StoreBatchTagsMessageHeader"))
		}
		if !hasHeader(h, "owner") && !hasHeader(h, "owner_unique_id") {
			errs = append(errs, errorf("error", ctx, "Event.Owner / Event.OwnerUniqueID", "owner / owner_unique_id", "header_missing",
				"StoreBatchTags header is missing owner or owner_unique_id.",
				"Set Event.Owner or Event.OwnerUniqueID.",
				`msg.Event.Owner = "$sys"`,
				"message/header.go:StoreBatchTagsMessageHeader"))
		}

	case "get":
		if !hasHeader(h, "event_id") && !hasHeader(h, "unique_id") {
			errs = append(errs, errorf("error", ctx, "Event.Id / Event.UniqueId", "event_id / unique_id", "header_missing",
				"GetEvent header is missing event_id or unique_id.",
				"Set Event.Id or Event.UniqueId to identify the event to retrieve.",
				`msg.Event.Id = "2024.01.15..."`,
				"message/header.go:GetEventMessageHeader"))
		}

	case "events_for_tag":
		if !hasHeader(h, "buffer_results") {
			errs = append(errs, errorf("warn", ctx, "NeuralMemory.GetEventsForTags.BufferResults", "buffer_results", "header_missing",
				"GetEventsForTags header is missing buffer_results; this field is expected.",
				"Set NeuralMemory.GetEventsForTags.BufferResults.",
				`msg.NeuralMemory.GetEventsForTags.BufferResults = true`,
				"message/types.go:GetEventsForTagsOptions.BufferResults"))
		}

	case "link":
		if !hasHeader(h, "strength_a") {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.Link.StrengthA", "strength_a", "header_missing",
				"LinkEvent header is missing strength_a.", "Set NeuralMemory.Link.StrengthA.",
				`msg.NeuralMemory.Link.StrengthA = 1.0`,
				"message/header.go:LinkEventsMessageHeader"))
		}
		if !hasHeader(h, "strength_b") {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.Link.StrengthB", "strength_b", "header_missing",
				"LinkEvent header is missing strength_b.", "Set NeuralMemory.Link.StrengthB.",
				`msg.NeuralMemory.Link.StrengthB = 1.0`,
				"message/header.go:LinkEventsMessageHeader"))
		}
		if !hasHeader(h, "category") {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.Link.Category", "category", "header_missing",
				"LinkEvent header is missing category.", "Set NeuralMemory.Link.Category.",
				`msg.NeuralMemory.Link.Category = "related"`,
				"message/header.go:LinkEventsMessageHeader"))
		}
		if !hasHeader(h, "timestamp") {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.Link.Timestamp", "timestamp", "header_missing",
				"LinkEvent header is missing timestamp.", "Set NeuralMemory.Link.Timestamp.",
				`msg.NeuralMemory.Link.Timestamp = "+1234567890.123456"`,
				"message/header.go:LinkEventsMessageHeader"))
		}
		if !hasHeader(h, "owner_event_id") && !hasHeader(h, "owner_unique_id") {
			errs = append(errs, errorf("error", ctx,
				"NeuralMemory.Link.OwnerID / NeuralMemory.Link.OwnerUniqueID",
				"owner_event_id / owner_unique_id", "header_missing",
				"LinkEvent header is missing owner_event_id or owner_unique_id.",
				"Set NeuralMemory.Link.OwnerID or NeuralMemory.Link.OwnerUniqueID.",
				`msg.NeuralMemory.Link.OwnerID = "owner-id"`,
				"message/header.go:LinkEventsMessageHeader"))
		}
		hasEventIds := hasHeader(h, "event_id_a") && hasHeader(h, "event_id_b")
		hasUniqueIds := hasHeader(h, "unique_id_a") && hasHeader(h, "unique_id_b")
		if !hasEventIds && !hasUniqueIds {
			errs = append(errs, errorf("error", ctx,
				"NeuralMemory.Link.EventA+EventB / NeuralMemory.Link.UniqueIdA+UniqueIdB",
				"event_id_a+event_id_b / unique_id_a+unique_id_b", "header_missing",
				"LinkEvent header is missing event pair (event_id_a+event_id_b or unique_id_a+unique_id_b).",
				"Set both EventA and EventB, or both UniqueIdA and UniqueIdB on NeuralMemory.Link.",
				`msg.NeuralMemory.Link.EventA = "a"\nmsg.NeuralMemory.Link.EventB = "b"`,
				"message/header.go:LinkEventsMessageHeader"))
		}

	case "unlink":
		if !hasHeader(h, "event_id") && !hasHeader(h, "unique_id") {
			errs = append(errs, errorf("error", ctx,
				"NeuralMemory.Link.Id / NeuralMemory.Link.UniqueId",
				"event_id / unique_id", "header_missing",
				"UnlinkEvent header is missing event_id or unique_id.",
				"Set NeuralMemory.Link.Id or NeuralMemory.Link.UniqueId.",
				`msg.NeuralMemory.Link.Id = "link-event-id"`,
				"message/header.go:UnlinkEventsMessageHeader"))
		}

	case "link_batch":
		if payloadLength == 0 {
			errs = append(errs, errorf("error", ctx, "NeuralMemory.BatchLinks", "payload", "required",
				"StoreBatchLinks payload is empty; batch link records must be encoded in the payload.",
				"Populate NeuralMemory.BatchLinks before encoding.",
				`msg.NeuralMemory.BatchLinks = []message.BatchLinkEventSpec{{...}}`,
				"message/encoder.go:FormatBatchLinksPayload"))
		}

	default:
		errs = append(errs, warnUncovered(ctx,
			fmt.Sprintf("NeuralMemory command %q is currently uncovered; header validation is in development.", cmd)))
	}

	return errs
}

func validateNeuralMemoryResponseHeader(h map[string]string) ValidationErrors {
	const ctx = "wire"
	var errs ValidationErrors

	// _status: WARN (not ERROR) because brief-hit responses may omit it
	if !hasHeader(h, "_status") {
		errs = append(errs, errorf("warn", ctx, "Response.Status", "_status", "header_missing",
			"NeuralMemory response (messageType 1001) is missing _status header.",
			"This is expected for brief-hit responses. For other responses, check the Evolutionary Neural Memory Actor.",
			"", "message/decoder.go:DecodeMessage"))
	}

	// Command-specific response checks
	dbCmd := h["_type"]
	if dbCmd == "" {
		dbCmd = h["_command"]
	}
	if dbCmd == "" {
		dbCmd = h["_db_cmd"]
	}

	switch dbCmd {
	case "get":
		if !hasHeader(h, "_event_id") && !hasHeader(h, "event_id") {
			errs = append(errs, errorf("warn", ctx, "Event.Id", "_event_id / event_id", "header_missing",
				"GetEventResponse is missing _event_id or event_id.",
				"", "", "message/decoder.go"))
		}
	case "link":
		if !hasHeader(h, "link_event") {
			errs = append(errs, errorf("warn", ctx, "Response.LinkId", "link_event", "header_missing",
				"LinkEventResponse is missing link_event (the assigned link ID).",
				"", "", "message/decoder.go"))
		}
	case "store", "store_batch", "tag_store_batch":
		if !hasHeader(h, "_count") {
			errs = append(errs, errorf("warn", ctx, "Response.TotalEvents", "_count", "header_missing",
				fmt.Sprintf("%s response is missing _count.", dbCmd),
				"", "", "message/decoder.go"))
		}
	case "link_batch":
		if !hasHeader(h, "_links_ok") {
			errs = append(errs, errorf("warn", ctx, "Response.StorageSuccessCount", "_links_ok", "header_missing",
				"StoreBatchLinksResponse is missing _links_ok.",
				"", "", "message/decoder.go"))
		}
	}

	return errs
}

// =============================================================================
// AI-ASSISTED REMEDIATION — ExplainValidationErrors (vLLM integration)
// =============================================================================

// vllmChatMessage represents a single message in a vLLM chat completion request.
type vllmChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// vllmChatRequest is the OpenAI-compatible chat completion request body.
type vllmChatRequest struct {
	Model       string            `json:"model"`
	Messages    []vllmChatMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature"`
}

// vllmChatResponse represents a minimal OpenAI-compatible chat completion response.
type vllmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// renderErrorPrompt produces the per-error prompt text from a ValidationError.
func renderErrorPrompt(ve ValidationError) string {
	refs := strings.Join(ve.References, ", ")
	return fmt.Sprintf(
		"You are a Pod-OS Go client expert. A message validation error occurred.\n\n"+
			"Intent: %s\n"+
			"Struct Path: %s\n"+
			"Wire Field: %s\n"+
			"Rule Violated: %s\n"+
			"Description: %s\n"+
			"Suggested Fix: %s\n"+
			"Example Code: %s\n"+
			"Source References: %s\n\n"+
			"Task: Provide corrected Go code for this message construction. Show all required fields "+
			"for the %s intent. If multiple valid approaches exist (e.g. EventA/EventB vs UniqueIdA/UniqueIdB), "+
			"show both. Use only types from the message package.",
		ve.Intent, ve.Field, ve.WireField, ve.Rule,
		ve.Message, ve.Fix, ve.ExampleCode, refs, ve.Intent,
	)
}

// ExplainValidationErrors submits validation errors to a vLLM-hosted endpoint for
// AI-assisted remediation. The endpoint must implement the OpenAI-compatible
// /v1/chat/completions interface (e.g. vLLM server).
//
// endpoint: base URL, e.g. "http://localhost:8000"
// model: model name to request, e.g. "meta-llama/Llama-3.1-8B-Instruct"
//
// Returns a combined AI-generated explanation and corrected code snippet, or an error.
// This function is a no-op (returns "", nil) when validation is disabled.
func ExplainValidationErrors(errs ValidationErrors, endpoint, model string) (string, error) {
	if !validationEnabled || len(errs) == 0 {
		return "", nil
	}

	if endpoint == "" {
		return "", fmt.Errorf("vLLM endpoint is required")
	}
	if model == "" {
		model = "default"
	}

	var sb strings.Builder
	for _, ve := range errs {
		prompt := renderErrorPrompt(ve)

		reqBody := vllmChatRequest{
			Model: model,
			Messages: []vllmChatMessage{
				{Role: "system", Content: "You are an expert in the Pod-OS Go client message library. Provide concise, correct Go code examples."},
				{Role: "user", Content: prompt},
			},
			MaxTokens:   512,
			Temperature: 0.1,
		}

		reqBytes, err := json.Marshal(reqBody)
		if err != nil {
			return sb.String(), fmt.Errorf("marshal vLLM request: %w", err)
		}

		httpClient := &http.Client{Timeout: 30 * time.Second}
		resp, err := httpClient.Post(
			endpoint+"/v1/chat/completions",
			"application/json",
			bytes.NewReader(reqBytes),
		)
		if err != nil {
			return sb.String(), fmt.Errorf("vLLM request failed: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return sb.String(), fmt.Errorf("read vLLM response: %w", err)
		}

		var chatResp vllmChatResponse
		if err := json.Unmarshal(body, &chatResp); err != nil {
			return sb.String(), fmt.Errorf("unmarshal vLLM response: %w", err)
		}

		if len(chatResp.Choices) > 0 {
			fmt.Fprintf(&sb, "=== %s / %s ===\n%s\n\n",
				ve.Intent, ve.Rule, chatResp.Choices[0].Message.Content)
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}
