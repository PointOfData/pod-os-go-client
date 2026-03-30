package message

import (
	"crypto"
	"encoding/json"
	"fmt"
)

// NullInt wraps an int to distinguish between a zero value and "not set".
type NullInt struct {
	Value int
	Valid bool // Valid is true if Value is set
}

// String returns a string representation of NullInt
func (n NullInt) String() string {
	if !n.Valid {
		return "Not Set"
	}
	return fmt.Sprintf("%d", n.Value)
}

// Int returns the int value, or 0 if not set
func (n NullInt) Int() int {
	if !n.Valid {
		return 0
	}
	return n.Value
}

// DataType represents the data type for message payloads
type DataType int

const (
	RAW DataType = 0
	// RAW DataType = "0x0000"
	// BPE  DataType = "0x0001"
	// GZIP DataType = "0x0002"
	// LZ7  DataType = "0x0004"
	// BZIP DataType = "0x0008"
	// RC4  DataType = "0x0100"
)

func (c DataType) Int() int {
	return int(c)
}

// DateTimeObject represents AIP date time
type DateTimeObject struct {
	Year        int
	Month       int
	Day         int
	Hour        int
	Minute      int
	Second      int
	Microsecond int
}

// Header interface for message headers
type Header interface {
	Header() string
}

// SocketMessage represents a socket message
type SocketMessage struct {
	MessageBytes []byte
	// Header is the tab-separated key=value wire header produced during encoding.
	// It is populated by EncodeMessage and is available for diagnostic use
	// (e.g. debug logging, wire hooks) without re-parsing MessageBytes.
	Header string
}

// =============================================================================
// ENVELOPE - Core routing fields for all messages
// =============================================================================

// Envelope contains the core routing fields required for all Actor messages.
// These fields are embedded in Message for convenient top-level access.
type Envelope struct {
	To         string // Recipient: <Actor Name>@<Gateway Name>
	From       string // Sender: <Actor Name>@<Gateway Name>
	Intent     Intent // Message intent type (from intents.go)
	ClientName string // Unique client identifier for this connection; required for GatewayId messages.
	MessageId  string `podos:"_msg_id"`     // Unique message identifier for request/response correlation; optional.
	Passcode   string `podos:"id:passcode"` // Optional passcode for authentication and authorization
	UserName   string `podos:"id:user"`     // Optional user name for authentication and authorization
}

// =============================================================================
// EVENT FIELDS - Shared event metadata across intents
// =============================================================================

// EventFields contains fields describing an Event Object.
// Used for StoreEvent, GetEvent, and other event-related operations.
type EventFields struct {
	UniqueId          string         `podos:"unique_id"` // Developer-provided unique ID for the event
	Id                string         `podos:"event_id"`  // AIP-generated unique ID with time and location; must be ASCII encoded
	LocalId           string         // Local machine ID for the event
	Owner             string         `podos:"owner"`           // Owner ID; indicates what entity created the event (default is "$sys")
	OwnerUniqueID     string         `podos:"owner_unique_id"` // Owner unique ID; required field if OwnerID is not provided. This is logically different from the Event owner Ids.
	Timestamp         string         `podos:"timestamp"`       // Event timestamp POSIX timestamp in microseconds; formatted as a string with 6 decimal places with + or - sign relative to January 1, 1970 00:00:00 UTC
	DateTime          DateTimeObject // Event date/time object
	Location          string         `podos:"loc"`       // Location specification (e.g., "TERRA|47.6|-122.5")
	LocationSeparator string         `podos:"loc_delim"` // Location segment delimiter (default "|")
	Type              string         `podos:"type"`      // Developer-defined event type string
	Tags              []TagOutput    `podos:"tags"`      // Tags for the event; for use when processing a Response message.
	Links             []LinkFields   `podos:"links"`     // Links for the event; for use when processing a Response message.
	PayloadData       PayloadFields  // Payload data; for use when processing a Response message so that Payload data is logically associated with the Event represented by EventFields.
	Status            string         `podos:"_status"` // Status of the event; used in StoreBatchEvents response
	Hits              int            `podos:"_hits"`   // Total search term match hits across this event object (from GetEventsForTags response)

}

// =============================================================================
// PAYLOAD FIELDS - Message payload data
// =============================================================================

// PayloadFields contains the message payload data and metadata.
type PayloadFields struct {
	Data     any      `podos:"data"`      // Payload data (string, []byte, or structured data)
	DataType DataType `podos:"data_type"` // Bitmap indicating data format/compression
	MimeType string   `podos:"mime"`      // MIME type (e.g., "application/json", "text/plain")
	DataSize int      `podos:"_datasize"` // Data size in bytes
}

