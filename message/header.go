package message

import (
	"strconv"
	"strings"
)

// ConstructHeader constructs the header for a message using the new composition structure.
// Halt:        99,
//
//	Start:       1,
//		Echo:        2,
//		Status:      3,
//		Request:     4,
//		Id:          5,
//		Disconnect:  6,
//		SendNext:    7,
//		NoSend:      8,
//		StreamOff:   9,
//		StreamOn:    10,
//		Record:      11,
//		BatchStart:  12,
//		BatchEnd:    13,
//		DbCommand:  1000,
//		AllQueued:   15,
//		User: 	  65536,
//
// Params: message *Message: The message to be sent.
//
// Returns: header string
func ConstructHeader(msg *Message, intent Intent, connectionIdUuid string) string {

	switch intent.Name {
	case "GatewayId": // ID Message
		return GatewayIdentifyConnectionHeader(msg) // VERIFIED: DO NOT CHANGE

	case "GatewayStreamOn": // Stream On
		return GatewayStreamOnHeader(msg) // VERIFIED: DO NOT CHANGE

	case "ActorEcho": // Echo Message
		return ActorEchoHeader(msg) // VERIFIED: DO NOT CHANGE

	case "StoreEvent": // Store Event
		return StoreEventMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "LinkEvent": // Link Event
		return LinkEventsMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "UnlinkEvent": // Unlink Event
		return UnlinkEventsMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "GetEvent": // Get Event
		return GetEventMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "GetEventsForTags": // Get Events For Tags
		return GetEventsForTagMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "StoreBatchEvents": // Batch Store Events
		return StoreBatchEventsMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "BatchStoreTags": // Batch Store Tags
		return StoreBatchTagsMessageHeader(msg) // VERIFIED: DO NOT CHANGE

	case "StoreBatchLinks": // Batch Link Events
		return BatchLinkEventsMessageHeader(msg)

	case "ActorStreamOff": // Stream Off
		return GatewayStreamOffHeader(msg)

	case "ActorRequest": // Request Message
		return ActorRequestHeader(msg)

	default:
		// If the intent is not recognized, return an empty string.
		return ""
	}
}

// GatewayIdentifyConnectionHeader constructs the named client connection.
//
// Params:
// msg: The Message struct containing GatewayIdentifyConnection fields.
// Required fields: ClientName
//
// Returns: header string
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func GatewayIdentifyConnectionHeader(msg *Message) string {
	var _header strings.Builder
	if msg.Passcode != "" && msg.UserName != "" {
		_header.WriteString("id:passcode=" + msg.Passcode + "\t")
		_header.WriteString("id:user=" + msg.UserName + "\t")
	}
	_header.WriteString("id:name=" + msg.ClientName + "\t")
	_header.WriteString("_msg_id=" + msg.MessageId)

	return _header.String()
}

