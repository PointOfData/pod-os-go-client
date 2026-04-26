package message

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// HEADER CONSTRUCTION TESTS
// =============================================================================

func TestStoreEventMessageHeader(t *testing.T) {
	tests := []struct {
		name           string
		msg            *Message
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "required fields only",
			msg: &Message{
				Envelope: Envelope{MessageId: "msg-123"},
				Event: &EventFields{
					Owner:             "owner1",
					Timestamp:         "+1234567890.123456",
					Location:          "TERRA|47.6|-122.5",
					LocationSeparator: "|",
				},
				Payload: &PayloadFields{MimeType: "application/json"},
			},
			wantContains: []string{
				"_db_cmd=store",
				"owner=owner1",
				"timestamp=+1234567890.123456",
				"loc=TERRA|47.6|-122.5",
				"loc_delim=|",
				"mime=application/json",
				"_msg_id=msg-123",
			},
		},
		{
			name: "with optional UniqueId and Type",
			msg: &Message{
				Envelope: Envelope{MessageId: "msg-456"},
				Event: &EventFields{
					UniqueId:          "unique-abc",
					Id:                "event-id-123",
					Owner:             "owner1",
					Timestamp:         "+1234567890.123456",
					Location:          "TERRA|47.6|-122.5",
					LocationSeparator: "|",
					Type:              "custom_type",
				},
				Payload: &PayloadFields{MimeType: "text/plain"},
			},
			wantContains: []string{
				"unique_id=unique-abc",
				"event_id=event-id-123",
				"type=custom_type",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := StoreEventMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("StoreEventMessageHeader() missing %q in header: %s", want, header)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(header, notWant) {
					t.Errorf("StoreEventMessageHeader() should not contain %q in header: %s", notWant, header)
				}
			}
		})
	}
}

func TestStoreBatchEventsMessageHeader(t *testing.T) {
	msg := &Message{
		Envelope: Envelope{MessageId: "batch-msg-123"},
	}

	header := StoreBatchEventsMessageHeader(msg)

	wantContains := []string{
		"_db_cmd=store_batch",
		"_msg_id=batch-msg-123",
	}

	for _, want := range wantContains {
		if !strings.Contains(header, want) {
			t.Errorf("StoreBatchEventsMessageHeader() missing %q in header: %s", want, header)
		}
	}
}

func TestStoreBatchTagsMessageHeader(t *testing.T) {
	tests := []struct {
		name         string
		msg          *Message
		wantContains []string
	}{
		{
			name: "with EventId and Owner",
			msg: &Message{
				Envelope: Envelope{MessageId: "tags-msg-123"},
				Event: &EventFields{
					Id:    "event-id-456",
					Owner: "owner1",
				},
			},
			wantContains: []string{
				"_db_cmd=tag_store_batch",
				"event_id=event-id-456",
				"owner=owner1",
				"_msg_id=tags-msg-123",
			},
		},
		{
			name: "with UniqueId and OwnerUniqueID",
			msg: &Message{
				Envelope: Envelope{MessageId: "tags-msg-456"},
				Event: &EventFields{
					UniqueId:      "unique-abc",
					OwnerUniqueID: "owner-unique-xyz",
				},
			},
			wantContains: []string{
				"_db_cmd=tag_store_batch",
				"unique_id=unique-abc",
				"owner_unique_id=owner-unique-xyz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := StoreBatchTagsMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("StoreBatchTagsMessageHeader() missing %q in header: %s", want, header)
				}
			}
		})
	}
}


