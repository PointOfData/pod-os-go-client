package message

// Intent represents an intent type for the Pod-OS system
type Intent struct {
	Name                string // Friendly Pod-OS name for the intent.
	RoutingMessageType  string // Defines message_type name for routing message functions.
	NeuralMemoryCommand string // Defines the command to send to the Neural Memory Actor.
	MessageType         int    // The message type integer set in the message header.
}

// IntentTypes var defines the intent types for the PodOs system.
// TODO: refactor into a logical information architecture that is namespaced by dot.
var IntentType = newIntentTypes()

// intentTypes is a struct that defines the intent types for the PodOs system. Struct is not exported.
type intentTypes struct {
	StoreEvent           Intent // StoreEvent is used to store a single event object in a Neural Memory database.
	StoreBatchEvents     Intent // StoreBatchEvents is used to store a batch of event objects (including tags) in a Neural Memory database.
	StoreBatchTags       Intent // StoreBatchTags is used to store a batch of tag objects (associated with an event object) in a Neural Memory database.
	GetEvent             Intent // GetEvent is used to retrieve a single event object from a Neural Memory database. Use GetTags=true to retrieve tags for the event.
	GetEventsForTags     Intent // GetEventsForTags is used to retrieve a batch of event objects from a Neural Memory database by searching for Tags.
	LinkEvent            Intent // LinkEvent is used to link two event objects in a Neural Memory database. It is also an Event Object itself.
	UnlinkEvent          Intent // UnlinkEvent is used to unlink two event objects in a Neural Memory database.
	StoreBatchLinks      Intent // StoreBatchLinks is used to store a batch of link objects in a Neural Memory database.
	ActorEcho            Intent // ActorEcho is used to echo a message from the Actor to the client.
	ActorHalt            Intent // ActorHalt is used to halt the Actor.
	ActorStart           Intent // ActorStart is used to start an Actor.
	GatewayStatus        Intent // GatewayStatus is used to get the status of a Gateway. It is never sent to an Actor.
	ActorRequest         Intent // ActorRequest is used to request a status from an Actor.
	GatewayId            Intent // GatewayID is used by a client to identify the connection (required for all messages and connections) and establish client authentication and authorization.
	GatewayDisconnect    Intent // GatewayDisconnect is used to disconnect a client from a Gateway.
	GatewaySendNext      Intent // ActorSendNext is used to send the next message from the Actor to the client.
	GatewayNoSend        Intent // ActorNoSend is used to tell the Actor to not send the next message to the client.
	GatewayStreamOff     Intent // ActorStreamOff is used to turn off the streaming of messages from the Actor to the client.
	GatewayStreamOn      Intent // ActorStreamOn is used to turn on the streaming of messages from the Actor to the client.
	ActorRecord          Intent // ActorRecord is used to record a message from the client to the Actor.
	GatewayBatchStart    Intent // ActorBatchStart is used to start a batch of messages from the client to the Actor.
	GatewayBatchEnd      Intent // ActorBatchEnd is used to end a batch of messages from the client to the Actor.
	ActorUser            Intent // User intent is used for user-defined intents and messages. Any value at or above 65536 is a user intent. The numbering system is arbitrary and defined by the developer.
	RouteAnyMessage      Intent // RouteAnyMessage is used to match any kind of message.
	RouteUserOnlyMessage Intent // RouteUserOnlyMessage is used to match only user-level messages.
	ActorReport          Intent // ActorReport is used to report the status of an Actor.
}

