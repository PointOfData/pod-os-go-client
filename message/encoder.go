package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// forceASCII forces the given string to be ASCII. This function omits invalid runes/characters.
func forceASCII(s string) string {
	rs := make([]rune, 0, len(s))
	for _, r := range s {
		if r <= 127 {
			rs = append(rs, r)
		}
	}
	return string(rs)
}

// SerializeTagValue serializes a Tag value of any type to its string representation for socket transmission.
// Supports: string, int types, uint types, float types, bool, []byte (base64), and complex types (JSON).
//
// Parameters: value any - The value to serialize
//
// Returns: string - The serialized string representation
func SerializeTagValue(value any) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.FormatInt(int64(v), 10)
	case int16:
		return strconv.FormatInt(int64(v), 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint:
		return strconv.FormatUint(uint64(v), 10)
	case uint8:
		return strconv.FormatUint(uint64(v), 10)
	case uint16:
		return strconv.FormatUint(uint64(v), 10)
	case uint32:
		return strconv.FormatUint(uint64(v), 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case []byte:
		return base64.StdEncoding.EncodeToString(v)
	default:
		// For complex types (maps, slices, structs), serialize as JSON
		if jsonBytes, err := json.Marshal(v); err == nil {
			return string(jsonBytes)
		}
		// Fallback to fmt.Sprintf for any other types
		return fmt.Sprintf("%v", v)
	}
}

// FormatBatchEventsPayload formats a slice of BatchEventSpec into the payload format required for StoreBatchEvents Intent.
// Each event is formatted as a tab-separated line using Pod-OS socket field names from struct tags.
// Events are joined with newlines (\n).
//
// Parameters: events []BatchEventSpec - The batch event specifications to format
//
// Returns: string - The formatted payload string ready for use in Message.PayloadData
func FormatBatchEventsPayload(events []BatchEventSpec) string {
	if len(events) == 0 {
		return ""
	}

	var result strings.Builder
	// Pre-allocate capacity for better performance
	result.Grow(len(events) * 200) // Estimate ~200 chars per event

	for i, spec := range events {
		if i > 0 {
			result.WriteByte('\n')
		}

		// Use reflection to read struct tags and format fields
		// This maintains separation between Go-idiomatic naming and Pod-OS socket naming
		fields := formatBatchEventSpecFields(spec)
		result.WriteString(strings.Join(fields, "\t"))

		// Append tags to each event line if present
		if len(spec.Tags) > 0 {
			tagString := formatTagsForBatchPayload(spec.Tags)
			result.WriteString(tagString)
		}
	}

	return result.String()
}

// FormatBatchLinkEventsPayload formats a slice of BatchLinkEventSpec into the payload format required for BatchLinkEvents Intent.
// Each event is formatted as a tab-separated line using Pod-OS socket field names from struct tags.
// Events are joined with newlines (\n).
//
// Parameters: events []BatchLinkEventSpec - The batch link event specifications to format
//
// Returns: string - The formatted payload string ready for use in Message.PayloadData
func FormatBatchLinkEventsPayload(events []BatchLinkEventSpec) string {
	if len(events) == 0 {
		return ""
	}

	var result strings.Builder
	result.Grow(len(events) * 200) // Estimate ~200 chars per event

	for i, event := range events {
		if i > 0 {
			result.WriteByte('\n')
		}

		fields := formatBatchLinkEventSpecFields(event)
		result.WriteString(strings.Join(fields, "\t"))
	}

	return result.String()
}