func TestGetEventMessageHeader(t *testing.T) {
	tests := []struct {
		name         string
		msg          *Message
		wantContains []string
	}{
		{
			name: "with EventId only",
			msg: &Message{
				Envelope: Envelope{MessageId: "get-msg-123"},
				Event: &EventFields{
					Id: "event-id-123",
				},
			},
			wantContains: []string{
				"_db_cmd=get",
				"event_id=event-id-123",
				"_msg_id=get-msg-123",
			},
		},
		{
			name: "with GetTags and GetLinks options",
			msg: &Message{
				Envelope: Envelope{MessageId: "get-msg-456"},
				Event: &EventFields{
					Id: "event-id-456",
				},
				NeuralMemory: &NeuralMemoryFields{
					GetEvent: &GetEventOptions{
						GetTags:  true,
						GetLinks: true,
						SendData: false,
					},
				},
			},
			wantContains: []string{
				"_db_cmd=get",
				"get_tags=Y",
				"get_links=Y",
			},
		},
		{
			name: "with SendData option",
			msg: &Message{
				Envelope: Envelope{MessageId: "get-msg-789"},
				Event: &EventFields{
					UniqueId: "unique-abc",
				},
				NeuralMemory: &NeuralMemoryFields{
					GetEvent: &GetEventOptions{
						SendData: true,
					},
				},
			},
			wantContains: []string{
				"_db_cmd=get",
				"unique_id=unique-abc",
				"send_data=Y",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := GetEventMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("GetEventMessageHeader() missing %q in header: %s", want, header)
				}
			}
		})
	}
}

func TestGetEventsForTagMessageHeader(t *testing.T) {
	tests := []struct {
		name         string
		msg          *Message
		wantContains []string
	}{
		{
			name: "with basic options",
			msg: &Message{
				Envelope: Envelope{MessageId: "search-msg-123"},
				NeuralMemory: &NeuralMemoryFields{
					GetEventsForTags: &GetEventsForTagsOptions{
						EventPattern:  "test*",
						BufferResults: true,
					},
				},
			},
			wantContains: []string{
				"_db_cmd=events_for_tag",
				"event=test*",
				"buffer_results=Y",
			},
		},
		{
			name: "with Owner",
			msg: &Message{
				Envelope: Envelope{MessageId: "search-msg-456"},
				NeuralMemory: &NeuralMemoryFields{
					GetEventsForTags: &GetEventsForTagsOptions{
						Owner: "owner1",
					},
				},
			},
			wantContains: []string{
				"owner=owner1",
			},
		},
		{
			name: "with OwnerUniqueID",
			msg: &Message{
				Envelope: Envelope{MessageId: "search-msg-789"},
				NeuralMemory: &NeuralMemoryFields{
					GetEventsForTags: &GetEventsForTagsOptions{
						OwnerUniqueID: "owner-unique-abc",
					},
				},
			},
			wantContains: []string{
				"owner_unique_id=owner-unique-abc",
			},
		},
		{
			name: "with IncludeBriefHits",
			msg: &Message{
				Envelope: Envelope{MessageId: "search-msg-brief"},
				NeuralMemory: &NeuralMemoryFields{
					GetEventsForTags: &GetEventsForTagsOptions{
						IncludeBriefHits: true,
					},
				},
			},
			wantContains: []string{
				"include_brief_hits=Y",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := GetEventsForTagMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("GetEventsForTagMessageHeader() missing %q in header: %s", want, header)
				}
			}
		})
	}
}