// =============================================================================
// NEURAL MEMORY FIELDS - Evolutionary Neural Memory Actor operations
// =============================================================================

// NeuralMemoryFields groups all Evolutionary Neural Memory Actor-specific operations.
// Set only the sub-struct relevant to your Intent; others should be nil.
type NeuralMemoryFields struct {
	// Intent-specific options (set one based on Intent)
	GetEvent         *GetEventOptions         // Options for GetEvent intent (use GetTags=true to retrieve tags)
	GetEventsForTags *GetEventsForTagsOptions // Options for GetEventsForTags intent

	// Search configuration
	Search *SearchOptions // Programmable search configuration

	// Link operations
	Link       *LinkFields          // Single link operation
	BatchLinks []BatchLinkEventSpec // Batch link operations
	Unlink     *LinkFields          // Single unlink operation

	// Tags for storage operations
	Tags        TagList          // Tags to store with an event
	BatchEvents []BatchEventSpec // Batch event storage
}

// GetEventOptions contains options for the GetEvent intent.
// Retrieves a single Event Object by ID or UniqueId.
type GetEventOptions struct {
	SendData          bool    `podos:"send_data"`           // Return payload data with MIME type in the Response payload section
	LocalIdOnly       bool    `podos:"local_id_only"`       // Return only local ID
	TagFormat         NullInt `podos:"tag_format"`          // Tag output format (0 or 1)
	RequestFormat     int     `podos:"request_format"`      // Output format (use 0 as default)
	FirstLink         int     `podos:"first_link"`          // First link index to retrieve
	LinkCount         int     `podos:"link_count"`          // Number of links to return
	GetTags           bool    `podos:"get_tags"`            // Return tags for event
	GetLinks          bool    `podos:"get_links"`           // Send link information in the payload. Takes precedence over the SendData setting.
	GetLinkTags       bool    `podos:"get_link_tags"`       // Return tags for links
	GetTargetTags     bool    `podos:"get_target_tags"`     // Return tags for link targets
	EventFacetFilter  string  `podos:"event_facet_filter"`  // Filter event tags by prefix
	LinkFacetFilter   string  `podos:"link_facet_filter"`   // Filter link tags by prefix
	TargetFacetFilter string  `podos:"target_facet_filter"` // Filter target tags by prefix
	CategoryFilter    string  `podos:"category_filter"`     // Filter by link category
	TagFilter         string  `podos:"tag_filter"`          // Regex filter for tags
}

// GetEventsForTagsOptions contains options for the GetEventsForTags intent.
// Searches for events matching tag patterns.
type GetEventsForTagsOptions struct {
	EventPattern        string `podos:"event"`                    // Event key filter (FASTPATTERN)
	EventPatternHigh    string `podos:"event_high"`               // Event key filter high range
	IncludeBriefHits    bool   `podos:"include_brief_hits"`       // Include only event ID and unique ID
	GetAllData          bool   `podos:"get_all_data"`             // Get all tag and link data for all matching events, but disable the output of statistics for individual matching terms (equivalent to include_tag_stats=N)
	FirstLink           int    `podos:"first_link"`               // First link to retrieve
	LinkCount           int    `podos:"link_count"`               // Number of links to retrieve
	EventsPerMessage    int    `podos:"events_per_message"`       // Events per reply message
	StartResult         int    `podos:"start_result"`             // Paging: first result index
	EndResult           int    `podos:"end_result"`               // Paging: last result index
	MinEventHits        int    `podos:"min_event_hits"`           // Minimum tag matches required
	CountOnly           bool   `podos:"count_only"`               // Return only match count
	GetMatchLinks       bool   `podos:"get_match_links"`          // Include the number of links associated with a matching event object, filtered by “link_category” if present.
	CountMatchLinks     bool   `podos:"count_match_links"`        // Return total links per event
	GetLinkTags         bool   `podos:"get_link_tags"`            // Return tags for links
	GetTargetTags       bool   `podos:"get_target_tags"`          // Return tags for link targets
	LinkTagFilter       string `podos:"link_tag_pattern"`         // If present, tags associated with a link event object attached to a matching event will be filtered according to the regular expression described by the header field.
	LinkedEventsFilter  string `podos:"linked_events_tag_filter"` // Regex filter for target tags
	LinkCategory        string `podos:"link_category"`            // Restrict any link results to the category name matching the string.This includes link output and/or link counts.
	Owner               string `podos:"owner"`                    // If present, all links and tags are filtered such that only data owned by the specified event object will be returned.
	OwnerUniqueID       string `podos:"owner_unique_id"`          // If present, all links and tags are filtered such that only data owned by the specified event object (by unique ID) will be returned.
	GetEventObjectCount bool   `podos:"get_eo_count"`             // Special flag requesting the total number of event objects in the database, regardless of type. No other operation is performed. The return message header will contain: event_count=<N> where N is a numeric value.

	// Search configuration (shared with SearchOptions)
	BufferResults      bool   `podos:"buffer_results"`        // Y: Send all results in a single message, using the payload section of the message, N: Send results in a series of individual result messages
	IncludeTagStats    bool   `podos:"include_tag_stats"`     // Y: Includes statistics for each tag value that resulted in a match hit.
	InvertHitTagFilter bool   `podos:"invert_hit_tag_filter"` // Invert the hit tag filter
	HitTagFilter       string `podos:"hit_tag_filter"`        // Filter for result tags
	BufferFormat       string `podos:"buffer_format"`         // Output format: 0 = format a, 1 = format b
}