// formatBatchLinkEventSpecFields formats a BatchLinkEventSpec into Pod-OS socket format using struct tags
func formatBatchLinkEventSpecFields(spec BatchLinkEventSpec) []string {
	var fields []string

	// Format EventFields
	event := spec.Event
	v := reflect.ValueOf(event)
	t := reflect.TypeOf(event)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		socketName := field.Tag.Get(SocketFieldTag)
		if socketName == "" {
			continue
		}
		if value.IsZero() {
			continue
		}
		fieldValue := formatFieldValue(field.Type, value.Interface())
		if fieldValue != "" {
			fields = append(fields, socketName+"="+fieldValue)
		}
	}

	// Format LinkFields
	link := spec.Link
	vLink := reflect.ValueOf(link)
	tLink := reflect.TypeOf(link)

	for i := 0; i < tLink.NumField(); i++ {
		field := tLink.Field(i)
		value := vLink.Field(i)

		socketName := field.Tag.Get(SocketFieldTag)
		if socketName == "" {
			continue
		}
		if value.IsZero() {
			continue
		}
		fieldValue := formatFieldValue(field.Type, value.Interface())
		if fieldValue != "" {
			fields = append(fields, socketName+"="+fieldValue)
		}
	}

	return fields
}

// formatBatchEventSpecFields formats a BatchEventSpec into Pod-OS socket format using struct tags
func formatBatchEventSpecFields(spec BatchEventSpec) []string {
	var fields []string

	// Use reflection to iterate over EventFields
	event := spec.Event
	v := reflect.ValueOf(event)
	t := reflect.TypeOf(event)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Check if field has podos tag
		socketName := field.Tag.Get(SocketFieldTag)
		if socketName == "" {
			continue
		}

		// Skip zero values for optional fields
		if value.IsZero() {
			continue
		}

		// Format as socketname=value
		fieldValue := formatFieldValue(field.Type, value.Interface())
		if fieldValue != "" {
			fields = append(fields, socketName+"="+fieldValue)
		}
	}

	return fields
}

// formatFieldValue formats a field value to its string representation
func formatFieldValue(fieldType reflect.Type, value interface{}) string {
	// Handle string types
	if fieldType.Kind() == reflect.String {
		if s, ok := value.(string); ok {
			return s
		}
		return ""
	}

	// Fallback: convert to string
	return fmt.Sprintf("%v", value)
}

// FormatBatchTagsPayload formats a TagList into the payload format required for StoreBatchTags and UpdateBatchTags Intents.
// Each tag is formatted as a newline-terminated record: frequency=key=value
//
// Parameters: tags TagList - The tags to format
//
// Returns: string - The formatted payload string ready for use in Message.PayloadData
func FormatBatchTagsPayload(tags TagList) string {
	if len(tags) == 0 {
		return ""
	}

	var result strings.Builder
	result.Grow(len(tags) * 50) // Estimate ~50 chars per tag

	for i, tag := range tags {
		if i > 0 {
			result.WriteByte('\n')
		}
		// Format: frequency=key=value
		result.WriteString(fmt.Sprintf("%d=%s=%s", tag.Frequency, tag.Key, SerializeTagValue(tag.Value)))
	}

	return result.String()
}

// formatTagsForBatchPayload formats tags for batch event payload
// Tag Format: tag_0=freq:key=value <tab> tag_1=freq:key=value ...
// The tag Value is serialized using SerializeTagValue() to support any type.
func formatTagsForBatchPayload(tags TagList) string {
	if len(tags) == 0 {
		return ""
	}
	tagFields := make([]string, 0, len(tags))
	for i, tag := range tags {
		uniqueTagId := fmt.Sprintf("tag_%d", i)
		tagField := fmt.Sprintf("%s=%d:%s=%s", uniqueTagId, tag.Frequency, tag.Key, SerializeTagValue(tag.Value))
		tagFields = append(tagFields, tagField)
	}
	return "\t" + strings.Join(tagFields, "\t")
}