func TestLinkEventsMessageHeader(t *testing.T) {
	tests := []struct {
		name         string
		msg          *Message
		wantContains []string
	}{
		{
			name: "with EventA and EventB",
			msg: &Message{
				Envelope: Envelope{MessageId: "link-msg-123"},
				Event: &EventFields{
					Owner: "owner1",
				},
				NeuralMemory: &NeuralMemoryFields{
					Link: &LinkFields{
						EventA:            "event-a-123",
						EventB:            "event-b-456",
						StrengthA:         0.8,
						StrengthB:         0.5,
						Category:          "related",
						Timestamp:         "+1234567890.123456",
						Location:          "TERRA|47.6|-122.5",
						LocationSeparator: "|",
						Type:              "link_type",
					},
				},
				Payload: &PayloadFields{MimeType: "application/json"},
			},
			wantContains: []string{
				"_db_cmd=link",
				"owner=owner1",
				"event_id_a=event-a-123",
				"event_id_b=event-b-456",
				"strength_a=0.8",
				"strength_b=0.5",
				"category=related",
			},
		},
		{
			name: "with UniqueIdA and UniqueIdB",
			msg: &Message{
				Envelope: Envelope{MessageId: "link-msg-456"},
				Event: &EventFields{
					Owner: "owner1",
				},
				NeuralMemory: &NeuralMemoryFields{
					Link: &LinkFields{
						UniqueIdA:         "unique-a-abc",
						UniqueIdB:         "unique-b-def",
						StrengthA:         1.0,
						StrengthB:         1.0,
						Category:          "parent",
						Timestamp:         "+1234567890.123456",
						Location:          "TERRA",
						LocationSeparator: "|",
					},
				},
				Payload: &PayloadFields{MimeType: "application/json"},
			},
			wantContains: []string{
				"unique_id_a=unique-a-abc",
				"unique_id_b=unique-b-def",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := LinkEventsMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("LinkEventsMessageHeader() missing %q in header: %s", want, header)
				}
			}
		})
	}
}

func TestUnlinkEventsMessageHeader(t *testing.T) {
	tests := []struct {
		name         string
		msg          *Message
		wantContains []string
	}{
		{
			name: "with Id",
			msg: &Message{
				Envelope: Envelope{MessageId: "unlink-msg-123"},
				NeuralMemory: &NeuralMemoryFields{
					Link: &LinkFields{
						Id:                "event-id-123",
						Owner:             "owner1",
						Timestamp:         "+1234567890.123456",
						Location:          "TERRA",
						LocationSeparator: "|",
					},
				},
			},
			wantContains: []string{
				"_db_cmd=unlink",
				"event_id=event-id-123",
				"owner=owner1",
			},
		},
		{
			name: "with UniqueId",
			msg: &Message{
				Envelope: Envelope{MessageId: "unlink-msg-456"},
				NeuralMemory: &NeuralMemoryFields{
					Link: &LinkFields{
						UniqueId:          "unique-abc",
						Timestamp:         "+1234567890.123456",
						Location:          "TERRA",
						LocationSeparator: "|",
					},
				},
			},
			wantContains: []string{
				"_db_cmd=unlink",
				"unique_id=unique-abc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := UnlinkEventsMessageHeader(tt.msg)

			for _, want := range tt.wantContains {
				if !strings.Contains(header, want) {
					t.Errorf("UnlinkEventsMessageHeader() missing %q in header: %s", want, header)
				}
			}
		})
	}
}

func TestBatchLinkEventsMessageHeader(t *testing.T) {
	msg := &Message{
		Envelope: Envelope{MessageId: "batch-link-msg-123"},
	}

	header := BatchLinkEventsMessageHeader(msg)

	wantContains := []string{
		"_db_cmd=link_batch",
		"_msg_id=batch-link-msg-123",
	}

	for _, want := range wantContains {
		if !strings.Contains(header, want) {
			t.Errorf("BatchLinkEventsMessageHeader() missing %q in header: %s", want, header)
		}
	}
}

// =============================================================================
// PAYLOAD ENCODING TESTS
// =============================================================================

func TestFormatBatchEventsPayload(t *testing.T) {
	events := []BatchEventSpec{
		{
			Event: EventFields{
				Owner:             "owner1",
				Timestamp:         "+1234567890.123456",
				Location:          "TERRA|47.6|-122.5",
				LocationSeparator: "|",
			},
			Tags: TagList{
				{Frequency: 1, Key: "key1", Value: "value1"},
			},
		},
		{
			Event: EventFields{
				Owner:             "owner2",
				Timestamp:         "+1234567891.123456",
				Location:          "TERRA|48.0|-123.0",
				LocationSeparator: "|",
			},
		},
	}

	payload := FormatBatchEventsPayload(events)

	// Should contain two lines separated by newline
	lines := strings.Split(payload, "\n")
	if len(lines) != 2 {
		t.Errorf("FormatBatchEventsPayload() expected 2 lines, got %d", len(lines))
	}

	// First line should contain owner1
	if !strings.Contains(lines[0], "owner=owner1") {
		t.Errorf("FormatBatchEventsPayload() first line missing owner1: %s", lines[0])
	}

	// First line should contain tag
	if !strings.Contains(lines[0], "tag_0=1:key1=value1") {
		t.Errorf("FormatBatchEventsPayload() first line missing tag: %s", lines[0])
	}
}

