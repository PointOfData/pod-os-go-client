package message

import (
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// SHARED TEST HELPERS
// =============================================================================

// withValidation enables validation for the duration of one test and restores
// the original state via the returned cleanup func.
func withValidation(t *testing.T) func() {
	t.Helper()
	orig := validationEnabled
	validationEnabled = true
	return func() { validationEnabled = orig }
}

// envelopeFor returns a minimal valid envelope for the given intent.
func envelopeFor(intent Intent) Envelope {
	return Envelope{
		To:     "mem@zeroth.pod-os.com",
		From:   "test@zeroth.pod-os.com",
		Intent: intent,
	}
}

// assertNoErrors fails the test if any error-severity ValidationError is present.
func assertNoErrors(t *testing.T, errs ValidationErrors, context string) {
	t.Helper()
	if hasErrors(errs) {
		t.Errorf("%s: unexpected errors:\n%s", context, errs.Error())
	}
}

// assertRule fails the test if no error with the given rule is found.
func assertRule(t *testing.T, errs ValidationErrors, rule, context string) {
	t.Helper()
	if !containsRule(errs, rule) {
		t.Errorf("%s: expected rule=%q, got:\n%s", context, rule, errs.Error())
	}
}

// assertField fails the test if no error mentioning the given field is found.
func assertField(t *testing.T, errs ValidationErrors, field, context string) {
	t.Helper()
	if !containsField(errs, field) {
		t.Errorf("%s: expected field=%q, got:\n%s", context, field, errs.Error())
	}
}

// assertWarn fails the test if no warn-severity ValidationError is present.
func assertWarn(t *testing.T, errs ValidationErrors, context string) {
	t.Helper()
	if !containsWarn(errs) {
		t.Errorf("%s: expected at least one WARN, got:\n%s", context, errs.Error())
	}
}

// rawMsg builds a minimal valid wire message using the shared buildMinimalMessage helper
// (defined in decoder_test.go in the same package).
func rawMsg(to, from, header string, msgType int, payload string) []byte {
	return buildMinimalMessage(to, from, header, msgType, 0, payload)
}

// =============================================================================
// ENV-GATE TESTS
// =============================================================================

func TestValidate_GateDisabled_ReturnsNil(t *testing.T) {
	orig := validationEnabled
	validationEnabled = false
	defer func() { validationEnabled = orig }()

	msg := &Message{} // completely invalid — would produce many errors if enabled
	if errs := msg.Validate(); errs != nil {
		t.Errorf("Validate() with validationEnabled=false must return nil, got %v", errs)
	}
}

func TestValidateRawMessage_GateDisabled_ReturnsNil(t *testing.T) {
	orig := validationEnabled
	validationEnabled = false
	defer func() { validationEnabled = orig }()

	if errs := ValidateRawMessage(nil); errs != nil {
		t.Errorf("ValidateRawMessage(nil) with validation disabled must return nil, got %v", errs)
	}
}

// =============================================================================
// ValidationErrors — engineer format (Error())
// =============================================================================

func TestValidationErrors_Error_Empty(t *testing.T) {
	var errs ValidationErrors
	if errs.Error() != "" {
		t.Errorf("Error() on empty slice must return empty string")
	}
}

func TestValidationErrors_Error_ContainsErrorPrefix(t *testing.T) {
	errs := ValidationErrors{
		{Severity: "error", Intent: "StoreEvent", Field: "Event.Location", WireField: "loc",
			Rule: "required", Message: "Location required.", Fix: "Set Location.", ExampleCode: `msg.Event.Location = "TERRA"`},
	}
	out := errs.Error()
	if !strings.Contains(out, "[ERROR]") {
		t.Errorf("Error() must contain [ERROR] prefix, got: %s", out)
	}
	if !strings.Contains(out, "What:") {
		t.Errorf("Error() must contain 'What:' line, got: %s", out)
	}
	if !strings.Contains(out, "Fix:") {
		t.Errorf("Error() must contain 'Fix:' line, got: %s", out)
	}
	if !strings.Contains(out, "Code:") {
		t.Errorf("Error() must contain 'Code:' line, got: %s", out)
	}
}

func TestValidationErrors_Error_ContainsWarnPrefix(t *testing.T) {
	errs := ValidationErrors{
		{Severity: "warn", Intent: "StoreEvent", Rule: "required", Message: "Timestamp missing."},
	}
	out := errs.Error()
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("Error() must contain [WARN] prefix for warn errors, got: %s", out)
	}
}

func TestValidationErrors_Error_OmitsCodeWhenEmpty(t *testing.T) {
	errs := ValidationErrors{
		{Severity: "error", Intent: "X", Rule: "required", Message: "msg", Fix: "fix", ExampleCode: ""},
	}
	if strings.Contains(errs.Error(), "Code:") {
		t.Errorf("Error() must not emit 'Code:' when ExampleCode is empty")
	}
}

func TestValidationErrors_Error_MultipleErrors(t *testing.T) {
	errs := ValidationErrors{
		{Severity: "error", Intent: "A", Field: "f1", Rule: "required", Message: "m1"},
		{Severity: "warn", Intent: "B", Field: "f2", Rule: "format", Message: "m2"},
	}
	out := errs.Error()
	if !strings.Contains(out, "[ERROR]") || !strings.Contains(out, "[WARN]") {
		t.Errorf("Error() must contain both severity prefixes: %s", out)
	}
}

// =============================================================================
// ValidationErrors — LLM JSON format (LLMJson())
// =============================================================================

func TestValidationErrors_LLMJson_Empty(t *testing.T) {
	var errs ValidationErrors
	out := errs.LLMJson()
	if out != "[]" {
		t.Errorf("LLMJson() on empty slice must be '[]', got %q", out)
	}
}

func TestValidationErrors_LLMJson_ValidJSON(t *testing.T) {
	errs := ValidationErrors{
		{Severity: "error", Intent: "LinkEvent", Field: "NeuralMemory.Link.Category",
			WireField: "category", Rule: "required",
			Message: "Category required.", Fix: "Set category.",
			ExampleCode: `msg.NeuralMemory.Link.Category = "related"`,
			References:  []string{"message/types.go:LinkFields.Category", "message/header.go:LinkEventsMessageHeader"}},
	}
	out := errs.LLMJson()
	var parsed []llmValidationError
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("LLMJson() produced invalid JSON: %v\nOutput: %s", err, out)
	}
	if len(parsed) != 1 {
		t.Fatalf("LLMJson() expected 1 item, got %d", len(parsed))
	}
	item := parsed[0]
	checks := map[string]string{
		"severity":    item.Severity,
		"intent":      item.Intent,
		"struct_path": item.StructPath,
		"wire_field":  item.WireField,
		"rule":        item.Rule,
	}
	want := map[string]string{
		"severity": "error", "intent": "LinkEvent",
		"struct_path": "NeuralMemory.Link.Category", "wire_field": "category", "rule": "required",
	}
	for k, got := range checks {
		if got != want[k] {
			t.Errorf("LLMJson item.%s = %q, want %q", k, got, want[k])
		}
	}
	if len(item.References) != 2 {
		t.Errorf("LLMJson item.references length = %d, want 2", len(item.References))
	}
}