// EncodeMessage creates a SocketMessage struct from a Message struct.
//
// Parameters: message *Message. As the message can be large (up to 2GB payload), it is passed as a pointer.
// conversationUuid: A unique identifier for the conversation.
//
// Returns: *SocketMessage and error. Returns an EncodeError if encoding fails.
func EncodeMessage(msg *Message, conversationUuid string) (*SocketMessage, error) {
	// Validate message is not nil
	if msg == nil {
		return nil, NewEncodeError(ErrCodeEncodeNilMessage, "message cannot be nil")
	}

	// Get payload data from composition
	payloadData := msg.PayloadData()
	var payloadDataType DataType
	if msg.Payload != nil {
		payloadDataType = msg.Payload.DataType
	}

	// Enforce a hard upper bound on payload size using the shared maximum
	// message size. While headers and other fields also contribute to the
	// total message size, this guard prevents obviously too-large payloads
	// from being encoded.
	if payloadData != nil {
		// Check payload size for common types
		switch v := payloadData.(type) {
		case string:
			if int64(len(v)) > MaxMessageSizeBytes {
				return nil, EncodeErrorWithField(ErrCodeEncodePayloadTooLarge, fmt.Sprintf("payload size %d bytes exceeds maximum %d bytes", len(v), MaxMessageSizeBytes), "PayloadData")
			}
		case []byte:
			if int64(len(v)) > MaxMessageSizeBytes {
				return nil, EncodeErrorWithField(ErrCodeEncodePayloadTooLarge, fmt.Sprintf("payload size %d bytes exceeds maximum %d bytes", len(v), MaxMessageSizeBytes), "PayloadData")
			}
		case []BatchEventSpec:
			// Estimate size for batch events
			estimatedSize := len(v) * 500 // Rough estimate
			if int64(estimatedSize) > MaxMessageSizeBytes {
				return nil, EncodeErrorWithField(ErrCodeEncodePayloadTooLarge, fmt.Sprintf("estimated payload size %d bytes exceeds maximum %d bytes", estimatedSize, MaxMessageSizeBytes), "PayloadData")
			}
		}
	}

	// Construct Header.
	messageHeader := ConstructHeader(msg, msg.Intent, conversationUuid)

	// Verify the To address is valid. It should be in the format of <ClientName>@<GatewayName>.<Domain Name>
	if !strings.Contains(msg.To, "@") {
		return nil, NewEncodeError(ErrCodeEncodeInvalidToAddress, "To address is required and must be in the format of <ClientName>@<GatewayName>.<Domain Name>. The To address is: "+msg.To)
	}
	localTo := strings.Split(msg.To, "@")[0]
	if localTo == "" {
		return nil, NewEncodeError(ErrCodeEncodeInvalidActorName, "Actor is required and must be in the format of <ClientName>@<GatewayName>.<Domain Name>")
	}
	localGateway := strings.Split(msg.To, "@")[1]
	if localGateway == "" {
		return nil, NewEncodeError(ErrCodeEncodeInvalidGatewayName, "GatewayName is required and must be in the format of <ClientName>@<GatewayName>.<Domain Name>")
	}

	// From address is the same as the To address, but with the conversation UUID appended to account for a unique From and id:Name connection for each message.
	// if there is no From address, log an error and return an error. It should be in the format of <ClientName>@<GatewayName>
	if !strings.Contains(msg.From, "@") {
		return nil, NewEncodeError(ErrCodeEncodeInvalidFromAddress, "From address is required and must be in the format of <ClientName>@<GatewayName>.<Domain Name>. The To address is: "+msg.To)
	}
	if msg.From == "" {
		return nil, NewEncodeError(ErrCodeEncodeInvalidFromAddress, "From address is required and must be in the format of <ClientName>@<GatewayName>")
	}
	localFrom := strings.Split(msg.From, "@")[0]
	if localFrom == "" {
		return nil, NewEncodeError(ErrCodeEncodeInvalidFromAddress, "Client name, acting as an Actor, is required and must be in the format of <ClientName>@<GatewayName>")
	}
	localFromGateway := strings.Split(msg.From, "@")[1]
	if localFromGateway == "" {
		return nil, NewEncodeError(ErrCodeEncodeInvalidGatewayName, "GatewayName is required and must be in the format of <ClientName>@<GatewayName>")
	}

	// Construct dataBytes Var based on object Type, and Message type.
	// The following byte uses 1 allocation. We do this once to handle different types of data.

	// Initialize a 0 length byte slice.
	// PayloadData, when string or slice/array, is always maintained as utf-8. Otherwise, the binary representation is maintained.

	// Header fields: message_size, header_size, and data_size are hex-encoded.
	// Data Size is Hex-encoded.

	dataBytes := make([]byte, 0)
	if msg.Intent.Name == IntentType.GatewayId.Name {
		// Do nothing for now; dataBytes is empty.
	} else if msg.Intent.Name == IntentType.GatewayStreamOn.Name {
		// Do nothing for now; dataBytes is empty.
	} else if msg.Intent.Name == IntentType.StoreBatchEvents.Name {
		// Handle StoreBatchEvents Intent payload formatting
		if payloadData != nil {
			// Try type assertion for []BatchEventSpec first
			if events, ok := payloadData.([]BatchEventSpec); ok {
				// Convert []BatchEventSpec to formatted string
				formattedPayload := FormatBatchEventsPayload(events)
				dataBytes = append(make([]byte, 0, len(formattedPayload)), dataBytes...)
				dataBytes = append(dataBytes, []byte(formattedPayload)...) //encoding is utf-8
			} else if str, ok := payloadData.(string); ok {
				// Handle string payload for backward compatibility (manual formatting)
				dataBytes = append(make([]byte, 0, len(str)), dataBytes...)
				dataBytes = append(dataBytes, []byte(str)...) //encoding is utf-8
			} else if strSlice, ok := payloadData.([]string); ok {
				// Handle []string for backward compatibility
				bulkString := strings.Join(strSlice, "")
				dataBytes = append(make([]byte, 0, len(bulkString)), dataBytes...)
				dataBytes = append(dataBytes, []byte(bulkString)...) //encoding is utf-8
			}
		}
	} else if msg.Intent.Name == IntentType.StoreBatchLinks.Name {
		// Handle StoreBatchLinks Intent payload formatting
		if payloadData != nil {
			// Try type assertion for []BatchLinkEventSpec first
			if links, ok := payloadData.([]BatchLinkEventSpec); ok {
				// Convert []BatchLinkEventSpec to formatted string
				formattedPayload := FormatBatchLinkEventsPayload(links)
				dataBytes = append(make([]byte, 0, len(formattedPayload)), dataBytes...)
				dataBytes = append(dataBytes, []byte(formattedPayload)...) //encoding is utf-8
			} else if str, ok := payloadData.(string); ok {
				// Handle string payload for backward compatibility (manual formatting)
				dataBytes = append(make([]byte, 0, len(str)), dataBytes...)
				dataBytes = append(dataBytes, []byte(str)...) //encoding is utf-8
			}
		}
	} else if msg.Intent.Name == IntentType.StoreBatchTags.Name || msg.Intent.Name == IntentType.UpdateBatchTags.Name {
		// Handle StoreBatchTags and UpdateBatchTags Intent payload formatting
		// Payload format: newline-terminated records of frequency=key=value
		if payloadData != nil {
			// Try type assertion for TagList first
			if tags, ok := payloadData.(TagList); ok {
				formattedPayload := FormatBatchTagsPayload(tags)
				dataBytes = append(make([]byte, 0, len(formattedPayload)), dataBytes...)
				dataBytes = append(dataBytes, []byte(formattedPayload)...) //encoding is utf-8
			} else if tags, ok := payloadData.([]Tag); ok {
				formattedPayload := FormatBatchTagsPayload(tags)
				dataBytes = append(make([]byte, 0, len(formattedPayload)), dataBytes...)
				dataBytes = append(dataBytes, []byte(formattedPayload)...) //encoding is utf-8
			} else if str, ok := payloadData.(string); ok {
				// Handle string payload for backward compatibility (manual formatting)
				dataBytes = append(make([]byte, 0, len(str)), dataBytes...)
				dataBytes = append(dataBytes, []byte(str)...) //encoding is utf-8
			}
		} else if msg.NeuralMemory != nil && len(msg.NeuralMemory.Tags) > 0 {
			// Also check for tags in NeuralMemory.Tags
			formattedPayload := FormatBatchTagsPayload(msg.NeuralMemory.Tags)
			dataBytes = append(make([]byte, 0, len(formattedPayload)), dataBytes...)
			dataBytes = append(dataBytes, []byte(formattedPayload)...) //encoding is utf-8
		}
	} else {
		if payloadData != nil {
			switch reflect.TypeOf(payloadData).Kind() {
			// Construct dataBytes Var based on object Type, and Message type.
			case reflect.String:
				dataBytes = append(make([]byte, 0, len(payloadData.(string))), dataBytes...)
				dataBytes = append(dataBytes, []byte(payloadData.(string))...) //encoding is utf-8
			case reflect.Slice:
				bulkString := strings.Join(payloadData.([]string), "")
				dataBytes = append(make([]byte, 0, len(bulkString)), dataBytes...)
				dataBytes = append(dataBytes, []byte(bulkString)...) //encoding is utf-8
			case reflect.Array:
				bulkString := strings.Join(payloadData.([]string), "")
				dataBytes = append(make([]byte, 0, len(bulkString)), dataBytes...)
				dataBytes = append(dataBytes, []byte(bulkString)...) //encoding is utf-8
			default:
				l := len(payloadData.(string)) + 2
				dataBytes = dataBytes[:l]
				offset := 0
				copy(dataBytes, payloadData.(string))
				offset += len(payloadData.(string))
			}
		}
	}
	// Encode the PayloadData length using base 16, with lower-case letters for a-f, padded to 8 characters with leading zeros, leading x denotes hex.
	payloadDataLengthEncoded := "x" + fmt.Sprintf("%08x", len(dataBytes))

	// Encode the message.to length using base 16, with lower-case letters for a-f
	toLengthEncoded := "x" + fmt.Sprintf("%08x", len(msg.To))

	// Encode the message.from length using base 16, with lower-case letters for a-f
	fromLengthEncoded := "x" + fmt.Sprintf("%08x", len(msg.From))

	// Encode the header using base 16, with lower-case letters for a-f.
	headerLengthEncoded := "x" + fmt.Sprintf("%08x", len(messageHeader))

	// Encode the MessageType using base 16, with lower-case letters for a-f.
	messageTypeEncoded := fmt.Sprintf("%09d", msg.Intent.MessageType)

	// Encode the DataTypes is not hex-encoded, but it is zero padded.
	// TODO: The AIP documents define multiple datatypes for future support. For now, we only support 0.
	dataTypeEncoded := fmt.Sprintf("%09d", payloadDataType.Int())

	// Calculate total length of the message. Encode using base 16, with lower-case letters for a-f.
	totalLength := len(msg.To) + 9 +
		len(msg.From) + 9 +
		len(messageHeader) + 9 +
		len(messageTypeEncoded) +
		len(dataTypeEncoded) +
		len(dataBytes) + 9 + 9 // 9 for the total Length field.

	if int64(totalLength) > MaxMessageSizeBytes {
		return nil, EncodeErrorWithField(
			ErrCodeEncodePayloadTooLarge,
			fmt.Sprintf("encoded message size %d bytes exceeds maximum %d bytes", totalLength, MaxMessageSizeBytes),
			"message",
		)
	}
	totalLengthEncoded := "x" + fmt.Sprintf("%08x", totalLength)

	// Construct the SocketMessage
	var _finalMessageBytes strings.Builder
	_finalMessageBytes.WriteString(forceASCII(totalLengthEncoded))
	_finalMessageBytes.WriteString(forceASCII(toLengthEncoded))
	_finalMessageBytes.WriteString(forceASCII(fromLengthEncoded))
	_finalMessageBytes.WriteString(forceASCII(headerLengthEncoded))
	_finalMessageBytes.WriteString(forceASCII(messageTypeEncoded))
	_finalMessageBytes.WriteString(forceASCII(dataTypeEncoded))
	_finalMessageBytes.WriteString(forceASCII(payloadDataLengthEncoded))
	_finalMessageBytes.WriteString(forceASCII(msg.To))
	_finalMessageBytes.WriteString(forceASCII(msg.From))
	_finalMessageBytes.WriteString(forceASCII(messageHeader))

	// return SocketMessage
	return &SocketMessage{
		MessageBytes: append([]byte(_finalMessageBytes.String()), dataBytes...),
	}, nil
}
