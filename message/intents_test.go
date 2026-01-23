package message

import (
	"testing"
)

func TestIntentFromMessageTypeAndCommand(t *testing.T) {
	tests := []struct {
		name             string
		messageType      int
		command          string
		expectFound      bool
		expectIntentName string
	}{
		// MEM_REQ (1000) - Request intents
		{"MEM_REQ store", 1000, "store", true, "StoreEvent"},
		{"MEM_REQ store_batch", 1000, "store_batch", true, "StoreBatchEvents"},
		{"MEM_REQ tag_store_batch", 1000, "tag_store_batch", true, "StoreBatchTags"},
		{"MEM_REQ get", 1000, "get", true, "GetEvent"},
		{"MEM_REQ events_for_tag", 1000, "events_for_tag", true, "GetEventsForTags"},
		{"MEM_REQ link", 1000, "link", true, "LinkEvent"},
		{"MEM_REQ unlink", 1000, "unlink", true, "UnlinkEvent"},
		{"MEM_REQ link_batch", 1000, "link_batch", true, "StoreBatchLinks"},
		{"MEM_REQ unknown command", 1000, "unknown", false, ""},

		// MEM_REPLY (1001) - Response intents
		{"MEM_REPLY store", 1001, "store", true, "StoreEventResponse"},
		{"MEM_REPLY store_batch", 1001, "store_batch", true, "StoreBatchEventsResponse"},
		{"MEM_REPLY tag_store_batch", 1001, "tag_store_batch", true, "StoreBatchTagsResponse"},
		{"MEM_REPLY get", 1001, "get", true, "GetEventResponse"},
		{"MEM_REPLY events_for_tag", 1001, "events_for_tag", true, "GetEventsForTagsResponse"},
		{"MEM_REPLY events_for_tags", 1001, "events_for_tags", true, "GetEventsForTagsResponse"},
		{"MEM_REPLY link", 1001, "link", true, "LinkEventResponse"},
		{"MEM_REPLY unlink", 1001, "unlink", true, "UnlinkEventResponse"},
		{"MEM_REPLY link_batch", 1001, "link_batch", true, "StoreBatchLinksResponse"},
		{"MEM_REPLY unknown command", 1001, "unknown", false, ""},

		// Other message types - fallback to intentFromMessageTypeInt
		{"ActorEcho", 2, "", true, "ActorEcho"},
		{"ActorStart", 1, "", true, "ActorStart"},
		{"GatewayStatus", 3, "", true, "GatewayStatus"},
		{"Unknown messageType", 9999, "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intent, found := IntentFromMessageTypeAndCommand(tt.messageType, tt.command)
			if found != tt.expectFound {
				t.Errorf("IntentFromMessageTypeAndCommand(%d, %q) found = %v, want %v",
					tt.messageType, tt.command, found, tt.expectFound)
			}
			if found && intent.Name != tt.expectIntentName {
				t.Errorf("IntentFromMessageTypeAndCommand(%d, %q) intent.Name = %q, want %q",
					tt.messageType, tt.command, intent.Name, tt.expectIntentName)
			}
		})
	}
}

func TestIntentFromResponseCommand(t *testing.T) {
	tests := []struct {
		command          string
		expectFound      bool
		expectIntentName string
	}{
		{"store", true, "StoreEventResponse"},
		{"store_batch", true, "StoreBatchEventsResponse"},
		{"tag_store_batch", true, "StoreBatchTagsResponse"},
		{"get", true, "GetEventResponse"},
		{"events_for_tag", true, "GetEventsForTagsResponse"},
		{"events_for_tags", true, "GetEventsForTagsResponse"}, // Both variants supported
		{"link", true, "LinkEventResponse"},
		{"unlink", true, "UnlinkEventResponse"},
		{"link_batch", true, "StoreBatchLinksResponse"},
		{"unknown", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			intent, found := IntentFromResponseCommand(tt.command)
			if found != tt.expectFound {
				t.Errorf("IntentFromResponseCommand(%q) found = %v, want %v",
					tt.command, found, tt.expectFound)
			}
			if found && intent.Name != tt.expectIntentName {
				t.Errorf("IntentFromResponseCommand(%q) intent.Name = %q, want %q",
					tt.command, intent.Name, tt.expectIntentName)
			}
		})
	}
}

func TestResponseIntentsHaveCorrectMessageType(t *testing.T) {
	// Verify all response intents have MessageType 1001 (MEM_REPLY)
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
		if intent.MessageType != 1001 {
			t.Errorf("%s.MessageType = %d, want 1001", intent.Name, intent.MessageType)
		}
		if intent.RoutingMessageType != "MEM_REPLY" {
			t.Errorf("%s.RoutingMessageType = %q, want MEM_REPLY", intent.Name, intent.RoutingMessageType)
		}
	}
}

func TestRequestIntentsHaveCorrectMessageType(t *testing.T) {
	// Verify all request intents have MessageType 1000 (MEM_REQ)
	requestIntents := []Intent{
		IntentType.StoreEvent,
		IntentType.StoreBatchEvents,
		IntentType.StoreBatchTags,
		IntentType.GetEvent,
		IntentType.GetEventsForTags,
		IntentType.LinkEvent,
		IntentType.UnlinkEvent,
		IntentType.StoreBatchLinks,
	}

	for _, intent := range requestIntents {
		if intent.MessageType != 1000 {
			t.Errorf("%s.MessageType = %d, want 1000", intent.Name, intent.MessageType)
		}
		if intent.RoutingMessageType != "MEM_REQ" {
			t.Errorf("%s.RoutingMessageType = %q, want MEM_REQ", intent.Name, intent.RoutingMessageType)
		}
	}
}