func TestValidationErrors_LLMJson_ReferencesEmptyArray(t *testing.T) {
	errs := ValidationErrors{{Severity: "error", Intent: "X", Rule: "required"}}
	out := errs.LLMJson()
	var parsed []llmValidationError
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed[0].References == nil {
		t.Errorf("LLMJson references must be [] not null when nil input")
	}
}

// =============================================================================
// ENVELOPE VALIDATOR
// =============================================================================

func TestValidate_Envelope(t *testing.T) {
	defer withValidation(t)()

	tests := []struct {
		name      string
		msg       *Message
		wantRule  string
		wantField string
		wantOK    bool
	}{
		{
			name: "valid envelope",
			msg: &Message{
				Envelope: Envelope{
					To: "mem@zeroth.pod-os.com", From: "test@zeroth.pod-os.com",
					Intent: IntentType.StoreEvent,
				},
				Event: &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantOK: true,
		},
		{
			name: "missing To",
			msg: &Message{
				Envelope: Envelope{From: "test@zeroth.pod-os.com", Intent: IntentType.StoreEvent},
				Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantRule: "required", wantField: "Envelope.To",
		},
		{
			name: "To without @",
			msg: &Message{
				Envelope: Envelope{To: "noatsign", From: "test@zeroth.pod-os.com", Intent: IntentType.StoreEvent},
				Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantRule: "format", wantField: "Envelope.To",
		},
		{
			name: "To starts with @",
			msg: &Message{
				Envelope: Envelope{To: "@gateway", From: "test@zeroth.pod-os.com", Intent: IntentType.StoreEvent},
				Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantRule: "format", wantField: "Envelope.To",
		},
		{
			name: "missing From",
			msg: &Message{
				Envelope: Envelope{To: "mem@zeroth.pod-os.com", Intent: IntentType.StoreEvent},
				Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantRule: "required", wantField: "Envelope.From",
		},
		{
			name: "From without @",
			msg: &Message{
				Envelope: Envelope{To: "mem@zeroth.pod-os.com", From: "bad", Intent: IntentType.StoreEvent},
				Event:    &EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"},
			},
			wantRule: "format", wantField: "Envelope.From",
		},
		{
			name: "missing Intent",
			msg: &Message{
				Envelope: Envelope{To: "mem@zeroth.pod-os.com", From: "test@zeroth.pod-os.com"},
			},
			wantRule: "required", wantField: "Envelope.Intent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			assertRule(t, errs, tt.wantRule, tt.name)
			if tt.wantField != "" {
				assertField(t, errs, tt.wantField, tt.name)
			}
		})
	}
}

// =============================================================================
// STORE EVENT
// =============================================================================

func TestValidate_StoreEvent(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.StoreEvent) }

	tests := []struct {
		name      string
		msg       *Message
		wantRule  string
		wantField string
		wantOK    bool
	}{
		{
			name: "valid/owner+location",
			msg: &Message{Envelope: base(),
				Event: &EventFields{Owner: "$sys", Location: "TERRA|47.6|-122.5", LocationSeparator: "|"}},
			wantOK: true,
		},
		{
			name: "valid/ownerUniqueID path",
			msg: &Message{Envelope: base(),
				Event: &EventFields{OwnerUniqueID: "owner-uid", Location: "TERRA", LocationSeparator: "|"}},
			wantOK: true,
		},
		{
			name: "nil Event", msg: &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name: "missing owner and ownerUniqueID",
			msg: &Message{Envelope: base(),
				Event: &EventFields{Location: "TERRA", LocationSeparator: "|"}},
			wantRule: "one_of_required",
		},
		{
			name: "missing Location",
			msg: &Message{Envelope: base(),
				Event: &EventFields{Owner: "$sys", LocationSeparator: "|"}},
			wantRule: "required", wantField: "Event.Location",
		},
		{
			name: "missing LocationSeparator",
			msg: &Message{Envelope: base(),
				Event: &EventFields{Owner: "$sys", Location: "TERRA"}},
			wantRule: "required", wantField: "Event.LocationSeparator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			assertRule(t, errs, tt.wantRule, tt.name)
			if tt.wantField != "" {
				assertField(t, errs, tt.wantField, tt.name)
			}
		})
	}
}

// =============================================================================
// STORE BATCH EVENTS
// =============================================================================

func TestValidate_StoreBatchEvents(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.StoreBatchEvents) }

	validSpec := BatchEventSpec{
		Event: EventFields{
			Owner: "$sys", Timestamp: "+1234567890.123456",
			Location: "TERRA|47.6|-122.5", LocationSeparator: "|",
		},
	}

	tests := []struct {
		name           string
		msg            *Message
		wantRule       string
		wantFieldPfx   string // containsFieldPrefix check
		wantOK         bool
	}{
		{
			name: "valid/single record",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{validSpec}},
			},
			wantOK: true,
		},
		{
			name: "valid/multiple records",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{validSpec, validSpec}},
			},
			wantOK: true,
		},
		{
			name: "nil NeuralMemory",
			msg:  &Message{Envelope: base()},
			wantRule: "required", wantFieldPfx: "NeuralMemory.BatchEvents",
		},
		{
			name: "empty BatchEvents",
			msg:  &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "required", wantFieldPfx: "NeuralMemory.BatchEvents",
		},
		{
			name: "record[0] missing Timestamp",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{
					{Event: EventFields{Owner: "$sys", Location: "TERRA", LocationSeparator: "|"}},
				}},
			},
			wantRule: "required", wantFieldPfx: "NeuralMemory.BatchEvents[0].Event.Timestamp",
		},
		{
			name: "record[0] missing Owner",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{
					{Event: EventFields{Timestamp: "+123.0", Location: "TERRA", LocationSeparator: "|"}},
				}},
			},
			wantRule: "one_of_required",
		},
		{
			name: "record[0] missing Location",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{
					{Event: EventFields{Owner: "$sys", Timestamp: "+123.0", LocationSeparator: "|"}},
				}},
			},
			wantFieldPfx: "NeuralMemory.BatchEvents[0].Event.Location",
		},
		{
			name: "record[0] missing LocationSeparator",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{
					{Event: EventFields{Owner: "$sys", Timestamp: "+123.0", Location: "TERRA"}},
				}},
			},
			wantFieldPfx: "NeuralMemory.BatchEvents[0].Event.LocationSeparator",
		},
		{
			name: "record[1] error (record[0] valid)",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchEvents: []BatchEventSpec{
					validSpec,
					{Event: EventFields{Owner: "$sys", Timestamp: "+123.0", Location: "TERRA"}},
				}},
			},
			wantFieldPfx: "NeuralMemory.BatchEvents[1].Event.LocationSeparator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			if tt.wantRule != "" {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
			if tt.wantFieldPfx != "" && !containsFieldPrefix(errs, tt.wantFieldPfx) {
				t.Errorf("%s: expected field prefix %q, got:\n%s", tt.name, tt.wantFieldPfx, errs.Error())
			}
		})
	}
}