// SearchOptions contains programmable search configuration.
type SearchOptions struct {
	Clause             string // Search clause specification
	Parameters         string // Search parameters
	BufferResults      bool   // Buffer all results in single reply
	IncludeTagStats    bool   // Include tag statistics
	InvertHitTagFilter bool   // Invert the hit tag filter
	HitTagFilter       string // Filter for result tags
	BufferFormat       string // Output format
}

// LinkFields contains fields for link operations between events.
type LinkFields struct {
	UniqueId          string         `podos:"unique_id"` // Developer-provided unique ID for the event
	Id                string         `podos:"event_id"`  // AIP-generated unique ID with time and location; must be ASCII encoded
	LocalId           string         // Local machine ID for the event
	Owner             string         `podos:"owner"`     // Owner ID; indicates what entity created the event (default is "$sys")
	Timestamp         string         `podos:"timestamp"` // Event timestamp POSIX timestamp in microseconds; formatted as a string with 6 decimal places with + or - sign relative to January 1, 1970 00:00:00 UTC
	DateTime          DateTimeObject // Event date/time object
	Location          string         `podos:"loc"`             // Location specification (e.g., "TERRA|47.6|-122.5")
	LocationSeparator string         `podos:"loc_delim"`       // Location segment delimiter (default "|")
	EventA            string         `podos:"event_id_a"`      // required field if UniqueIdA is not provided
	EventB            string         `podos:"event_id_b"`      // required field if UniqueIdB is not provided
	UniqueIdA         string         `podos:"unique_id_a"`     // required field if EventA is not provided
	UniqueIdB         string         `podos:"unique_id_b"`     // required field if EventB is not provided
	StrengthA         float64        `podos:"strength_a"`      // Link strength A->B, required
	StrengthB         float64        `podos:"strength_b"`      // Link strength B->A, required
	Category          string         `podos:"category"`        // Link category, required
	Type              string         `podos:"type"`            // Developer-defined event type string
	OwnerUniqueID     string         `podos:"owner_unique_id"` // Owner unique ID; required field if OwnerID is not provided. This is logically different from the Event owner Ids.
	OwnerID           string         `podos:"owner_event_id"`  // Owner ID; required field if OwnerUniqueID is not provided. This is logically different from the Event owner Ids.
	Tags              []TagOutput    `podos:"tags"`            // Tags for this link; populated from _linktag records
	TargetTags        []TagOutput    `podos:"target_tags"`     // Tags describing the target event; populated from _targettag records; this is a convenience field for the developer to access the target tags without an additional call.
	Status            string         `podos:"_status"`         // Status of the link; used in StoreBatchLinks response
	Message           string         `podos:"_msg"`            // Message of the link; used in StoreBatchLinks response
}

// =============================================================================
// RESPONSE FIELDS - Response-only data populated by decoder
// =============================================================================

// ResponseFields contains data populated when decoding response messages.
// These fields are never set by the caller; they are filled by DecodeMessage.
type ResponseFields struct {
	Status              string         // Processing status: "OK" or "ERROR"
	Message             string         // Status description or error message
	TagCount            int            // Sum of Number of tags in response; used in Get and StoreEvent responses
	LinkCount           int            // Sum of Total number of links found; used in Get and GetEventsForTags responses
	LinkId              string         // Link ID returned by LinkEventResponse (link_event header field)
	DateTime            DateTimeObject // Parsed event datetime
	TotalEvents         int            // Total number of events found or stored
	ReturnedEvents      int            // Number of events returned in response
	StartResult         int            // Paging: first result index, -1 is not set
	EndResult           int            // Paging: last result index, -1 is not set
	StorageErrorCount   int            // Number of errors encountered during storage operations
	StorageSuccessCount int            // Number of successfully stored events

	// Batch-specific response fields
	EventRecords              []EventFields             // Parsed event results; used in GetEventsForTags and GetEvent responses
	StoreLinkBatchEventRecord StoreLinkBatchEventRecord // Parsed link event results; used in LinkEventBatch response
	StoreBatchEventRecord     StoreBatchEventRecord     // Parsed event results; used in StoreBatchEvents response

	// GetEventsForTags-specific response fields
	MatchTermCount int  // Number of different matching tag values; used in GetEventsForTags response
	IsBuffered     bool // Whether response is buffered; used in GetEventsForTags response

	// Brief hits response fields (for include_brief_hits=Y)
	BriefHits []BriefHitRecord // Brief hit records when include_brief_hits=Yx
}