// ActorHaltHeader constructs the actor halt header.
//
// Params:
//
// msg: The Message struct containing ActorHalt fields. Required fields: ClientName.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func ActorHaltHeader(msg *Message) string {
	var _header strings.Builder
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// GatewayStreamOnHeader directs the Gateway to stream messages (enable asynchronous messages) to the client.
//
// Params:
//
// msg: The Message struct containing GatewayStreamOn fields. Required fields: ClientName.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func GatewayStreamOnHeader(msg *Message) string {
	var _header strings.Builder
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// GatewayStreamOffHeader directs the Gateway to stop streaming messages (enable synchronous message traffic) to the client.
//
// Params:
//
// msg: The Message struct containing GatewayStreamOff fields. Required fields: ClientName.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func GatewayStreamOffHeader(msg *Message) string {
	var _header strings.Builder
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// ActorRequestHeader constructs the actor request header.
// This message is used to send a request to check the Actor's status.
//
// Params:
//
// msg: The Message struct containing ActorRequest fields. Required fields: ClientName.
//
// Returns: string for header
func ActorRequestHeader(msg *Message) string {
	var _header strings.Builder
	_header.WriteString("_type=" + "status" + "\t")
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// ActorEchoHeader constructs the actor echo header.
// This message will cause the actor to reply with a message containing all of the fields found in the sent
// header, but prefixed with "echo:". The data type and data payload will also be echoed.
//
// Params:
//
// msg: The Message struct containing ActorEcho fields. Required fields: ClientName.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func ActorEchoHeader(msg *Message) string {
	var _header strings.Builder
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// MemStoreEventMessageHeader constructs the Neural Memory Database store event message header.
//
// Params:
//
// msg: The Message struct containing StoreEvent fields. Required fields: ClientName.
// Optional fields include Event.Owner, Event.UniqueId, Event.Type, Event.LocationSeparator,
// Event.Location, Event.Timestamp, and Payload.MimeType.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func StoreEventMessageHeader(msg *Message) string {
	var eventOwner string

	// Construct the owner string.
	switch eventOwner {
	case "":
		switch msg.Event.CreateOwner {
		case "Y":
			eventOwner = "create_owner=Y"
		case "N":
			eventOwner = "create_owner=N"
		default:
			eventOwner = "create_owner=Y"
		}
	case "$sys":
		eventOwner = "owner=" + eventOwner + "\t"
	default:
		eventOwner = "owner=" + forceASCII(eventOwner) + "\t"
	}

	// Build the final string
	var _header strings.Builder
	_header.WriteString("_db_cmd=" + "store" + "\t")
	if msg.Event.UniqueId != "" {
		_header.WriteString("unique_id=" + msg.Event.UniqueId + "\t")
	}
	_header.WriteString(eventOwner)
	if msg.Event.Timestamp != "" {
		_header.WriteString("timestamp=" + msg.Event.Timestamp + "\t")
	} else {
		_header.WriteString("timestamp=" + GetTimestamp() + "\t")
	}

	_header.WriteString("loc_delim=" + msg.Event.LocationSeparator + "\t")
	_header.WriteString("loc=" + msg.Event.Location + "\t")

	if msg.Event.Type != "" {
		_header.WriteString("type=" + msg.Event.Type + "\t")
	} else {
		_header.WriteString("type=" + "store event" + "\t")
	}

	_header.WriteString("mime=" + msg.Payload.MimeType + "\t")
	_header.WriteString("_msg_id=" + msg.MessageId)
	return _header.String()
}

// MemStoreBatchEventsMessageHeader constructs the machine neural memory (mem) store event message header.
//
// Params:
//
// msg: The Message struct containing StoreBatchEvents fields. Required fields: ClientName.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func StoreBatchEventsMessageHeader(msg *Message) string {

	// Build the final string
	var _header strings.Builder
	_header.WriteString("_db_cmd=" + "store_batch" + "\t")
	if msg.MessageId != "" {
		_header.WriteString("_msg_id=" + msg.MessageId + "\t")
	}
	return _header.String()
}

// StoreBatchTagsMessageHeader constructs header string to add n tags to an event.
//
// Params:
//
// msg: The Message struct containing BatchStoreTags fields. Required fields: ClientName.
// Optional fields include Event.UniqueId, Event.Id, and Event.Owner.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func StoreBatchTagsMessageHeader(msg *Message) string {
	var eventOwner string

	// Construct the owner string.
	switch eventOwner {
	case "":
		eventOwner = "owner=$sys"
	case "$sys":
		eventOwner = "owner=$sys\t"
	default:
		eventOwner = "owner=" + eventOwner + "\t"
	}

	// Build the final string
	var _header strings.Builder
	_header.WriteString("_db_cmd=" + "tag_store_batch" + "\t")

	if msg.Event.UniqueId != "" {
		_header.WriteString("unique_id=" + msg.Event.UniqueId + "\t")
	} else if msg.Event.Id != "" {
		_header.WriteString("event_id=" + forceASCII(msg.Event.Id) + "\t")
	}
	_header.WriteString(eventOwner)
	if msg.MessageId != "" {
		_header.WriteString("_msg_id=" + msg.MessageId)
	}
	return _header.String()
}