// =============================================================================
// STORE BATCH TAGS
// =============================================================================

func TestValidate_StoreBatchTags(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.StoreBatchTags) }

	validEvent := &EventFields{Id: "event-id", Owner: "$sys"}
	validTags := TagList{{Key: "category", Value: "value1"}}

	tests := []struct {
		name        string
		msg         *Message
		wantRule    string
		wantField   string
		wantOK      bool
	}{
		{
			name:   "valid/id+owner+tags",
			msg:    &Message{Envelope: base(), Event: validEvent, NeuralMemory: &NeuralMemoryFields{Tags: validTags}},
			wantOK: true,
		},
		{
			name: "valid/uniqueId path",
			msg: &Message{
				Envelope:     base(),
				Event:        &EventFields{UniqueId: "my-uid", Owner: "$sys"},
				NeuralMemory: &NeuralMemoryFields{Tags: validTags},
			},
			wantOK: true,
		},
		{
			name:     "nil Event",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{Tags: validTags}},
			wantRule: "nil_struct",
		},
		{
			name: "missing Event.Id AND Event.UniqueId",
			msg: &Message{
				Envelope:     base(),
				Event:        &EventFields{Owner: "$sys"},
				NeuralMemory: &NeuralMemoryFields{Tags: validTags},
			},
			wantRule: "one_of_required",
		},
		{
			name: "missing Event.Owner AND Event.OwnerUniqueID",
			msg: &Message{
				Envelope:     base(),
				Event:        &EventFields{Id: "eid"},
				NeuralMemory: &NeuralMemoryFields{Tags: validTags},
			},
			wantRule: "one_of_required",
		},
		{
			name:     "nil NeuralMemory.Tags",
			msg:      &Message{Envelope: base(), Event: validEvent, NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "required",
		},
		{
			name: "tag missing Key",
			msg: &Message{
				Envelope:     base(),
				Event:        validEvent,
				NeuralMemory: &NeuralMemoryFields{Tags: TagList{{Key: "", Value: "v"}}},
			},
			wantRule: "payload_format",
		},
		{
			name: "tag nil Value",
			msg: &Message{
				Envelope:     base(),
				Event:        validEvent,
				NeuralMemory: &NeuralMemoryFields{Tags: TagList{{Key: "k", Value: nil}}},
			},
			wantRule: "payload_format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			assertRule(t, errs, tt.wantRule, tt.name)
			if tt.wantField != "" {
				assertField(t, errs, tt.wantField, tt.name)
			}
		})
	}
}

// =============================================================================
// GET EVENT
// =============================================================================

func TestValidate_GetEvent(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.GetEvent) }

	tests := []struct {
		name     string
		msg      *Message
		wantRule string
		wantOK   bool
	}{
		{
			name:   "valid/id",
			msg:    &Message{Envelope: base(), Event: &EventFields{Id: "2024.01.15..."}},
			wantOK: true,
		},
		{
			name:   "valid/uniqueId",
			msg:    &Message{Envelope: base(), Event: &EventFields{UniqueId: "my-uid"}},
			wantOK: true,
		},
		{
			name:     "nil Event",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "missing Id and UniqueId",
			msg:      &Message{Envelope: base(), Event: &EventFields{}},
			wantRule: "one_of_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
			} else {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
		})
	}
}

// =============================================================================
// GET EVENTS FOR TAGS
// =============================================================================

func TestValidate_GetEventsForTags(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.GetEventsForTags) }

	tests := []struct {
		name     string
		msg      *Message
		wantRule string
		wantOK   bool
	}{
		{
			name: "valid/full options",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{
					GetEventsForTags: &GetEventsForTagsOptions{
						EventPattern: "key=value", BufferResults: true,
					},
				},
			},
			wantOK: true,
		},
		{
			name: "valid/empty options struct",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{GetEventsForTags: &GetEventsForTagsOptions{}},
			},
			wantOK: true,
		},
		{
			name:     "nil NeuralMemory",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "nil GetEventsForTags",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "nil_struct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
			} else {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
		})
	}
}

// =============================================================================
// LINK EVENT
// =============================================================================

func TestValidate_LinkEvent(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.LinkEvent) }

	validLink := &LinkFields{
		EventA: "a", EventB: "b",
		Category: "related", StrengthA: 1.0, StrengthB: 1.0,
		Timestamp: "+1234567890.123456", OwnerID: "owner-event-id",
		Location: "TERRA|47.6|-122.5", LocationSeparator: "|",
	}

	tests := []struct {
		name      string
		msg       *Message
		wantRule  string
		wantField string
		wantOK    bool
	}{
		{
			name: "valid/eventA+B",
			msg: &Message{
				Envelope:     base(),
				Event:        &EventFields{Owner: "$sys"},
				NeuralMemory: &NeuralMemoryFields{Link: validLink},
			},
			wantOK: true,
		},
		{
			name: "valid/uniqueIdA+B",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{
					UniqueIdA: "ua", UniqueIdB: "ub",
					Category: "related", StrengthA: 1.0, StrengthB: 1.0,
					Timestamp: "+123.0", OwnerID: "oid",
					Location: "TERRA", LocationSeparator: "|",
				}},
			},
			wantOK: true,
		},
		{
			name:     "nil NeuralMemory",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "nil Link",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "nil_struct",
		},
		{
			name: "missing event pair (only A set)",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "one_of_required",
		},
		{
			name: "uniqueId pair incomplete (only A set)",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{UniqueIdA: "ua", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "one_of_required",
		},
		{
			name: "missing Category",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.Category",
		},
		{
			name: "missing StrengthA",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthB: 1, Timestamp: "+1", OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.StrengthA",
		},
		{
			name: "missing StrengthB",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, Timestamp: "+1", OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.StrengthB",
		},
		{
			name: "missing Timestamp",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1, OwnerID: "o", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.Timestamp",
		},
		{
			name: "missing OwnerID and OwnerUniqueID",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", Location: "L", LocationSeparator: "|"}},
			},
			wantRule: "one_of_required",
		},
		{
			name: "valid/ownerUniqueID path",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerUniqueID: "ou", Location: "L", LocationSeparator: "|"}},
			},
			wantOK: true,
		},
		{
			name: "missing Location",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerID: "o", LocationSeparator: "|"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.Location",
		},
		{
			name: "missing LocationSeparator",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1, Timestamp: "+1", OwnerID: "o", Location: "L"}},
			},
			wantRule: "required", wantField: "NeuralMemory.Link.LocationSeparator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			assertRule(t, errs, tt.wantRule, tt.name)
			if tt.wantField != "" {
				assertField(t, errs, tt.wantField, tt.name)
			}
		})
	}
}