func TestFormatBatchTagsPayload(t *testing.T) {
	tags := TagList{
		{Frequency: 1, Key: "key1", Value: "value1"},
		{Frequency: 5, Key: "key2", Value: "value2"},
		{Frequency: 10, Key: "key3", Value: 123}, // Test int value
	}

	payload := FormatBatchTagsPayload(tags)

	// Should contain three lines separated by newline
	lines := strings.Split(payload, "\n")
	if len(lines) != 3 {
		t.Errorf("FormatBatchTagsPayload() expected 3 lines, got %d", len(lines))
	}

	// Check format: frequency=key=value
	expectedLines := []string{
		"1=key1=value1",
		"5=key2=value2",
		"10=key3=123",
	}

	for i, expected := range expectedLines {
		if lines[i] != expected {
			t.Errorf("FormatBatchTagsPayload() line %d = %q, want %q", i, lines[i], expected)
		}
	}
}

func TestFormatBatchTagsPayload_Empty(t *testing.T) {
	payload := FormatBatchTagsPayload(nil)
	if payload != "" {
		t.Errorf("FormatBatchTagsPayload(nil) = %q, want empty string", payload)
	}

	payload = FormatBatchTagsPayload(TagList{})
	if payload != "" {
		t.Errorf("FormatBatchTagsPayload(empty) = %q, want empty string", payload)
	}
}

func TestFormatBatchLinkEventsPayload(t *testing.T) {
	links := []BatchLinkEventSpec{
		{
			Event: EventFields{
				Owner:             "owner1",
				Timestamp:         "+1234567890.123456",
				Location:          "TERRA",
				LocationSeparator: "|",
			},
			Link: LinkFields{
				EventA:    "event-a-1",
				EventB:    "event-b-1",
				StrengthA: 0.8,
				StrengthB: 0.5,
				Category:  "related",
			},
		},
	}

	payload := FormatBatchLinkEventsPayload(links)

	// Should contain link fields
	wantContains := []string{
		"event_id_a=event-a-1",
		"event_id_b=event-b-1",
		"strength_a=0.8",
		"strength_b=0.5",
		"category=related",
	}

	for _, want := range wantContains {
		if !strings.Contains(payload, want) {
			t.Errorf("FormatBatchLinkEventsPayload() missing %q in payload: %s", want, payload)
		}
	}
}

// =============================================================================
// RESPONSE DECODING TESTS
// =============================================================================

func TestDecodeMessage_StoreEventResponse(t *testing.T) {
	header := "_type=store\t_status=OK\t_count=1\tlocal_id=local123\t_msg_id=msg-123"
	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001, // MEM_REPLY
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "StoreEventResponse" {
		t.Errorf("Intent.Name = %q, want StoreEventResponse", decoded.Intent.Name)
	}

	if decoded.Response.Status != "OK" {
		t.Errorf("Response.Status = %q, want OK", decoded.Response.Status)
	}

	if decoded.Response.TotalEvents != 1 {
		t.Errorf("Response.TotalEvents = %d, want 1", decoded.Response.TotalEvents)
	}

	if decoded.MessageId != "msg-123" {
		t.Errorf("MessageId = %q, want msg-123", decoded.MessageId)
	}
}