// GetEventMessageHeader constructs the _get_ event message header.
//
// Params:
//
// msg: The Message struct containing GetEvent fields. Required fields: ClientName.
// Optional fields include Event.Id, Event.UniqueId, and NeuralMemory.GetEvent options.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func GetEventMessageHeader(msg *Message) string {

	var header strings.Builder

	// Base command
	header.WriteString("_db_cmd=get\t")
	/* 	if msg.Event.Timestamp != "" {
	   		header.WriteString("timestamp=" + msg.Event.Timestamp + "\t")
	   	} else {
	   		header.WriteString("timestamp=" + GetTimestamp() + "\t")
	   	}
	   	if msg.Event.LocationSeparator != "" {
	   		header.WriteString("loc_delim=" + msg.Event.LocationSeparator + "\t")
	   	}
	   	if msg.Event.Location != "" {
	   		header.WriteString("loc=" + msg.Event.Location + "\t")
	   	} */

	// Event identifiers from Event struct
	if msg.Event != nil {
		if msg.Event.Id != "" {
			header.WriteString("event_id=" + forceASCII(msg.Event.Id) + "\t")
		}
		if msg.Event.UniqueId != "" {
			header.WriteString("unique_id=" + msg.Event.UniqueId + "\t")
		}
	}

	// GetEvent options from NeuralMemory.GetEvent
	if opts := msg.GetEventOpts(); opts != nil {
		// Boolean flags (only include when true)
		if opts.SendData {
			header.WriteString("send_data=Y\t")
		}
		if opts.LocalIdOnly {
			header.WriteString("local_id_only=Y\t")
		}
		if opts.GetTags {
			header.WriteString("get_tags=Y\t")
		}
		if opts.GetLinks {
			header.WriteString("get_links=Y\t")
		}
		if opts.GetLinkTags {
			header.WriteString("get_link_tags=Y\t")
		}
		if opts.GetTargetTags {
			header.WriteString("get_target_tags=Y\t")
		}

		// String fields (only include when non-empty)
		if opts.EventFacetFilter != "" {
			header.WriteString("event_facet_filter=" + opts.EventFacetFilter + "\t")
		}
		if opts.LinkFacetFilter != "" {
			header.WriteString("link_facet_filter=" + opts.LinkFacetFilter + "\t")
		}
		if opts.TargetFacetFilter != "" {
			header.WriteString("target_facet_filter=" + opts.TargetFacetFilter + "\t")
		}
		if opts.CategoryFilter != "" {
			header.WriteString("category_filter=" + opts.CategoryFilter + "\t")
		}
		if opts.TagFilter != "" {
			header.WriteString("tag_filter=" + opts.TagFilter + "\t")
		}

		// Integer fields
		if opts.TagFormat.Valid {
			header.WriteString("tag_format=" + strconv.Itoa(opts.TagFormat.Value) + "\t")
		}
		if opts.RequestFormat == 2 {
			header.WriteString("request_format=2\t")
		}
		if opts.FirstLink > 0 {
			header.WriteString("first_link=" + strconv.Itoa(opts.FirstLink) + "\t")
		}
		if opts.LinkCount > 0 {
			header.WriteString("link_count=" + strconv.Itoa(opts.LinkCount) + "\t")
		}
	}

	// Message ID (always last)
	if msg.MessageId != "" {
		header.WriteString("_msg_id=" + msg.MessageId)
	}

	return header.String()
}