// =============================================================================
// UNLINK EVENT
// =============================================================================

func TestValidate_UnlinkEvent(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.UnlinkEvent) }

	tests := []struct {
		name     string
		msg      *Message
		wantRule string
		wantOK   bool
	}{
		{
			name:   "valid/id",
			msg:    &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{Id: "link-id"}}},
			wantOK: true,
		},
		{
			name:   "valid/uniqueId",
			msg:    &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{UniqueId: "link-uid"}}},
			wantOK: true,
		},
		{
			name:     "nil NeuralMemory",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "nil Link",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "nil_struct",
		},
		{
			name:     "missing Id and UniqueId",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{}}},
			wantRule: "one_of_required",
		},
		{
			name: "Location set without LocationSeparator",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{Id: "lid", Location: "TERRA"}},
			},
			wantRule: "required",
		},
		{
			name: "Location with LocationSeparator — valid",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{Id: "lid", Location: "TERRA", LocationSeparator: "|"}},
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
			} else {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
		})
	}
}

// =============================================================================
// STORE BATCH LINKS
// =============================================================================

func TestValidate_StoreBatchLinks(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope { return envelopeFor(IntentType.StoreBatchLinks) }

	validSpec := BatchLinkEventSpec{
		Event: EventFields{Owner: "$sys", Timestamp: "+123.0"},
		Link: LinkFields{
			EventA: "a", EventB: "b",
			Category: "related", StrengthA: 1.0, StrengthB: 1.0,
			Timestamp: "+123.0", OwnerID: "owner-id",
		},
	}

	tests := []struct {
		name         string
		msg          *Message
		wantRule     string
		wantFieldPfx string
		wantOK       bool
	}{
		{
			name: "valid/single record",
			msg: &Message{
				Envelope:     base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{validSpec}},
			},
			wantOK: true,
		},
		{
			name:     "nil NeuralMemory",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "empty BatchLinks",
			msg:      &Message{Envelope: base(), NeuralMemory: &NeuralMemoryFields{}},
			wantRule: "required",
		},
		{
			name: "record[0] missing Event.Timestamp",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: EventFields{Owner: "$sys"},
					Link:  validSpec.Link,
				}}},
			},
			wantFieldPfx: "NeuralMemory.BatchLinks[0].Event.Timestamp",
		},
		{
			name: "record[0] missing Event.Owner",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: EventFields{Timestamp: "+1"},
					Link:  validSpec.Link,
				}}},
			},
			wantRule: "payload_format",
		},
		{
			name: "record[0] missing Link.Timestamp (NOT auto-generated)",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: validSpec.Event,
					Link: LinkFields{
						EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1,
						OwnerID: "o",
						// Timestamp intentionally empty
					},
				}}},
			},
			wantFieldPfx: "NeuralMemory.BatchLinks[0].Link.Timestamp",
		},
		{
			name: "record[0] missing Link event pair",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: validSpec.Event,
					Link: LinkFields{
						Category: "c", StrengthA: 1, StrengthB: 1,
						Timestamp: "+1", OwnerID: "o",
					},
				}}},
			},
			wantRule: "payload_format",
		},
		{
			name: "record[0] missing Link.Category",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: validSpec.Event,
					Link: LinkFields{
						EventA: "a", EventB: "b", StrengthA: 1, StrengthB: 1,
						Timestamp: "+1", OwnerID: "o",
					},
				}}},
			},
			wantFieldPfx: "NeuralMemory.BatchLinks[0].Link.Category",
		},
		{
			name: "record[0] missing Link.StrengthA",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: validSpec.Event,
					Link: LinkFields{
						EventA: "a", EventB: "b", Category: "c", StrengthB: 1,
						Timestamp: "+1", OwnerID: "o",
					},
				}}},
			},
			wantFieldPfx: "NeuralMemory.BatchLinks[0].Link.StrengthA",
		},
		{
			name: "record[0] missing Link.OwnerID",
			msg: &Message{
				Envelope: base(),
				NeuralMemory: &NeuralMemoryFields{BatchLinks: []BatchLinkEventSpec{{
					Event: validSpec.Event,
					Link: LinkFields{
						EventA: "a", EventB: "b", Category: "c", StrengthA: 1, StrengthB: 1,
						Timestamp: "+1",
					},
				}}},
			},
			wantRule: "payload_format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
				return
			}
			if tt.wantRule != "" {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
			if tt.wantFieldPfx != "" && !containsFieldPrefix(errs, tt.wantFieldPfx) {
				t.Errorf("%s: expected field prefix %q, got:\n%s", tt.name, tt.wantFieldPfx, errs.Error())
			}
		})
	}
}

// =============================================================================
// GATEWAY / ACTOR INTENTS
// =============================================================================

func TestValidate_GatewayId(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope {
		return Envelope{
			To:     "gateway@zeroth.pod-os.com",
			From:   "test@zeroth.pod-os.com",
			Intent: IntentType.GatewayId,
		}
	}

	tests := []struct {
		name     string
		msg      *Message
		wantRule string
		wantOK   bool
	}{
		{
			name:   "valid/name only",
			msg:    &Message{Envelope: func() Envelope { e := base(); e.ClientName = "TestClient"; return e }()},
			wantOK: true,
		},
		{
			name: "valid/with credentials",
			msg: &Message{Envelope: func() Envelope {
				e := base()
				e.ClientName = "TestClient"
				e.UserName = "admin"
				e.Passcode = "secret"
				return e
			}()},
			wantOK: true,
		},
		{
			name:     "missing ClientName",
			msg:      &Message{Envelope: base()},
			wantRule: "required",
		},
		{
			name: "UserName without Passcode",
			msg: &Message{Envelope: func() Envelope {
				e := base()
				e.ClientName = "TC"
				e.UserName = "admin"
				return e
			}()},
			wantRule: "required",
		},
		{
			name: "Passcode without UserName",
			msg: &Message{Envelope: func() Envelope {
				e := base()
				e.ClientName = "TC"
				e.Passcode = "secret"
				return e
			}()},
			wantRule: "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
			} else {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
		})
	}
}