func TestDecodeMessage_StoreBatchEventsResponse(t *testing.T) {
	header := "_type=store_batch\t_status=OK\t_count=2\t_user=testuser\t_msg_id=msg-456"
	payload := "_event_id=event1\t_status=OK\n_event_id=event2\t_status=OK"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "StoreBatchEventsResponse" {
		t.Errorf("Intent.Name = %q, want StoreBatchEventsResponse", decoded.Intent.Name)
	}

	if len(decoded.Response.StoreBatchEventRecord.EventResults) != 2 {
		t.Errorf("StoreBatchEventRecord.EventResults count = %d, want 2", len(decoded.Response.StoreBatchEventRecord.EventResults))
	}
}

func TestDecodeMessage_GetEventResponse(t *testing.T) {
	header := "_type=get\t_status=OK\t_event_id=event123\t_unique_id=unique456\t_tag_count=3\t_link_count=2\t_datasize=100\t_mimetype=application/json"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "GetEventResponse" {
		t.Errorf("Intent.Name = %q, want GetEventResponse", decoded.Intent.Name)
	}

	if decoded.Event.Id != "event123" {
		t.Errorf("Event.Id = %q, want event123", decoded.Event.Id)
	}

	if decoded.Response.TagCount != 3 {
		t.Errorf("Response.TagCount = %d, want 3", decoded.Response.TagCount)
	}

	if decoded.Response.LinkCount != 2 {
		t.Errorf("Response.LinkCount = %d, want 2", decoded.Response.LinkCount)
	}
}

func TestDecodeMessage_GetEventsForTagsResponse_BriefHits(t *testing.T) {
	header := "_type=events_for_tag\t_status=OK\t_count=2"
	payload := "_brief_hit=event1\t_hits=5\n_brief_hit=event2\t_hits=3"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "GetEventsForTagsResponse" {
		t.Errorf("Intent.Name = %q, want GetEventsForTagsResponse", decoded.Intent.Name)
	}

	if len(decoded.Response.BriefHits) != 2 {
		t.Errorf("BriefHits count = %d, want 2", len(decoded.Response.BriefHits))
	}

	if decoded.Response.BriefHits[0].EventId != "event1" {
		t.Errorf("BriefHits[0].EventId = %q, want event1", decoded.Response.BriefHits[0].EventId)
	}

	if decoded.Response.BriefHits[0].TotalHits != 5 {
		t.Errorf("BriefHits[0].TotalEvents = %d, want 5", decoded.Response.BriefHits[0].TotalHits)
	}
}

func TestDecodeMessage_GetEventsForTagsResponse_Events(t *testing.T) {
	header := "_type=events_for_tag\t_status=OK\t_count=1"
	payload := "_event_id=event1\towner=owner1\ttag:1:key1=value1"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if len(decoded.Response.EventRecords) != 1 {
		t.Errorf("EventRecords count = %d, want 1", len(decoded.Response.EventRecords))
	}

	if decoded.Response.EventRecords[0].Id != "event1" {
		t.Errorf("EventRecords[0].Id = %q, want event1", decoded.Response.EventRecords[0].Id)
	}
}

func TestDecodeMessage_LinkEventResponse(t *testing.T) {
	header := "_type=link\t_status=OK\t_count=1\tlink_event=link123\t_msg_id=msg-789"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "LinkEventResponse" {
		t.Errorf("Intent.Name = %q, want LinkEventResponse", decoded.Intent.Name)
	}

	if decoded.Response.LinkId != "link123" {
		t.Errorf("Response.LinkId = %q, want link123", decoded.Response.LinkId)
	}
}

func TestDecodeMessage_UnlinkEventResponse(t *testing.T) {
	header := "_type=unlink\t_status=OK\t_count=1"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		"",
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "UnlinkEventResponse" {
		t.Errorf("Intent.Name = %q, want UnlinkEventResponse", decoded.Intent.Name)
	}

	if decoded.Response.Status != "OK" {
		t.Errorf("Response.Status = %q, want OK", decoded.Response.Status)
	}
}