func newIntentTypes() *intentTypes {
	return &intentTypes{
		StoreEvent:           Intent{Name: "StoreEvent", NeuralMemoryCommand: "store", MessageType: 1000, RoutingMessageType: "DB"},
		StoreBatchEvents:     Intent{Name: "StoreBatchEvents", NeuralMemoryCommand: "store_batch", MessageType: 1000, RoutingMessageType: "DB"},
		StoreBatchTags:       Intent{Name: "StoreBatchTags", NeuralMemoryCommand: "tag_store_batch", MessageType: 1000, RoutingMessageType: "DB"},
		GetEvent:             Intent{Name: "GetEvent", NeuralMemoryCommand: "get", MessageType: 1000, RoutingMessageType: "DB"},
		GetEventsForTags:     Intent{Name: "GetEventsForTags", NeuralMemoryCommand: "events_for_tag", MessageType: 1000, RoutingMessageType: "DB"},
		LinkEvent:            Intent{Name: "LinkEvent", NeuralMemoryCommand: "link", MessageType: 1000, RoutingMessageType: "DB"},
		UnlinkEvent:          Intent{Name: "UnlinkEvent", NeuralMemoryCommand: "unlink", MessageType: 1000, RoutingMessageType: "DB"},
		StoreBatchLinks:      Intent{Name: "StoreBatchLinks", NeuralMemoryCommand: "link_batch", MessageType: 1000, RoutingMessageType: "DB"},
		ActorEcho:            Intent{Name: "ActorEcho", MessageType: 2, RoutingMessageType: "ECHO"},
		ActorHalt:            Intent{Name: "ActorHalt", MessageType: 99, RoutingMessageType: "HALT"},
		ActorStart:           Intent{Name: "ActorStart", MessageType: 1, RoutingMessageType: "START"},
		GatewayStatus:        Intent{Name: "GatewayStatus", MessageType: 3, RoutingMessageType: "STATUS"},
		ActorRequest:         Intent{Name: "ActorRequest", MessageType: 4, RoutingMessageType: "REQUEST"},
		GatewayId:            Intent{Name: "GatewayId", MessageType: 5, RoutingMessageType: "ID"},
		GatewayDisconnect:    Intent{Name: "GatewayDisconnect", MessageType: 6, RoutingMessageType: "DISCONNECT"},
		GatewaySendNext:      Intent{Name: "GatewaySendNext", MessageType: 7, RoutingMessageType: "NEXT"},
		GatewayNoSend:        Intent{Name: "GatewayNoSend", MessageType: 8, RoutingMessageType: "NO_SEND"},
		GatewayStreamOff:     Intent{Name: "GatewayStreamOff", MessageType: 9, RoutingMessageType: "STREAM_OFF"},
		GatewayStreamOn:      Intent{Name: "GatewayStreamOn", MessageType: 10, RoutingMessageType: "STREAM_ON"},
		ActorRecord:          Intent{Name: "ActorRecord", MessageType: 11, RoutingMessageType: "RECORD"},
		GatewayBatchStart:    Intent{Name: "GatewayBatchStart", MessageType: 12, RoutingMessageType: "BATCH_START"},
		GatewayBatchEnd:      Intent{Name: "GatewayBatchEnd", MessageType: 13, RoutingMessageType: "BATCH_END"},
		ActorReport:          Intent{Name: "ActorReport", MessageType: 19, RoutingMessageType: "REPORT"},
		ActorUser:            Intent{Name: "ActorUser", MessageType: 65536, RoutingMessageType: "USER"},
		RouteAnyMessage:      Intent{Name: "RouteAnyMessage", RoutingMessageType: "ANY"},
		RouteUserOnlyMessage: Intent{Name: "RouteUserOnlyMessage", RoutingMessageType: "USERONLY"},
	}
}

// commandToIntent maps NeuralMemoryCommand strings to their corresponding Intent.
// Used when decoding response messages to determine the intent from the _command header field.
var commandToIntent = map[string]Intent{
	"store":           IntentType.StoreEvent,
	"store_batch":     IntentType.StoreBatchEvents,
	"tag_store_batch": IntentType.StoreBatchTags,
	"get":             IntentType.GetEvent,
	"events_for_tag":  IntentType.GetEventsForTags,
	"link":            IntentType.LinkEvent,
	"unlink":          IntentType.UnlinkEvent,
	"link_batch":      IntentType.StoreBatchLinks,
}

// IntentFromCommand returns the Intent corresponding to the given command string.
// Returns the matching Intent and true if found, or an empty Intent and false if not found.
func IntentFromCommand(command string) (Intent, bool) {
	intent, ok := commandToIntent[command]
	return intent, ok
}

// IntentFromMessageType returns the Intent corresponding to the given messageType.
// Accepts either an int (MessageType) or a string (NeuralMemoryCommand like "store_batch").
// Also accepts pointer types (*int, *string) which will be dereferenced.
// Returns the matching Intent and true if found, or an empty Intent and false if not found.
func IntentFromMessageType(messageType any) (Intent, bool) {
	switch v := messageType.(type) {
	case int:
		return intentFromMessageTypeInt(v)
	case *int:
		if v == nil {
			return Intent{}, false
		}
		return intentFromMessageTypeInt(*v)
	case string:
		return IntentFromCommand(v)
	case *string:
		if v == nil {
			return Intent{}, false
		}
		return IntentFromCommand(*v)
	default:
		return Intent{}, false
	}
}