func TestValidate_GatewayStream(t *testing.T) {
	defer withValidation(t)()

	for _, intent := range []Intent{IntentType.GatewayStreamOn, IntentType.GatewayStreamOff} {
		t.Run(intent.Name, func(t *testing.T) {
			msg := &Message{
				Envelope: Envelope{
					To:     "gateway@zeroth.pod-os.com",
					From:   "test@zeroth.pod-os.com",
					Intent: intent,
				},
			}
			assertNoErrors(t, msg.Validate(), intent.Name)
		})
	}
}

func TestValidate_ActorRequest_Valid(t *testing.T) {
	defer withValidation(t)()
	msg := &Message{
		Envelope: Envelope{
			To:     "actor@zeroth.pod-os.com",
			From:   "test@zeroth.pod-os.com",
			Intent: IntentType.ActorRequest,
		},
	}
	assertNoErrors(t, msg.Validate(), "ActorRequest")
}

func TestValidate_ActorReport(t *testing.T) {
	defer withValidation(t)()

	base := func() Envelope {
		return Envelope{To: "a@g", From: "b@g", Intent: IntentType.ActorReport}
	}

	tests := []struct {
		name     string
		msg      *Message
		wantRule string
		wantOK   bool
	}{
		{
			name:   "valid",
			msg:    &Message{Envelope: base(), Response: &ResponseFields{Status: "OK", Message: "healthy"}},
			wantOK: true,
		},
		{
			name:     "nil Response",
			msg:      &Message{Envelope: base()},
			wantRule: "nil_struct",
		},
		{
			name:     "missing Status",
			msg:      &Message{Envelope: base(), Response: &ResponseFields{Message: "msg"}},
			wantRule: "required",
		},
		{
			name:     "missing Message",
			msg:      &Message{Envelope: base(), Response: &ResponseFields{Status: "OK"}},
			wantRule: "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantOK {
				assertNoErrors(t, errs, tt.name)
			} else {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
		})
	}
}

func TestValidate_Status_Valid(t *testing.T) {
	defer withValidation(t)()
	msg := &Message{
		Envelope: Envelope{To: "a@g", From: "b@g", Intent: IntentType.Status},
	}
	assertNoErrors(t, msg.Validate(), "Status")
}

func TestValidate_StatusRequest_Valid(t *testing.T) {
	defer withValidation(t)()
	msg := &Message{
		Envelope: Envelope{To: "a@g", From: "b@g", Intent: IntentType.StatusRequest, MessageId: "probe-1"},
	}
	assertNoErrors(t, msg.Validate(), "StatusRequest")
}

func TestValidate_StatusRequest_MissingTo(t *testing.T) {
	defer withValidation(t)()
	msg := &Message{
		Envelope: Envelope{From: "b@g", Intent: IntentType.StatusRequest, MessageId: "probe-1"},
	}
	errs := msg.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation errors for missing To")
	}
}

// =============================================================================
// RESPONSE INTENT VALIDATORS
// =============================================================================

func TestValidate_ResponseIntents(t *testing.T) {
	defer withValidation(t)()

	responseIntents := []Intent{
		IntentType.StoreEventResponse,
		IntentType.StoreBatchEventsResponse,
		IntentType.StoreBatchTagsResponse,
		IntentType.GetEventResponse,
		IntentType.GetEventsForTagsResponse,
		IntentType.LinkEventResponse,
		IntentType.UnlinkEventResponse,
		IntentType.StoreBatchLinksResponse,
	}

	for _, intent := range responseIntents {
		t.Run(intent.Name+"/nil Response", func(t *testing.T) {
			msg := &Message{
				Envelope: Envelope{To: "c@g", From: "m@g", Intent: intent},
			}
			assertWarn(t, msg.Validate(), intent.Name+" nil Response")
		})

		t.Run(intent.Name+"/empty Status", func(t *testing.T) {
			msg := &Message{
				Envelope: Envelope{To: "c@g", From: "m@g", Intent: intent},
				Response: &ResponseFields{Status: ""},
			}
			assertWarn(t, msg.Validate(), intent.Name+" empty Status")
		})

		t.Run(intent.Name+"/valid", func(t *testing.T) {
			msg := &Message{
				Envelope: Envelope{To: "c@g", From: "m@g", Intent: intent},
				Response: &ResponseFields{Status: "OK"},
			}
			assertNoErrors(t, msg.Validate(), intent.Name+" valid")
		})
	}
}

// =============================================================================
// WIRE VALIDATOR — Stage 1 (framing)
// =============================================================================

func TestValidateRawMessage_Stage1(t *testing.T) {
	defer withValidation(t)()

	tests := []struct {
		name     string
		raw      []byte
		wantRule string
	}{
		{
			name:     "nil input",
			raw:      nil,
			wantRule: "nil_struct",
		},
		{
			name:     "too short (<63 bytes)",
			raw:      []byte("tooshort"),
			wantRule: "format",
		},
		{
			name:     "bad To format (no @)",
			raw:      rawMsg("noatsign", "test@zeroth.pod-os.com", "_db_cmd=store\ttimestamp=+1", 1000, ""),
			wantRule: "format",
		},
		{
			name:     "bad To format (@ at start)",
			raw:      rawMsg("@gateway", "test@zeroth.pod-os.com", "_db_cmd=store\ttimestamp=+1", 1000, ""),
			wantRule: "format",
		},
		{
			name:     "bad From format",
			raw:      rawMsg("mem@zeroth.pod-os.com", "badformat", "_db_cmd=store\ttimestamp=+1", 1000, ""),
			wantRule: "format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateRawMessage(tt.raw)
			assertRule(t, errs, tt.wantRule, tt.name)
		})
	}
}

// =============================================================================
// WIRE VALIDATOR — Stage 2: NeuralMemory requests (messageType 1000)
// =============================================================================