func TestDecodeMessage_StoreBatchLinksResponse(t *testing.T) {
	header := "_type=link_batch\t_status=OK\t_total_link_requests_found=2\t_links_ok=2\t_links_with_errors=0"
	payload := "_status=OK\tevent_id_a=eventA1\tevent_id_b=eventB1\tstrength_a=0.8\tstrength_b=0.5\tcategory=related\n_status=OK\tevent_id_a=eventA2\tevent_id_b=eventB2\tstrength_a=1.0\tstrength_b=1.0\tcategory=parent"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Intent.Name != "StoreBatchLinksResponse" {
		t.Errorf("Intent.Name = %q, want StoreBatchLinksResponse", decoded.Intent.Name)
	}

	if decoded.Response.TotalEvents != 2 {
		t.Errorf("Response.TotalEvents = %d, want 2", decoded.Response.TotalEvents)
	}

	if decoded.Response.StorageSuccessCount != 2 {
		t.Errorf("Response.StorageSuccessCount = %d, want 2", decoded.Response.StorageSuccessCount)
	}

	if len(decoded.Response.StoreLinkBatchEventRecord.LinkResults) != 2 {
		t.Errorf("StoreLinkBatchEventRecord.LinkResults count = %d, want 2", len(decoded.Response.StoreLinkBatchEventRecord.LinkResults))
	}

	// Verify first link record
	if decoded.Response.StoreLinkBatchEventRecord.LinkResults[0].EventA != "eventA1" {
		t.Errorf("First link EventA = %q, want eventA1", decoded.Response.StoreLinkBatchEventRecord.LinkResults[0].EventA)
	}

	if decoded.Response.StoreLinkBatchEventRecord.LinkResults[0].Category != "related" {
		t.Errorf("First link Category = %q, want related", decoded.Response.StoreLinkBatchEventRecord.LinkResults[0].Category)
	}
}

// =============================================================================
// ROUND-TRIP TESTS
// =============================================================================

func TestEncodeDecodeRoundTrip_StoreEvent(t *testing.T) {
	originalMsg := &Message{
		Envelope: Envelope{
			To:        "mem@gateway.example.com",
			From:      "client@gateway.example.com",
			Intent:    IntentType.StoreEvent,
			MessageId: "roundtrip-123",
		},
		Event: &EventFields{
			UniqueId:          "unique-test-123",
			Owner:             "test-owner",
			Timestamp:         "+1234567890.123456",
			Location:          "TERRA|47.6|-122.5",
			LocationSeparator: "|",
			Type:              "test_event",
		},
		Payload: &PayloadFields{
			MimeType: "application/json",
			Data:     `{"test": "data"}`,
		},
	}

	// Encode
	encoded, err := EncodeMessage(originalMsg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() error = %v", err)
	}

	// Verify encoded message contains expected header fields
	msgStr := string(encoded.MessageBytes)
	wantContains := []string{
		"_db_cmd=store",
		"unique_id=unique-test-123",
		"owner=test-owner",
		"_msg_id=roundtrip-123",
	}

	for _, want := range wantContains {
		if !strings.Contains(msgStr, want) {
			t.Errorf("Encoded message missing %q", want)
		}
	}
}