type StoreBatchEventRecord struct {
	Status       string        `podos:"_status"`
	Message      string        `podos:"_msg"`
	EventCount   int           `podos:"_count"` // Total number of Events stored.
	EventResults []EventFields // Event results; used in StoreBatchEvents response
}

type StoreLinkBatchEventRecord struct {
	Status                 string       `podos:"_status"`
	Message                string       `podos:"_msg"`
	TotalLinkRequestsFound int          `podos:"_total_link_requests_found"`
	LinksOk                int          `podos:"_links_ok"`
	LinksWithErrors        int          `podos:"_links_with_errors"`
	LinkResults            []LinkFields // Link results; used in StoreBatchLinks response
}

// BriefHitRecord represents a brief hit result from GetEventsForTags with include_brief_hits=Y
type BriefHitRecord struct {
	EventId   string // _brief_hit field value - the event ID
	TotalHits int    // _hits field value - total number of search term match hits
}

// =============================================================================
// MESSAGE - Composed Actor message
// =============================================================================

// Message represents an Actor message using composition for clarity.
// The Envelope is embedded for convenient access to core routing fields.
//
// However, it is important for the developer to understand that the Message struct represents
// two use cases: (1) sending a message to an Actor, and (2) processing a response message from an Actor.
// The NeuralMemory field is a special case to ease the developer's use of the Evolutionary Neural Memory Database Actor that is natively part of AIP and Pod-OS.
//
// Sending a message to an Actor:
// The Event, NeuralMemory (if applicable), and Payload fields are used to send a message to an Actor.
// The Event field is used to identify the event to be stored or retrieved.
// The NeuralMemory field (if applicable) is used to specify the operation to be performed on the event.
// The Payload field is used to send data to the Actor.
//	msg := &message.Message{
//	    Envelope: message.Envelope{
//	        To:         "mem@zeroth.example.com",
//	        From:       "MyClient@zeroth.example.com",
//	        Intent:     message.IntentType.GetEvent,
//	        ClientName: "MyClient",
//	        MessageId:  uuid.New().String(),
//	    },
//	    Event: &message.EventFields{
//	        Id: "2024.01.15...",
//	    },
//	    NeuralMemory: &message.NeuralMemoryFields{
//	        GetEvent: &message.GetEventOptions{
//	            SendData: true,
//	            GetTags:  true,
//	        },
//	    },
//	}
//
// Processing a response message from an Actor:
// The Event (capturing that a Response event occurred) and Response fields are used to process a response message from an Actor.
// The Event field is used to capture that a Response event occurred.
// The Response field is used to process the response data from the Actor.
//	msg := &message.Message{
//	    Envelope: message.Envelope{
//	        To:         "mem@zeroth.example.com",
//	        From:       "MyClient@zeroth.example.com",
//	        Intent:     message.IntentType.GetEvent,
//	        ClientName: "MyClient",
//	        MessageId:  "msg-12345-abcde",
//	    },
//	    Event: &message.EventFields{
//	        Id: "2024.01.15.14.30.45.123456@actor1|location1|segment1",
//	        UniqueId: "user-provided-unique-id-123",
//	        Owner: "$sys",
//	        Timestamp: "1705327845123456",
//	        Location: "TERRA|47.619463|-122.518691",
//	        LocationSeparator: "|",
//	        Type: "user_action",
//	    },
//	    Response: &message.ResponseFields{
//	        Status: "OK",
//	        Message: "Event retrieved successfully",
//	        EventResults: []message.EventFields{
//	            {
//	                Id: "2024.01.15.14.30.45.123456@actor1|location1|segment1",
//	                UniqueId: "user-provided-unique-id-123",
//	                Owner: "2024.01.15.14.30.45.123456@actor1|location1|segment0",
//	                Type: "user_action",
//	            },
//	        },
//	    },
//	}

type Message struct {
	Envelope // Embedded core routing fields (To, From, Intent, ClientName, MessageId, Passcode)

	// Event metadata (nil for non-event operations)
	Event *EventFields

	// Payload data (nil if no payload); used to send data to the Actor.
	Payload *PayloadFields

	// Evolutionary Neural Memory Actor operations (nil for Gateway-only messages)
	NeuralMemory *NeuralMemoryFields

	// Response data (populated by decoder, nil for requests)
	Response *ResponseFields

	// PublicKey for encryption operations
	PublicKey crypto.PublicKey
}