func TestValidateRawMessage_NeuralMemoryRequests(t *testing.T) {
	defer withValidation(t)()

	const to = "mem@zeroth.pod-os.com"
	const from = "test@zeroth.pod-os.com"

	tests := []struct {
		name      string
		header    string
		payload   string
		msgType   int
		wantOK    bool
		wantRule  string
		wantField string
	}{
		// ---- missing _db_cmd ----
		{
			name:     "missing _db_cmd",
			header:   "timestamp=+1", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- store ----
		{
			name:   "store/valid",
			header: "_db_cmd=store\ttimestamp=+1234567890.123456",
			msgType: 1000, wantOK: true,
		},
		{
			name:     "store/missing timestamp",
			header:   "_db_cmd=store", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- store_batch ----
		{
			name:   "store_batch/valid with payload",
			header: "_db_cmd=store_batch", msgType: 1000,
			payload: "timestamp=+1\towner=sys\tloc=T\tloc_delim=|",
			wantOK: true,
		},
		{
			name:     "store_batch/empty payload",
			header:   "_db_cmd=store_batch", msgType: 1000,
			wantRule: "required",
		},
		// ---- tag_store_batch ----
		{
			name:   "tag_store_batch/valid with event_id",
			header: "_db_cmd=tag_store_batch\tevent_id=eid\towner=sys", msgType: 1000, wantOK: true,
		},
		{
			name:   "tag_store_batch/valid with unique_id",
			header: "_db_cmd=tag_store_batch\tunique_id=uid\towner=sys", msgType: 1000, wantOK: true,
		},
		{
			name:   "tag_store_batch/valid with owner_unique_id",
			header: "_db_cmd=tag_store_batch\tevent_id=eid\towner_unique_id=ouid", msgType: 1000, wantOK: true,
		},
		{
			name:     "tag_store_batch/missing event id",
			header:   "_db_cmd=tag_store_batch\towner=sys", msgType: 1000,
			wantRule: "header_missing",
		},
		{
			name:     "tag_store_batch/missing owner",
			header:   "_db_cmd=tag_store_batch\tevent_id=eid", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- get ----
		{
			name:   "get/valid with event_id",
			header: "_db_cmd=get\tevent_id=eid", msgType: 1000, wantOK: true,
		},
		{
			name:   "get/valid with unique_id",
			header: "_db_cmd=get\tunique_id=uid", msgType: 1000, wantOK: true,
		},
		{
			name:     "get/missing id",
			header:   "_db_cmd=get", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- events_for_tag ----
		{
			name:   "events_for_tag/valid",
			header: "_db_cmd=events_for_tag\tbuffer_results=Y", msgType: 1000, wantOK: true,
		},
		{
			name:   "events_for_tag/missing buffer_results → warn only",
			header: "_db_cmd=events_for_tag", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- link ----
		{
			name:   "link/valid with event ids",
			header: "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tcategory=c\tstrength_a=1\tstrength_b=1\ttimestamp=+1\towner_event_id=o",
			msgType: 1000, wantOK: true,
		},
		{
			name:   "link/valid with unique ids",
			header: "_db_cmd=link\tunique_id_a=ua\tunique_id_b=ub\tcategory=c\tstrength_a=1\tstrength_b=1\ttimestamp=+1\towner_event_id=o",
			msgType: 1000, wantOK: true,
		},
		{
			name:     "link/missing strength_a",
			header:   "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tcategory=c\tstrength_b=1\ttimestamp=+1\towner_event_id=o",
			msgType:  1000,
			wantField: "NeuralMemory.Link.StrengthA",
		},
		{
			name:     "link/missing strength_b",
			header:   "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tcategory=c\tstrength_a=1\ttimestamp=+1\towner_event_id=o",
			msgType:  1000,
			wantField: "NeuralMemory.Link.StrengthB",
		},
		{
			name:     "link/missing category",
			header:   "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tstrength_a=1\tstrength_b=1\ttimestamp=+1\towner_event_id=o",
			msgType:  1000,
			wantField: "NeuralMemory.Link.Category",
		},
		{
			name:     "link/missing timestamp",
			header:   "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tcategory=c\tstrength_a=1\tstrength_b=1\towner_event_id=o",
			msgType:  1000,
			wantField: "NeuralMemory.Link.Timestamp",
		},
		{
			name:     "link/missing owner",
			header:   "_db_cmd=link\tevent_id_a=a\tevent_id_b=b\tcategory=c\tstrength_a=1\tstrength_b=1\ttimestamp=+1",
			msgType:  1000,
			wantRule: "header_missing",
		},
		{
			name:     "link/missing event pair",
			header:   "_db_cmd=link\tcategory=c\tstrength_a=1\tstrength_b=1\ttimestamp=+1\towner_event_id=o",
			msgType:  1000,
			wantRule: "header_missing",
		},
		// ---- unlink ----
		{
			name:   "unlink/valid with event_id",
			header: "_db_cmd=unlink\tevent_id=link-id", msgType: 1000, wantOK: true,
		},
		{
			name:   "unlink/valid with unique_id",
			header: "_db_cmd=unlink\tunique_id=link-uid", msgType: 1000, wantOK: true,
		},
		{
			name:     "unlink/missing id",
			header:   "_db_cmd=unlink", msgType: 1000,
			wantRule: "header_missing",
		},
		// ---- link_batch ----
		{
			name:   "link_batch/valid with payload",
			header: "_db_cmd=link_batch", msgType: 1000,
			payload: "event_id_a=a\tevent_id_b=b\tcategory=c",
			wantOK: true,
		},
		{
			name:     "link_batch/empty payload",
			header:   "_db_cmd=link_batch", msgType: 1000,
			wantRule: "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawMsg(to, from, tt.header, tt.msgType, tt.payload)
			errs := ValidateRawMessage(raw)
			if tt.wantOK {
				if hasErrors(errs) {
					t.Errorf("expected no errors, got:\n%s", errs.Error())
				}
				return
			}
			if tt.wantRule != "" {
				assertRule(t, errs, tt.wantRule, tt.name)
			}
			if tt.wantField != "" {
				assertField(t, errs, tt.wantField, tt.name)
			}
		})
	}
}

// =============================================================================
// WIRE VALIDATOR — Stage 2: NeuralMemory responses (messageType 1001)
// =============================================================================

func TestValidateRawMessage_NeuralMemoryResponses(t *testing.T) {
	defer withValidation(t)()

	const to = "test@zeroth.pod-os.com"
	const from = "mem@zeroth.pod-os.com"

	tests := []struct {
		name     string
		header   string
		wantWarn bool
		wantOK   bool
	}{
		{
			name:   "store response/valid",
			header: "_type=store\t_status=OK\t_count=1",
			wantOK: true,
		},
		{
			name:     "store response/missing _status (WARN not ERROR)",
			header:   "_type=store\t_count=1",
			wantWarn: true,
		},
		{
			name:   "get response/valid",
			header: "_type=get\t_status=OK\t_event_id=eid",
			wantOK: true,
		},
		{
			name:     "get response/missing _event_id",
			header:   "_type=get\t_status=OK",
			wantWarn: true,
		},
		{
			name:   "link response/valid",
			header: "_type=link\t_status=OK\tlink_event=lid",
			wantOK: true,
		},
		{
			name:     "link response/missing link_event",
			header:   "_type=link\t_status=OK",
			wantWarn: true,
		},
		{
			name:   "store_batch response/valid",
			header: "_type=store_batch\t_status=OK\t_count=5",
			wantOK: true,
		},
		{
			name:     "store_batch response/missing _count",
			header:   "_type=store_batch\t_status=OK",
			wantWarn: true,
		},
		{
			name:   "tag_store_batch response/valid",
			header: "_type=tag_store_batch\t_status=OK\t_count=3",
			wantOK: true,
		},
		{
			name:   "link_batch response/valid",
			header: "_type=link_batch\t_status=OK\t_links_ok=2",
			wantOK: true,
		},
		{
			name:     "link_batch response/missing _links_ok",
			header:   "_type=link_batch\t_status=OK",
			wantWarn: true,
		},
		{
			name:   "events_for_tag response/valid",
			header: "_type=events_for_tag\t_status=OK",
			wantOK: true,
		},
		{
			name:   "brief_hit response/no _status (WARN, not ERROR)",
			header: "_type=events_for_tag",
			wantWarn: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawMsg(to, from, tt.header, 1001, "")
			errs := ValidateRawMessage(raw)
			if tt.wantOK {
				if hasErrors(errs) {
					t.Errorf("expected no errors, got:\n%s", errs.Error())
				}
				return
			}
			if tt.wantWarn {
				assertWarn(t, errs, tt.name)
				// Response warnings must NOT be errors
				if hasErrors(errs) {
					t.Errorf("%s: response warnings must not be errors, got:\n%s", tt.name, errs.Error())
				}
			}
		})
	}
}

// =============================================================================
// WIRE VALIDATOR — Stage 2: non-NeuralMemory intents
// =============================================================================

func TestValidateRawMessage_NonNeuralMemoryIntents(t *testing.T) {
	defer withValidation(t)()

	const gw = "gateway@zeroth.pod-os.com"
	const cl = "test@zeroth.pod-os.com"

	tests := []struct {
		name     string
		to       string
		from     string
		header   string
		msgType  int
		wantOK   bool
		wantRule string
	}{
		// GatewayId (5)
		{
			name: "GatewayId/valid",
			to: gw, from: cl,
			header: "id:name=TestClient", msgType: 5, wantOK: true,
		},
		{
			name: "GatewayId/missing id:name",
			to: gw, from: cl,
			header: "", msgType: 5, wantRule: "header_missing",
		},
		// GatewayStreamOn (10)
		{
			name: "GatewayStreamOn/valid empty header",
			to: gw, from: cl,
			header: "", msgType: 10, wantOK: true,
		},
		{
			name: "GatewayStreamOn/valid with msg_id",
			to: gw, from: cl,
			header: "_msg_id=abc-123", msgType: 10, wantOK: true,
		},
		// GatewayStreamOff (9)
		{
			name: "GatewayStreamOff/valid",
			to: gw, from: cl,
			header: "", msgType: 9, wantOK: true,
		},
		// ActorRequest (4)
		{
			name: "ActorRequest/valid",
			to: "actor@zeroth.pod-os.com", from: cl,
			header: "_type=status", msgType: 4, wantOK: true,
		},
		{
			name: "ActorRequest/missing _type=status",
			to: "actor@zeroth.pod-os.com", from: cl,
			header: "", msgType: 4, wantRule: "header_missing",
		},
		{
			name: "ActorRequest/_type wrong value",
			to: "actor@zeroth.pod-os.com", from: cl,
			header: "_type=wrong", msgType: 4, wantRule: "header_missing",
		},
		// Status (3)
		{
			name: "Status/valid empty",
			to: cl, from: "actor@zeroth.pod-os.com",
			header: "", msgType: 3, wantOK: true,
		},
		// ActorResponse (30)
		{
			name: "ActorResponse/valid",
			to: cl, from: "actor@zeroth.pod-os.com",
			header: "_status=OK\t_msg=done", msgType: 30, wantOK: true,
		},
		// ActorReport (19)
		{
			name: "ActorReport/valid",
			to: cl, from: "actor@zeroth.pod-os.com",
			header: "_status=OK\t_msg=healthy", msgType: 19, wantOK: true,
		},
		// ActorEcho (2)
		{
			name: "ActorEcho/with msg_id",
			to: "actor@zeroth.pod-os.com", from: cl,
			header: "_msg_id=echo-123", msgType: 2, wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := rawMsg(tt.to, tt.from, tt.header, tt.msgType, "")
			errs := ValidateRawMessage(raw)
			if tt.wantOK {
				if hasErrors(errs) {
					t.Errorf("expected no errors, got:\n%s", errs.Error())
				}
				return
			}
			assertRule(t, errs, tt.wantRule, tt.name)
		})
	}
}

// =============================================================================
// WIRE VALIDATOR — completely unrecognised messageType produces format ERROR
// =============================================================================

// An unrecognised messageType (e.g. 9999) indicates incorrect message construction
// and produces a "format" error, not merely a warning.
func TestValidateRawMessage_UnrecognisedMsgType_Error(t *testing.T) {
	defer withValidation(t)()
	raw := rawMsg("a@g", "b@g", "", 9999, "")
	errs := ValidateRawMessage(raw)
	assertRule(t, errs, "format", "unrecognised messageType")
	if !hasErrors(errs) {
		t.Errorf("unrecognised messageType should produce an error-severity diagnostic, got:\n%s", errs.Error())
	}
}

// =============================================================================
// WIRE VALIDATOR — optional _msg_id present but empty → WARN
// =============================================================================

func TestValidateRawMessage_EmptyMsgId_Warn(t *testing.T) {
	defer withValidation(t)()
	raw := rawMsg(
		"mem@zeroth.pod-os.com", "test@zeroth.pod-os.com",
		"_db_cmd=store\ttimestamp=+1\t_msg_id=",
		1000, "",
	)
	errs := ValidateRawMessage(raw)
	if !containsWarn(errs) {
		t.Errorf("empty _msg_id should produce WARN, got:\n%s", errs.Error())
	}
}

// =============================================================================
// FULL ROUND-TRIP: Validate() → EncodeMessage() → ValidateRawMessage()
// =============================================================================

func TestValidate_RoundTrip_StoreEvent(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:        "mem@zeroth.pod-os.com",
			From:      "test@zeroth.pod-os.com",
			Intent:    IntentType.StoreEvent,
			MessageId: "round-trip-001",
		},
		Event: &EventFields{
			Owner:             "$sys",
			Location:          "TERRA|47.6|-122.5",
			LocationSeparator: "|",
			Type:              "test_event",
		},
	}

	// Step 1: struct validation
	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors before encode: %s", errs.Error())
	}

	// Step 2: encode
	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	// Step 3: wire validation
	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded StoreEvent produced errors:\n%s", errs.Error())
	}
}