func TestEncodeDecodeRoundTrip_StoreBatchTags(t *testing.T) {
	originalMsg := &Message{
		Envelope: Envelope{
			To:        "mem@gateway.example.com",
			From:      "client@gateway.example.com",
			Intent:    IntentType.StoreBatchTags,
			MessageId: "batch-tags-123",
		},
		Event: &EventFields{
			UniqueId:      "unique-event-123",
			OwnerUniqueID: "owner-unique-456",
		},
		NeuralMemory: &NeuralMemoryFields{
			Tags: TagList{
				{Frequency: 1, Key: "key1", Value: "value1"},
				{Frequency: 2, Key: "key2", Value: "value2"},
			},
		},
		Payload: &PayloadFields{},
	}

	// Encode
	encoded, err := EncodeMessage(originalMsg, "conv-uuid")
	if err != nil {
		t.Fatalf("EncodeMessage() error = %v", err)
	}

	// Verify encoded message contains expected header fields
	msgStr := string(encoded.MessageBytes)

	if !strings.Contains(msgStr, "_db_cmd=tag_store_batch") {
		t.Error("Encoded message missing _db_cmd=tag_store_batch")
	}

	if !strings.Contains(msgStr, "unique_id=unique-event-123") {
		t.Error("Encoded message missing unique_id")
	}

	if !strings.Contains(msgStr, "owner_unique_id=owner-unique-456") {
		t.Error("Encoded message missing owner_unique_id")
	}

	// Verify payload contains tags in correct format
	if !strings.Contains(msgStr, "1=key1=value1") {
		t.Error("Encoded message missing tag 1")
	}
}

func TestConstructHeader_AllIntents(t *testing.T) {
	// Test that ConstructHeader returns non-empty strings for all supported intents
	tests := []struct {
		intent     Intent
		setupMsg   func() *Message
		wantPrefix string
	}{
		{
			intent: IntentType.GatewayId,
			setupMsg: func() *Message {
				return &Message{
					Envelope: Envelope{ClientName: "test-client"},
				}
			},
			wantPrefix: "id:name=",
		},
		{
			intent: IntentType.StoreEvent,
			setupMsg: func() *Message {
				return &Message{
					Event:   &EventFields{Owner: "owner1", Timestamp: "+123", Location: "L", LocationSeparator: "|"},
					Payload: &PayloadFields{MimeType: "text/plain"},
				}
			},
			wantPrefix: "_db_cmd=store",
		},
		{
			intent: IntentType.StoreBatchEvents,
			setupMsg: func() *Message {
				return &Message{}
			},
			wantPrefix: "_db_cmd=store_batch",
		},
		{
			intent:     IntentType.StoreBatchTags,
			setupMsg:   func() *Message { return &Message{Event: &EventFields{Id: "e1", Owner: "o1"}} },
			wantPrefix: "_db_cmd=tag_store_batch",
		},
		{
			intent:     IntentType.GetEvent,
			setupMsg:   func() *Message { return &Message{Event: &EventFields{Id: "e1"}} },
			wantPrefix: "_db_cmd=get",
		},
		{
			intent: IntentType.GetEventsForTags,
			setupMsg: func() *Message {
				return &Message{NeuralMemory: &NeuralMemoryFields{GetEventsForTags: &GetEventsForTagsOptions{}}}
			},
			wantPrefix: "_db_cmd=events_for_tag",
		},
		{
			intent: IntentType.LinkEvent,
			setupMsg: func() *Message {
				return &Message{
					Event: &EventFields{Owner: "o1"},
					NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{
						EventA: "a", EventB: "b", StrengthA: 1, StrengthB: 1, Category: "c",
						Timestamp: "+123", Location: "L", LocationSeparator: "|",
					}},
					Payload: &PayloadFields{MimeType: "text/plain"},
				}
			},
			wantPrefix: "_db_cmd=link",
		},
		{
			intent: IntentType.UnlinkEvent,
			setupMsg: func() *Message {
				return &Message{NeuralMemory: &NeuralMemoryFields{Link: &LinkFields{Id: "e1"}}}
			},
			wantPrefix: "_db_cmd=unlink",
		},
		{
			intent:     IntentType.StoreBatchLinks,
			setupMsg:   func() *Message { return &Message{} },
			wantPrefix: "_db_cmd=link_batch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.intent.Name, func(t *testing.T) {
			msg := tt.setupMsg()
			header := ConstructHeader(msg, tt.intent, "conv-uuid")

			if header == "" {
				t.Errorf("ConstructHeader() for %s returned empty string", tt.intent.Name)
			}

			if !strings.HasPrefix(header, tt.wantPrefix) {
				t.Errorf("ConstructHeader() for %s = %q, want prefix %q", tt.intent.Name, header, tt.wantPrefix)
			}
		})
	}
}