// intentFromMessageTypeInt returns the Intent corresponding to the given messageType integer.
func intentFromMessageTypeInt(messageType int) (Intent, bool) {
	allIntents := []Intent{
		IntentType.StoreEvent, IntentType.StoreBatchEvents, IntentType.StoreBatchTags,
		IntentType.GetEvent, IntentType.GetEventsForTags,
		IntentType.LinkEvent, IntentType.UnlinkEvent, IntentType.StoreBatchLinks,
		IntentType.ActorEcho, IntentType.ActorHalt, IntentType.ActorStart,
		IntentType.GatewayStatus, IntentType.ActorRequest, IntentType.GatewayId,
		IntentType.GatewayDisconnect, IntentType.GatewaySendNext, IntentType.GatewayNoSend,
		IntentType.GatewayStreamOff, IntentType.GatewayStreamOn, IntentType.ActorRecord,
		IntentType.GatewayBatchStart, IntentType.GatewayBatchEnd, IntentType.ActorReport, IntentType.ActorUser,
	}
	for _, intent := range allIntents {
		if intent.MessageType == messageType {
			return intent, true
		}
	}
	return Intent{}, false
}

// Routing Test Types
const (
	RoutingTestTypeNone                      = "NONE"   // No test performed. Used for routes where all matching message types are subjected to the same action.
	RoutingTestTypeEquals                    = "EQ"     // test_data_source = test_value_source
	RoutingTestTypeNotEquals                 = "NE"     // test_data_source != test_value_source
	RoutingTestTypeLessThan                  = "LT"     // test_data_source < test_value_source
	RoutingTestTypeLessThanOrEqual           = "LE"     // test_data_source <= test_value_source
	RoutingTestTypeGreaterThan               = "GT"     // test_data_source > test_value_source
	RoutingTestTypeGreaterThanOrEqual        = "GE"     // test_data_source >= test_value_source
	RoutingTestTypeRange                     = "range"  // test_data_source >= test_low && test_data_source <= test_high
	RoutingTestTypeExclude                   = "excl"   // test_data_source < test_low || test_data_source > test_high
	RoutingTestTypeRegex                     = "regexp" // test_data_source compared to regular expression in test_pattern
	RoutingTestTypeNumericEquals             = "#EQ"    // Numeric equality test; test_data_source = test_value_source
	RoutingTestTypeNumericNotEquals          = "#NE"    // Numeric non-equality test; test_data_source != test_value_source
	RoutingTestTypeNumericLessThan           = "#LT"    // Numeric less than test; test_data_source < test_value_source
	RoutingTestTypeNumericLessThanOrEqual    = "#LE"    // Numeric less than or equal test; test_data_source <= test_value_source
	RoutingTestTypeNumericGreaterThan        = "#GT"    // Numeric greater than test; test_data_source > test_value_source
	RoutingTestTypeNumericGreaterThanOrEqual = "#GE"    // Numeric greater than or equal test; test_data_source >= test_value_source
	RoutingTestTypeNumericRange              = "#RANGE" // Numeric range test; test_data_source >= test_low && test_data_source <= test_high
	RoutingTestTypeNumericExclude            = "#EXCL"  // Numeric exclusion test; test_data_source < test_low || test_data_source > test_high

)

// Routing Action Types
const (
	RoutingActionTypeNone      = "NONE"      // Do not do anything. This is most useful for performing compound matches on routes, where the next route specification is invoked on a match, but no action is taken with regard to the message itself.
	RoutingActionTypeRoute     = "ROUTE"     // Send the message to the destination(s) in the "dest" route field.
	RoutingActionTypeDiscard   = "DISCARD"   // Discard the message.
	RoutingActionTypeChange    = "CHANGE"    // Alter the original message as defined in the change_field route field. If there are no destinations defined, then alter the original message. Otherwise, send new messages to the named destinations with the altered data.
	RoutingActionTypeDuplicate = "DUPLICATE" // Duplicate the message and send to the named destinations without other alterations.
)