// =============================================================================
// HELPER METHODS - Convenience accessors for Message
// =============================================================================

// GetEventOpts returns GetEvent options or nil if not set.
func (m *Message) GetEventOpts() *GetEventOptions {
	if m.NeuralMemory != nil {
		return m.NeuralMemory.GetEvent
	}
	return nil
}

// GetEventsForTagsOpts returns GetEventsForTags options or nil if not set.
func (m *Message) GetEventsForTagsOpts() *GetEventsForTagsOptions {
	if m.NeuralMemory != nil {
		return m.NeuralMemory.GetEventsForTags
	}
	return nil
}

// EventId returns the Event.Id or empty string if Event is nil.
func (m *Message) EventId() string {
	if m.Event != nil {
		return m.Event.Id
	}
	return ""
}

// EventUniqueId returns the Event.UniqueId or empty string if Event is nil.
func (m *Message) EventUniqueId() string {
	if m.Event != nil {
		return m.Event.UniqueId
	}
	return ""
}

// PayloadData returns the Payload.Data or nil if Payload is nil.
func (m *Message) PayloadData() any {
	if m.Payload != nil {
		return m.Payload.Data
	}
	return nil
}

// PayloadMimeType returns the Payload.MimeType or empty string if Payload is nil.
func (m *Message) PayloadMimeType() string {
	if m.Payload != nil {
		return m.Payload.MimeType
	}
	return ""
}

// ProcessingStatus returns the Response.Status or empty string if Response is nil.
func (m *Message) ProcessingStatus() string {
	if m.Response != nil {
		return m.Response.Status
	}
	return ""
}

// ProcessingMessage returns the Response.Message or empty string if Response is nil.
func (m *Message) ProcessingMessage() string {
	if m.Response != nil {
		return m.Response.Message
	}
	return ""
}

// Tags returns the NeuralMemory.Tags or nil if NeuralMemory is nil.
func (m *Message) Tags() TagList {
	if m.NeuralMemory != nil {
		return m.NeuralMemory.Tags
	}
	return nil
}

// Link returns the NeuralMemory.Link or nil if NeuralMemory is nil.
func (m *Message) Link() *LinkFields {
	if m.NeuralMemory != nil {
		return m.NeuralMemory.Link
	}
	return nil
}

// =============================================================================
// BATCH TYPES - For batch operations
// =============================================================================

// BatchEventSpec represents a single event specification for batch storage.
type BatchEventSpec struct {
	Event EventFields // Event metadata
	Tags  TagList     // Tags to store with the event
}

// BatchLinkEventSpec represents a single link specification for batch linking.
type BatchLinkEventSpec struct {
	Event EventFields // Event metadata for the link event
	Link  LinkFields  // Link specification
}

// =============================================================================
// TAG TYPES
// =============================================================================

// Tag represents a piece of important data for an Event Object.
// This is a Facet construction extending tagvalue into key/value structure.
type Tag struct {
	Frequency     int    // Count of occurrences
	Key           string // Tag key/category
	Value         any    // Supports string, int, float64, bool, map, slice, JSON objects
	Timestamp     string `podos:"timestamp"` // Event timestamp POSIX timestamp in microseconds; formatted as a string with 6 decimal places with + or - sign relative to January 1, 1970 00:00:00 UTC
	Id            string // Tag's Event Object ID
	Owner         string // Tag owner Event Object ID
	OwnerUniqueID string // Tag owner unique identifier
}

// StringValue returns the Value as a string.
func (t Tag) StringValue() (string, bool) {
	if s, ok := t.Value.(string); ok {
		return s, true
	}
	if t.Value == nil {
		return "", false
	}
	return fmt.Sprintf("%v", t.Value), false
}