// MemGetEventsForTagMessageHeader constructs the Machine Neural Memory (mem) events_for_tag header.
// Lists all events associated with tag(s) that match a pattern.
//
// Params:
//
// msg: The Message struct containing GetEventsForTags fields. Required fields: ClientName.
// Optional fields include NeuralMemory.GetEventsForTags options.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func GetEventsForTagMessageHeader(msg *Message) string {

	var header strings.Builder

	// Base command
	header.WriteString("_db_cmd=events_for_tag\t")

	// Get options from composition
	opts := msg.GetEventsForTagsOpts()

	// Buffer results and format - check opts first, then fallback
	bufferResults := false
	includeTagStats := false
	invertHitTagFilter := false
	hitTagFilter := ""
	bufferFormat := "0"

	if opts != nil {
		bufferResults = opts.BufferResults
		includeTagStats = opts.IncludeTagStats
		invertHitTagFilter = opts.InvertHitTagFilter
		hitTagFilter = opts.HitTagFilter
		if opts.BufferFormat != "" {
			bufferFormat = opts.BufferFormat
		}
	}

	// Buffer results - always include (Y for buffered in payload, N for individual)
	if bufferResults {
		header.WriteString("buffer_results=Y\t") // V
	} else {
		header.WriteString("buffer_results=N\t") // V
	}

	// Boolean flags (only include when true)
	if includeTagStats {
		header.WriteString("include_tag_stats=Y\t") // V
	}

	if opts != nil {
		if opts.IncludeBriefHits {
			header.WriteString("include_brief_hits=Y\t") //V
		}
		if opts.GetAllData {
			header.WriteString("get_all_data=Y\t") // V
		}
		if opts.CountOnly {
			header.WriteString("count_only=Y\t") // V
		}
		if opts.GetMatchLinks {
			header.WriteString("get_match_links=Y\t") //V
		}
		if opts.CountMatchLinks {
			header.WriteString("count_match_links=Y\t") //V
		}
		if opts.GetLinkTags {
			header.WriteString("get_link_tags=Y\t") // V
		}
		if invertHitTagFilter {
			header.WriteString("invert_hit_tag_filter=Y\t") // V
		}
		if opts.NoEventData {
			header.WriteString("no_event_data=Y\t") // V
		}

		// String fields (only include when non-empty)
		if opts.EventPattern != "" {
			header.WriteString("event=" + forceASCII(opts.EventPattern) + "\t") // V FASTPATTERN supported
		}
		if opts.EventPatternHigh != "" {
			header.WriteString("event_high=" + forceASCII(opts.EventPatternHigh) + "\t") // V
		}
		if opts.LinkTagFilter != "" {
			header.WriteString("link_tag_filter=" + forceASCII(opts.LinkTagFilter) + "\t") // V
		}
		if opts.LinkedEventsFilter != "" {
			header.WriteString("linked_events_tag_filter=" + forceASCII(opts.LinkedEventsFilter) + "\t") // V
		}
		if opts.LinkCategory != "" {
			header.WriteString("link_category=" + opts.LinkCategory + "\t") // V
		}
		if opts.Owner != "" {
			header.WriteString("owner=" + forceASCII(opts.Owner) + "\t") //V
		}
		if opts.CreateOwner {
			header.WriteString("create_owner=Y\t") // V
		}
		if hitTagFilter != "" {
			header.WriteString("hit_tag_filter=" + forceASCII(hitTagFilter) + "\t")
		}

		// Integer fields (only include when > 0, except EventsPerMessage which can be -1)
		if opts.FirstLink > 0 {
			header.WriteString("first_link=" + strconv.Itoa(opts.FirstLink) + "\t")
		}
		if opts.LinkCount > 0 {
			header.WriteString("link_count=" + strconv.Itoa(opts.LinkCount) + "\t")
		}
		if opts.EventsPerMessage != 0 {
			header.WriteString("events_per_message=" + strconv.Itoa(opts.EventsPerMessage) + "\t")
		}
		if opts.StartResult > 0 {
			header.WriteString("start_result=" + strconv.Itoa(opts.StartResult) + "\t")
		}
		if opts.EndResult > 0 {
			header.WriteString("end_result=" + strconv.Itoa(opts.EndResult) + "\t")
		}
		if opts.MinEventHits > 0 {
			header.WriteString("min_event_hits=" + strconv.Itoa(opts.MinEventHits) + "\t")
		}
	}

	// Buffer format (always include, defaults to "0")
	header.WriteString("buffer_format=" + bufferFormat + "\t")

	// Message ID (always last)
	if msg.MessageId != "" {
		header.WriteString("_msg_id=" + msg.MessageId)
	}

	return header.String()
}