func TestValidate_RoundTrip_LinkEvent(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:        "mem@zeroth.pod-os.com",
			From:      "test@zeroth.pod-os.com",
			Intent:    IntentType.LinkEvent,
			MessageId: "round-trip-002",
		},
		Event: &EventFields{Owner: "$sys"},
		NeuralMemory: &NeuralMemoryFields{
			Link: &LinkFields{
				EventA: "event-a-001", EventB: "event-b-002",
				Category:          "related",
				StrengthA:         0.8,
				StrengthB:         0.5,
				Timestamp:         "+1234567890.123456",
				OwnerID:           "owner-event-id",
				Location:          "TERRA|47.6|-122.5",
				LocationSeparator: "|",
			},
		},
	}

	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors: %s", errs.Error())
	}

	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded LinkEvent produced errors:\n%s", errs.Error())
	}
}

func TestValidate_RoundTrip_GetEvent(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:     "mem@zeroth.pod-os.com",
			From:   "test@zeroth.pod-os.com",
			Intent: IntentType.GetEvent,
		},
		Event: &EventFields{Id: "2024.01.15.14.30.45.123456@actor1|location1|segment1"},
	}

	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors: %s", errs.Error())
	}

	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded GetEvent produced errors:\n%s", errs.Error())
	}
}