// IntValue returns the Value as an int.
func (t Tag) IntValue() (int, bool) {
	switch v := t.Value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// FloatValue returns the Value as a float64.
func (t Tag) FloatValue() (float64, bool) {
	switch v := t.Value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// BoolValue returns the Value as a bool.
func (t Tag) BoolValue() (bool, bool) {
	if b, ok := t.Value.(bool); ok {
		return b, true
	}
	return false, false
}

// JSONValue unmarshals the Value into the provided target.
func (t Tag) JSONValue(target any) error {
	if t.Value == nil {
		return fmt.Errorf("tag value is nil")
	}

	if str, ok := t.Value.(string); ok {
		return json.Unmarshal([]byte(str), target)
	}

	jsonBytes, err := json.Marshal(t.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal tag value: %w", err)
	}

	return json.Unmarshal(jsonBytes, target)
}

// TagList is a list of Tags.
type TagList []Tag

// TagOutputList is deprecated, use TagOutput directly.
type TagOutputList TagOutput

// TagOutput represents a parsed tag from response payload.
type TagOutput struct {
	Frequency   int
	Category    string
	Key         string
	Value       string
	Owner       string
	Timestamp   string
	TargetTagId string // ID of the target tag; used to identify the target tag in the response.
}

// =============================================================================
// SEARCH TYPES
// =============================================================================

// SearchProgram represents a search program configuration.
type SearchProgram struct {
	SearchClause       []any
	SearchParameters   string
	SearchResults      string
	BufferResults      bool
	IncludeTagStats    bool
	InvertHitTagFilter bool
	HitTagFilter       string
}

// =============================================================================
// CONFIGURATION TYPES
// =============================================================================

// PodOsConfiguration represents Pod-OS configuration.
type PodOsConfiguration struct {
	DomainName        string              // Client's domain name (required)
	GatewayDefinition []GatewayDefinition // List of Gateways to be contained by the Pod-OS configuration.
	InstanceName      string              // The name of the instance of the Pod-OS configuration.
}

// GatewayDefinition represents a gateway/actor definition.
type GatewayDefinition struct {
	Name                   string              `podos:"name"`                  // Gateway name (required)
	Port                   int                 `podos:"port"`                  // Gateway port (required); default is 62312
	Logfile                string              `podos:"logfile"`               // Gateway log file (required); default is /var/log/pod-os/pod_gateway_<version>_log.txt
	LogLevel               int                 `podos:"log_level"`             // Gateway log level (required); lowest is 0, highest is 5. The greater the value, the more verbose the logging where 5 includes debugging information.
	DefaultService         string              `podos:"default_service"`       // Name of the Actor to receive messages if no other Actors are defined.
	DefaultUpstream        string              `podos:"default_upstream"`      // The name of the Actor to which all messages that cannot be locally routed are to be sent. This must always be a local Actor name, not a universal service name such as actor@gateway.domain.
	Throttle               int                 `podos:"throttle"`              // Delay in milliseconds. The Gateway accepts connections on a TCP socket from services and clients, and uses non-blocking I/O so that message queues can be updated. Unless a throttle value is given, the agent will constantly check for new connections and message queue changes, which may negatively affect CPU usage.
	LogToConsole           string              `podos:"log_to_console"`        // If set to “1”, then the logging is done to the console as well as the log file.
	AllowShutdownFrom      string              `podos:"allow_shutdown_from"`   // Name of the Actor which is allowed to request a HALT (clean up and shut down task). This must be a fully qualified Actor name in the format actor@gateway.domain
	DefaultIdleTimeout     int                 `podos:"default_idle_timeout"`  // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout.
	ActorStartList         string              `podos:"service_list"`          // The list of Actor names, separated by vertical bars, that are to be started when the Gateway starts.
	AutoShutdownTimeout    int                 `podos:"auto_shutdown_timeout"` // Number of seconds of inactivity before automatic shutdown of the Gateway.
	NeuralMemoryExecutable string              `podos:"pod_db_command"`        // Full path to the pod_db database handler executable file
	PeerExecutable         string              `podos:"pod_peer_command"`      // Full path to the peer executable file
	ShellActors            []ShellActor        // List of Shell Actors to be contained by the Gateway.
	NeuralMemoryActors     []NeuralMemoryActor // List of Evolutionary Neural Memory Actors to be contained by the Gateway.
	PeerActors             []PeerActor         // List of Peer Actors to be contained by the Gateway.
	ScriptActors           []ScriptActor       // List of Script Actors to be contained by the Gateway.
	MailboxActors          []MailboxActor      // List of Mailbox Actors to be contained by the Gateway.
	SocketActors           []SocketActor       // List of Socket Actors to be contained by the Gateway.
	RouterActors           []RouterActor       // List of Router Actors to be contained by the Gateway.
	Namespace              string              // Namespace of the Gateway.
	Domain                 string              // Domain of the Gateway.
}

// ShellActor represents a shell Actor
// The SHELL Actor is used to define an Actor that is a connection to an outside application, such as a command line utility or a web service.
type ShellActor struct {
	Name           string              `podos:"myname"`          // Name of the shell actor; passed to the shell on startup (required).
	Command        string              `podos:"command"`         // The fully qualified path name of the executable to be run as the Actor (required).
	Arguments      ShellActorArguments `podos:"args"`            // Command line arguments, separated with vertical bars. (required)
	Type           string              `podos:"type"`            // Type is always "SHELL" (required)
	AutoStart      string              `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart    string              `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int                 `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	StreamMessages string              `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

}
type ShellActorArguments struct {
	GatewayName      string `podos:"agent"`     // Name of the gateway where the Actor is hosted. (required)
	Host             string `podos:"host"`      // Fully-qualified hostname on which the Actor resides. (required)
	Port             int    `podos:"port"`      // Port on which the Actor listens for connections. (required)
	GatewayProcessId int64  `podos:"agent_pid"` // The process ID of the local Gateway process.
	Name             string `podos:"myname"`    // Name of the shell actor; passed to the executable upon startup (required).
}

// NeuralMemoryActor represents a neural memory actor/service.
type NeuralMemoryActor struct {
	Name           string `podos:"myname"`          // Name of the neural memory actor; passed to the database handler on startup (required)
	Type           string `podos:"type"`            // Type is always "POD_DB" (required)
	PodDbCommand   string `podos:"pod_db_command"`  // Full path to the pod_db database handler executable file (required)
	DbPath         string `podos:"dbpath"`          // Fully qualified name of a path (not a file) where the database is stored. Default is /var/lib/pod-os/<actor_name>_db. (required)
	GatewayName    string `podos:"agent"`           // Name of the gateway where the Actor is hosted. (required)
	Host           string `podos:"host"`            // Fully-qualified hostname on which the Actor resides. (required)
	Settings       string `podos:"settings"`        // Fully qualified name of a .ini file where the database settings are stored. (required)
	Autostart      string `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list.
	DemandStart    string `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int    `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	Throttle       int    `podos:"throttle"`        // Delay in milliseconds. The Actor accepts connections on a TCP socket from Gateways and uses non-blocking I/O so that message queues can be updated. Unless a throttle value is given, the Actor will constantly check for new connections and message queue changes, which may negatively affect CPU usage.
	StreamMessages string `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.
}

// PeerActor represents a peer actor.
// The PEER Actor is used to define a remote Gateway to which a connection will potentially be made. The
// connection is established by a helper program which is started in the same way as the database handler.
type PeerActor struct {
	Type             string `podos:"type"`         // Type is always "peer" (required)
	GatewayName      string `podos:"agent"`        // Name of the gateway where the Actor is hosted. (required)
	Host             string `podos:"host"`         // Fully-qualified hostname on which the Actor resides. (required)
	Port             int    `podos:"port"`         // Port on which the Actor listens for connections. (required)
	Name             string `podos:"myname"`       // Name of the shell actor; passed to the executable upon startup (required).
	GatewayProcessId int64  `podos:"agent_pid"`    // The process ID of the local Gateway process.
	TargetGateway    string `podos:"target_agent"` // The name of the gateway to which the Actor is connected. (required)
	TargetHost       string `podos:"target_host"`  // The fully-qualified-DNS-name hostname on which the target gateway resides. (required)
	TargetPort       int    `podos:"target_port"`  // The port on which the target gateway listens for connections. (required)
	TargetActorName  string `podos:"target_name"`  // The name of the Actor on the target gateway. (required)

	AutoStart      string `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart    string `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int    `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	StreamMessages string `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.
}

// ScriptActor represents a script actor.
// The Script Actor is used to define an Actor that is a connection to an
// external application that communicates with the Agent via a socket.
type ScriptActor struct {
	Type   string `podos:"type"`   // Type is always "script" (required)
	Name   string `podos:"myname"` // Name of the script actor; passed to the script on startup (required).
	Source string `podos:"source"` // Fully qualified name of a file where the script is stored. (required)

	AutoStart      string `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart    string `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int    `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	StreamMessages string `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.

}

// MailboxActor represents a mailbox actor.
// The MAILBOX Actor is used to store and retrieve messages, typically for purposes of backups or for
// client and peer connections that are not always available, or which cannot be started. Thus, a
// MAILBOX Actor may have the same name as another Actor, so long as that Actor is not another
// MAILBOX Actor.
type MailboxActor struct {
	Type     string `podos:"type"`     // Type is always "mailbox" (required)
	Name     string `podos:"myname"`   // Name of the mailbox actor; passed to the mailbox on startup (required).
	Filename string `podos:"filename"` // Fully qualified name of a file where the mailbox is stored. (required).

	AutoStart      string `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart    string `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int    `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	StreamMessages string `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.

}

// SocketActor represents a socket actor.
// The SOCKET Actor is used to define an Actor which is a connection to an external application that communicates with the Actor via a socket.
type SocketActor struct {
	Type             string `podos:"type"`      // Type is always "socket" (required)
	Name             string `podos:"myname"`    // The name of the service as passed to the service on startup(required)
	GatewayName      string `podos:"agent"`     // Name of the gateway where the Actor is hosted. (required)
	Host             string `podos:"host"`      // Fully-qualified hostname on which the Actor resides. (required)
	Port             int    `podos:"port"`      // Port on which the Actor listens for connections. (required)
	GatewayProcessId int64  `podos:"agent_pid"` // The process ID of the local Gateway process.

	AutoStart      string `podos:"autostart"`       // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart    string `podos:"demand_start"`    // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it. (required)
	IdleTimeout    int    `podos:"idle_timeout"`    // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout. (required)
	StreamMessages string `podos:"stream_messages"` // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor. (required)

	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.
}

// RouterActor represents a router actor.
type RouterActor struct {
	Name              string // Name of the router actor (required)
	Type              string `podos:"type"`                // Type is always "router" (required)
	AutoStart         string `podos:"autostart"`           // "Y" or "N" - Determines if the actor is to be started automatically on startup of the gateway. This is independent of the list of actors to be started; if this value is NO, then the actor will not be started even if in the startup list. (required)
	DemandStart       string `podos:"demand_start"`        // "Y" or "N" - Determines if the Actor is to be started if a message is sent to it.
	StreamMessages    string `podos:"stream_messages"`     // "Y" or "N" - If set to YES, messages will be sent to the Actor as they are received. Otherwise, messages will be sent one at a time when requested by the Actor.
	IdleTimeout       int    `podos:"idle_timeout"`        // Number of seconds of inactivity before automatic shutdown of the Actor. Negative one indicates no idle timeout.
	AllowShutdownFrom string `podos:"allow_shutdown_from"` // The name of an Actor which is allowed to halt the Actor. This must be formatted as actor@gateway
	PreserveSend      string `podos:"preserve_send"`       // "Y" or "N" - If YES, the message send queue for the Actor will be preserved on Actor shutdown.
	PreserveReceive   string `podos:"preserve_recv"`       // "Y" or "N" - If YES, the message receive queue for the Actor will be preserved on Actor shutdown.
	SavePath          string `podos:"qsave_path"`          // Fully qualified name of a path (not a file) where queue contents are to be saved.
	KeepAlive         int    `podos:"keepalive"`           // If not zero, a keepalive message will be sent to the Actor every N seconds, where N is the integer value specified.
	BusyLevel         int    `podos:"busy_level"`          // If set, the system will send a BUSY message reply for all messages sent to an Actor which has at least N messages waiting in the inbound or outbound queues, where N is the integer specified.

}

type RouteDefinition struct {
	MessageType                string `podos:"message_type"`                  // The type of message to which the route applies; only accept strings found in pod-os-go-client/message/intents.go IntentType.RoutingMessageType (e.g., "DB", "ECHO", "START", "STATUS", "REQUEST", "ID", "DISCONNECT", "NEXT", "NO_SEND", "STREAM_OFF", "STREAM_ON", "RECORD", "BATCH_START", "BATCH_END", "USER", "ANY", "USERONLY") (required)
	MessageNeuralMemoryCommand string `podos:"message_neural_memory_command"` // The database command to match; only accept strings found in pod-os-go-client/message/intents.go IntentType.NeuralMemoryCommand (e.g., "store", "store_batch", "tag_store_batch", "get", "events_for_tag", "tags_for_event", "link", "unlink", "link_batch")
	TestDataSource             string `podos:"test_data_source"`              // The data source to test; the source of the data to be used for comparison. Usually a part of a Message.
	TestType                   string `podos:"test_type"`                     // The type of test to perform; only accept strings found in pod-os-go-client/message/intents.go RoutingMessageTestType (e.g., "NONE", "EQ", "NE", "LT", "LE", "GT", "GE", "RANGE", "EXCL", "REGEXP", "NUMEQ", "NUMNE", "NUMLT", "NUMLE", "NUMGT", "NUMGE", "NUMRANGE", "NUMEXCL")
	TestPattern                string `podos:"test_pattern"`                  // The pattern to test; only used for REGEXP and NUMREGEXP tests.
	TestLowValue               string `podos:"test_low_value"`                // The low value of a comparison range.
	TestHighValue              string `podos:"test_high_value"`               // The high value of a comparison range.
	TestValueSource            string `podos:"test_value_source"`             // The source for the data against which the test data is to be compared. The default equation is “if test_data_source (operator) test_value_source”
	TestValue                  string `podos:"test_value"`                    // A literal value to be compared, if the test_value_source is “$test_value”.
	ActionType                 string `podos:"action_type"`                   // The action to take on successful matching of the message. Only accept strings found in pod-os-go-client/message/intents.go RoutingMessageActionType (e.g., "NONE", "ECHO", "START", "STATUS", "REQUEST", "ID", "DISCONNECT", "NEXT", "NO_SEND", "STREAM_OFF", "STREAM_ON", "RECORD", "BATCH_START", "BATCH_END", "USER", "ANY", "USERONLY")
}