// =============================================================================
// HELPER TESTS
// =============================================================================

func TestParseEventTagField(t *testing.T) {
	tests := []struct {
		input     string
		wantFreq  int
		wantKey   string
		wantValue string
	}{
		{
			input:     "event_tag:000000001:5=category=test_value",
			wantFreq:  5,
			wantKey:   "category",
			wantValue: "test_value",
		},
		{
			input:     "event_tag:000000002:1=simple_key=simple_value",
			wantFreq:  1,
			wantKey:   "simple_key",
			wantValue: "simple_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tag := parseEventTagField(tt.input)
			if tag == nil {
				t.Fatal("parseEventTagField() returned nil")
			}

			if tag.Frequency != tt.wantFreq {
				t.Errorf("Frequency = %d, want %d", tag.Frequency, tt.wantFreq)
			}

			if tag.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", tag.Key, tt.wantKey)
			}

			if tag.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", tag.Value, tt.wantValue)
			}
		})
	}
}

func TestSerializeTagValue(t *testing.T) {
	tests := []struct {
		input interface{}
		want  string
	}{
		{"hello", "hello"},
		{123, "123"},
		{45.67, "45.67"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
		{[]byte("bytes"), "Ynl0ZXM="}, // Base64 encoded
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			got := SerializeTagValue(tt.input)
			if got != tt.want {
				t.Errorf("SerializeTagValue(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDecodeMessage_GetEventsForTagsResponse_LinksPopulateUniqueIdA(t *testing.T) {
	header := "_type=events_for_tag\t_status=OK\t_count=1"
	payload := "_event_id=evt1\tunique_id=src_uid\ttag:1:_unique_id=src_uid\n" +
		"_link=link1\tsource=evt1\ttarget=evt2\tstrength=0.9\tcategory=describes\ttarget_unique_id=tgt_uid"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if len(decoded.Response.EventRecords) != 1 {
		t.Fatalf("EventRecords count = %d, want 1", len(decoded.Response.EventRecords))
	}

	event := decoded.Response.EventRecords[0]
	if len(event.Links) != 1 {
		t.Fatalf("Links count = %d, want 1", len(event.Links))
	}

	link := event.Links[0]
	if link.UniqueIdA != "src_uid" {
		t.Errorf("Link.UniqueIdA = %q, want %q", link.UniqueIdA, "src_uid")
	}
	if link.UniqueIdB != "tgt_uid" {
		t.Errorf("Link.UniqueIdB = %q, want %q", link.UniqueIdB, "tgt_uid")
	}
}

func TestDecodeMessage_GetEventResponse_LinksPopulateUniqueIdA(t *testing.T) {
	header := "_type=get\t_status=OK\t_event_id=evt1\t_unique_id=src_uid"
	payload := "_link=link1\ttarget_event=evt2\ttarget_unique_id=tgt_uid\tstrength=0.9\tcategory=describes"

	msg := buildMinimalMessage(
		"mem@gateway.example.com",
		"client@gateway.example.com",
		header,
		1001,
		0,
		payload,
	)

	decoded, err := DecodeMessage(msg)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	if decoded.Event == nil {
		t.Fatal("Event is nil")
	}

	if len(decoded.Event.Links) != 1 {
		t.Fatalf("Links count = %d, want 1", len(decoded.Event.Links))
	}

	link := decoded.Event.Links[0]
	if link.UniqueIdA != "src_uid" {
		t.Errorf("Link.UniqueIdA = %q, want %q", link.UniqueIdA, "src_uid")
	}
	if link.UniqueIdB != "tgt_uid" {
		t.Errorf("Link.UniqueIdB = %q, want %q", link.UniqueIdB, "tgt_uid")
	}
}