// MemLinkEventsMessageHeader constructs header string to link two existing events.
//
// Params:
//
// msg: The Message struct containing LinkEvent fields. Required fields: ClientName.
// Optional fields include Event.Owner, Event.CreateOwner, and NeuralMemory.Link fields.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func LinkEventsMessageHeader(msg *Message) string {
	var eventOwner string

	switch eventOwner {
	case "":
		switch msg.Event.CreateOwner {
		case "Y":
			eventOwner = "create_owner=Y"
		case "N":
			eventOwner = "create_owner=N"
		default:
			eventOwner = "create_owner=Y"
		}
	case "$sys":
		eventOwner = "owner=" + eventOwner + "\t"
	default:
		eventOwner = "owner=" + forceASCII(eventOwner) + "\t"
	}

	// Build the final string
	var _header strings.Builder

	_header.WriteString("_db_cmd=" + "link" + "\t")
	_header.WriteString(eventOwner)
	_header.WriteString("id:name=" + msg.ClientName + "\t")
	if msg.MessageId != "" {
		_header.WriteString("_msg_id=" + msg.MessageId + "\t")
	}
	// Prefer UniqueIdA/UniqueIdB if not empty; otherwise use EventA.UniqueId/EventB.UniqueId
	if msg.NeuralMemory.Link.UniqueIdA != "" && msg.NeuralMemory.Link.UniqueIdB != "" {
		_header.WriteString("unique_id_a=" + msg.NeuralMemory.Link.UniqueIdA + "\t")
		_header.WriteString("unique_id_b=" + msg.NeuralMemory.Link.UniqueIdB + "\t")
	} else if msg.NeuralMemory.Link.EventA != "" && msg.NeuralMemory.Link.EventB != "" {
		_header.WriteString("event_a=" + forceASCII(msg.NeuralMemory.Link.EventA) + "\t")
		_header.WriteString("event_b=" + forceASCII(msg.NeuralMemory.Link.EventB) + "\t")
	}

	_header.WriteString("strength_a=" + strconv.FormatFloat(msg.NeuralMemory.Link.StrengthA, 'f', -1, 64) + "\t")
	_header.WriteString("strength_b=" + strconv.FormatFloat(msg.NeuralMemory.Link.StrengthB, 'f', -1, 64) + "\t")
	_header.WriteString("category=" + msg.NeuralMemory.Link.Category)
	_header.WriteString("loc_delim=" + msg.Event.LocationSeparator + "\t")
	_header.WriteString("loc=" + msg.Event.Location + "\t")
	_header.WriteString("type=" + msg.Event.Type + "\t")
	_header.WriteString("mime=" + msg.Payload.MimeType + "\t")
	_header.WriteString("timestamp=" + msg.Event.Timestamp + "\t")

	return _header.String()
}

// MemBatchLinkEventsMessageHeader constructs header string to link a batch of existing events.
// Link details are provided in the payload as newline-separated, tab-separated name=value pairs.
//
// Params:
//
// msg: The Message struct containing BatchLinkEvents fields.
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func BatchLinkEventsMessageHeader(msg *Message) string {

	// Build the final string
	var _header strings.Builder
	_header.WriteString("_db_cmd=" + "link_batch" + "\t")
	if msg.MessageId != "" {
		_header.WriteString("_msg_id=" + msg.MessageId + "\t")
	}
	return _header.String()
}

// UnlinkEventsMessageHeader constructs header string to unlink two existing events.
//
// Params:
//
// msg: The Message struct containing UnlinkEvent fields. Required fields: ClientName.
// Optional fields include Event.UniqueId and Event.Owner.
//
// connectionIdUuid: The connection UUID (not used in header but passed for consistency).
//
// Returns: string for header
// VERIFIED: DO NOT CHANGE FUNCTION SIGNATURE OR PARAMETERS
func UnlinkEventsMessageHeader(msg *Message) string {
	var eventOwner string

	// Construct the owner string.
	switch eventOwner {
	case "":
		switch msg.Event.CreateOwner {
		case "Y":
			eventOwner = "create_owner=Y"
		case "N":
			eventOwner = "create_owner=N"
		default:
			eventOwner = "create_owner=Y"
		}
	case "$sys":
		eventOwner = "owner=" + eventOwner + "\t"
	default:
		eventOwner = "owner=" + forceASCII(eventOwner) + "\t"
	}

	// Build the final string
	var _header strings.Builder
	_header.WriteString("_db_cmd=" + "unlink" + "\t")
	_header.WriteString(eventOwner)
	_header.WriteString("event_id=" + forceASCII(msg.Event.Id) + "\t")
	_header.WriteString("loc_delim=" + msg.Event.LocationSeparator + "\t")
	_header.WriteString("loc=" + msg.Event.Location + "\t")
	_header.WriteString("timestamp=" + msg.Event.Timestamp + "\t")
	_header.WriteString("id:name=" + msg.ClientName + "\t")
	if msg.MessageId != "" {
		_header.WriteString("_msg_id=" + msg.MessageId + "\t")
	}

	return _header.String()
}