func TestValidate_RoundTrip_GetEventsForTags(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:     "mem@zeroth.pod-os.com",
			From:   "test@zeroth.pod-os.com",
			Intent: IntentType.GetEventsForTags,
		},
		NeuralMemory: &NeuralMemoryFields{
			GetEventsForTags: &GetEventsForTagsOptions{
				EventPattern:  "category=machine_learning",
				BufferResults: true,
			},
		},
	}

	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors: %s", errs.Error())
	}

	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded GetEventsForTags produced errors:\n%s", errs.Error())
	}
}

func TestValidate_RoundTrip_GatewayId(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:         "zeroth.pod-os.com@zeroth.pod-os.com",
			From:       "TestClient@zeroth.pod-os.com",
			Intent:     IntentType.GatewayId,
			ClientName: "TestClient",
		},
	}

	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors: %s", errs.Error())
	}

	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded GatewayId produced errors:\n%s", errs.Error())
	}
}

func TestValidate_RoundTrip_UnlinkEvent(t *testing.T) {
	defer withValidation(t)()

	msg := &Message{
		Envelope: Envelope{
			To:     "mem@zeroth.pod-os.com",
			From:   "test@zeroth.pod-os.com",
			Intent: IntentType.UnlinkEvent,
		},
		NeuralMemory: &NeuralMemoryFields{
			Link: &LinkFields{Id: "2024.01.15.14.30.45.123456@actor1|location1|segment1"},
		},
	}

	if errs := msg.Validate(); hasErrors(errs) {
		t.Fatalf("Validate() produced errors: %s", errs.Error())
	}

	socket, err := EncodeMessage(msg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() failed: %v", err)
	}

	errs := ValidateRawMessage(socket.MessageBytes)
	if hasErrors(errs) {
		t.Errorf("ValidateRawMessage() on encoded UnlinkEvent produced errors:\n%s", errs.Error())
	}
}

// =============================================================================
// VLLM — prompt rendering and edge-case behaviour
// =============================================================================

func TestRenderErrorPrompt_ContainsAllFields(t *testing.T) {
	ve := ValidationError{
		Severity:    "error",
		Intent:      "LinkEvent",
		Field:       "NeuralMemory.Link.Category",
		WireField:   "category",
		Rule:        "required",
		Message:     "Category is required.",
		Fix:         "Set NeuralMemory.Link.Category.",
		ExampleCode: `msg.NeuralMemory.Link.Category = "related"`,
		References:  []string{"message/types.go:LinkFields.Category", "message/header.go:LinkEventsMessageHeader"},
	}
	prompt := renderErrorPrompt(ve)
	for _, want := range []string{
		"LinkEvent",
		"NeuralMemory.Link.Category",
		"category",
		"required",
		"Category is required.",
		"message/types.go:LinkFields.Category",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("renderErrorPrompt() missing %q in:\n%s", want, prompt)
		}
	}
}

func TestExplainValidationErrors_GateDisabled(t *testing.T) {
	orig := validationEnabled
	validationEnabled = false
	defer func() { validationEnabled = orig }()

	result, err := ExplainValidationErrors(
		ValidationErrors{{Intent: "StoreEvent", Rule: "required"}},
		"http://localhost:8000", "test-model",
	)
	if err != nil {
		t.Errorf("must not error when gate disabled: %v", err)
	}
	if result != "" {
		t.Errorf("must return empty string when gate disabled, got %q", result)
	}
}

func TestExplainValidationErrors_EmptyErrors(t *testing.T) {
	defer withValidation(t)()
	result, err := ExplainValidationErrors(nil, "http://localhost:8000", "model")
	if err != nil || result != "" {
		t.Errorf("empty errors: want ('', nil), got (%q, %v)", result, err)
	}
}

func TestExplainValidationErrors_MissingEndpoint(t *testing.T) {
	defer withValidation(t)()
	_, err := ExplainValidationErrors(
		ValidationErrors{{Intent: "StoreEvent", Rule: "required"}},
		"", "model",
	)
	if err == nil {
		t.Errorf("must return error when endpoint is empty")
	}
}

// =============================================================================
// HELPERS — assertion utilities
// =============================================================================

func containsRule(errs ValidationErrors, rule string) bool {
	for _, e := range errs {
		if e.Rule == rule {
			return true
		}
	}
	return false
}

func containsField(errs ValidationErrors, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

func containsFieldPrefix(errs ValidationErrors, prefix string) bool {
	for _, e := range errs {
		if strings.HasPrefix(e.Field, prefix) {
			return true
		}
	}
	return false
}

func containsWarn(errs ValidationErrors) bool {
	for _, e := range errs {
		if strings.EqualFold(e.Severity, "warn") {
			return true
		}
	}
	return false
}

func hasErrors(errs ValidationErrors) bool {
	for _, e := range errs {
		if strings.EqualFold(e.Severity, "error") {
			return true
		}
	}
	return false
}
