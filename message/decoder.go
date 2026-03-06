package message

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/PointOfData/pod-os-go-client/log"
)

// decoderLogger is the logger used by DecodeMessage. Set via SetDecoderLogger.
// Defaults to NoOpLogger when nil.
var decoderLogger log.Logger

// SetDecoderLogger sets the logger used by DecodeMessage for decode errors and debug output.
// Call from the application when creating a client to enable decode logging.
func SetDecoderLogger(l log.Logger) {
	decoderLogger = l
}

// decodeMessageSizeParam decodes a message size parameter from bytes
func decodeMessageSizeParam(param []byte) (n int64, err error) {
	// Trim null bytes and whitespace from the parameter
	paramStr := strings.TrimRight(string(param), "\x00")
	paramStr = strings.TrimSpace(paramStr)

	// Handle empty string
	if len(paramStr) == 0 {
		return 0, nil
	}

	if paramStr[0] == 'x' {
		n, err := strconv.ParseInt(paramStr[1:], 16, 32)
		return n, err
	}

	n, err = strconv.ParseInt(paramStr, 10, 32)
	return n, err

}

// decodeHeader decodes a header string into a map
func decodeHeader(s string) (h map[string]string, err error) {
	header := make(map[string]string)
	// split the following string by tab character then add key/value pair to header map.
	// Header values are variable by message type.
	splitString := strings.Split(s, "\t")
	for _, str := range splitString {
		if strings.Contains(str, "=") {
			// Split the string by the first "=" character
			parts := strings.SplitN(str, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				header[key] = value
			}
		}
	}

	return header, nil
}

// payloadContainsLinkRecords reports whether a payload (string or []byte) begins with
// a GetEvent link record marker ("_link="). This is used to detect responses where
// get_links=Y took precedence over send_data, but the MIME type was left as
// application/octet-stream by the server.
func payloadContainsLinkRecords(data interface{}) bool {
	switch v := data.(type) {
	case string:
		return strings.HasPrefix(v, "_link=")
	case []byte:
		return len(v) >= 6 && string(v[:6]) == "_link="
	}
	return false
}

// parseEventTagHeaders parses event_tag headers from GetEvent response
// Format: event_tag:<freq>:<timestamp>=tag_value
// Returns a slice of TagOutput parsed from the headers
func parseEventTagHeaders(msg *Message, headerMap *map[string]string) []TagOutput {
	var results []TagOutput

	for key, value := range *headerMap {
		if !strings.HasPrefix(key, "event_tag:") {
			continue
		}

		// Parse key format: event_tag:<freq>:<timestamp>
		parts := strings.Split(key, ":")
		if len(parts) < 2 {
			continue
		}

		tag := TagOutput{
			Value: value,
		}

		// Parse frequency (second part after event_tag:)
		if len(parts) >= 2 {
			if freq, err := strconv.Atoi(parts[2]); err == nil {
				tag.Frequency = freq
			} else {
				tag.Frequency = 1 // Default to 1 if parsing fails
			}
		}

		// Timestamp is in parts[2] if present, but we don't need to store it
		// as TagOutput doesn't have a timestamp field

		// Parse the value to extract key=value if present
		if eqIdx := strings.Index(value, "="); eqIdx > 0 {
			tag.Key = value[:eqIdx]
			tag.Value = value[eqIdx+1:]
		}

		results = append(results, tag)
	}

	return results
}

// ensureEvent initializes the Event field if nil
func ensureEvent(msg *Message) *EventFields {
	if msg.Event == nil {
		msg.Event = &EventFields{}
	}
	return msg.Event
}

// ensurePayload initializes the Payload field if nil
func ensurePayload(msg *Message) *PayloadFields {
	if msg.Payload == nil {
		msg.Payload = &PayloadFields{}
	}
	return msg.Payload
}

// ensureResponse initializes the Response field if nil
func ensureResponse(msg *Message) *ResponseFields {
	if msg.Response == nil {
		msg.Response = &ResponseFields{}
	}
	return msg.Response
}

// ensureNeuralMemory initializes the NeuralMemory field if nil
func ensureNeuralMemory(msg *Message) *NeuralMemoryFields {
	if msg.NeuralMemory == nil {
		msg.NeuralMemory = &NeuralMemoryFields{}
	}
	return msg.NeuralMemory
}

func decodeEventFields(eventMap map[string]string, event *EventFields) (eventFields *EventFields, ok bool) {
	if eventId, exists := eventMap["_event_id"]; exists {
		event.Id = forceASCII(eventId)
	} else if eventId, exists := eventMap["event_id"]; exists {
		event.Id = forceASCII(eventId)
	}
	if localId, exists := eventMap["local_id"]; exists {
		event.LocalId = localId
	}
	if eventLocalId, exists := eventMap["_event_local_id"]; exists {
		event.LocalId = eventLocalId
	}

	if uniqueId, exists := eventMap["unique_id"]; exists {
		event.UniqueId = uniqueId
	} else if uniqueId, exists := eventMap["_unique_id"]; exists {
		event.UniqueId = uniqueId
	} else if uniqueId, exists := eventMap["tag:1:_unique_id"]; exists {
		// this is a special case for the unique_id tag in GetEventsForTags response
		event.UniqueId = uniqueId
	}

	if eventType, exists := eventMap["event_type"]; exists {
		event.Type = eventType
	} else if eventType, exists := eventMap["_type"]; exists {
		event.Type = eventType
	} else if eventType, exists := eventMap["type"]; exists {
		event.Type = eventType
	}
	if _, exists := eventMap["_user"]; exists {
		// Doing nothing for now. Unclear how to use this.
		// event.Owner = user
	}
	if owner, exists := eventMap["_owner_id"]; exists {
		event.Owner = owner
	} else if owner, exists := eventMap["owner"]; exists {
		event.Owner = owner
	} else if owner, exists := eventMap["_event_owner"]; exists {
		event.Owner = owner
	}

	// Decode event date/time into Response.DateTime
	eventDateTime := DateTimeObject{}
	if year, exists := eventMap["event_year"]; exists {
		eventDateTime.Year, _ = strconv.Atoi(year)
	} else if year, exists := eventMap["_event_year"]; exists {
		eventDateTime.Year, _ = strconv.Atoi(year)
	}
	if month, exists := eventMap["event_mon"]; exists {
		eventDateTime.Month, _ = strconv.Atoi(month)
	} else if month, exists := eventMap["_event_month"]; exists {
		eventDateTime.Month, _ = strconv.Atoi(month)
	}
	if day, exists := eventMap["event_day"]; exists {
		eventDateTime.Day, _ = strconv.Atoi(day)
	} else if day, exists := eventMap["_event_day"]; exists {
		eventDateTime.Day, _ = strconv.Atoi(day)
	}
	if hour, exists := eventMap["event_hour"]; exists {
		eventDateTime.Hour, _ = strconv.Atoi(hour)
	} else if hour, exists := eventMap["_event_hour"]; exists {
		eventDateTime.Hour, _ = strconv.Atoi(hour)
	}
	if minute, exists := eventMap["event_min"]; exists {
		eventDateTime.Minute, _ = strconv.Atoi(minute)
	} else if minute, exists := eventMap["_event_min"]; exists {
		eventDateTime.Minute, _ = strconv.Atoi(minute)
	}
	if second, exists := eventMap["event_sec"]; exists {
		eventDateTime.Second, _ = strconv.Atoi(second)
	} else if second, exists := eventMap["_event_sec"]; exists {
		eventDateTime.Second, _ = strconv.Atoi(second)
	}
	if microsecond, exists := eventMap["event_usec"]; exists {
		eventDateTime.Microsecond, _ = strconv.Atoi(microsecond)
	} else if microsecond, exists := eventMap["_event_usec"]; exists {
		eventDateTime.Microsecond, _ = strconv.Atoi(microsecond)
	}
	event.DateTime = eventDateTime

	// Decode timestamp from eventMap
	if timestamp, exists := eventMap["timestamp"]; exists {
		event.Timestamp = timestamp
	} else if timestamp, exists := eventMap["_timestamp"]; exists {
		event.Timestamp = timestamp
	}

	// Decode _hits (search term match count per event, present in GetEventsForTags response)
	if hits, exists := eventMap["_hits"]; exists {
		if h, err := strconv.Atoi(hits); err == nil {
			event.Hits = h
		}
	}

	// Loop over coordinate n fields and concatenate the values into the Location string.
	// Example response: \t_coordinate_01=TERRA\t_coordinate_02=47.619463\t_coordinate_03=-122.518691
	// Location separator is "|" by default.
	location := ""
	for i := 1; i <= 9; i++ {
		if coord, exists := eventMap[fmt.Sprintf("_coordinate_0%d", i)]; exists {
			location += coord + "|"
		} else if coord, exists := eventMap[fmt.Sprintf("coordinate_0%d", i)]; exists {
			location += coord + "|"
		}
	}
	event.Location = strings.TrimRight(location, "|")
	event.LocationSeparator = "|"

	return event, true
}

func decodeLinkEventFields(linkMap map[string]string, link *LinkFields) (linkEventFields *LinkFields, ok bool) {
	if eventId, exists := linkMap["_event_id"]; exists {
		link.Id = forceASCII(eventId)
	} else if eventId, exists := linkMap["event_id"]; exists {
		link.Id = forceASCII(eventId)
	}
	if localId, exists := linkMap["local_id"]; exists {
		link.LocalId = localId
	} else if eventLocalId, exists := linkMap["_event_local_id"]; exists {
		link.LocalId = eventLocalId
	}

	if uniqueId, exists := linkMap["unique_id"]; exists {
		link.UniqueId = uniqueId
	} else if uniqueId, exists := linkMap["_unique_id"]; exists {
		link.UniqueId = uniqueId
	}
	if eventType, exists := linkMap["event_type"]; exists {
		link.Type = eventType
	} else if eventType, exists := linkMap["_type"]; exists {
		link.Type = eventType
	}
	if user, exists := linkMap["_user"]; exists {
		link.Owner = user
	}
	if owner, exists := linkMap["_owner_id"]; exists {
		link.Owner = owner
	}

	// Decode event date/time into Response.DateTime
	eventDateTime := DateTimeObject{}
	if year, exists := linkMap["event_year"]; exists {
		eventDateTime.Year, _ = strconv.Atoi(year)
	} else if year, exists := linkMap["_event_year"]; exists {
		eventDateTime.Year, _ = strconv.Atoi(year)
	}
	if month, exists := linkMap["event_mon"]; exists {
		eventDateTime.Month, _ = strconv.Atoi(month)
	} else if month, exists := linkMap["_event_month"]; exists {
		eventDateTime.Month, _ = strconv.Atoi(month)
	}
	if day, exists := linkMap["event_day"]; exists {
		eventDateTime.Day, _ = strconv.Atoi(day)
	} else if day, exists := linkMap["_event_day"]; exists {
		eventDateTime.Day, _ = strconv.Atoi(day)
	}
	if hour, exists := linkMap["event_hour"]; exists {
		eventDateTime.Hour, _ = strconv.Atoi(hour)
	} else if hour, exists := linkMap["_event_hour"]; exists {
		eventDateTime.Hour, _ = strconv.Atoi(hour)
	}
	if minute, exists := linkMap["event_min"]; exists {
		eventDateTime.Minute, _ = strconv.Atoi(minute)
	} else if minute, exists := linkMap["_event_min"]; exists {
		eventDateTime.Minute, _ = strconv.Atoi(minute)
	}
	if second, exists := linkMap["event_sec"]; exists {
		eventDateTime.Second, _ = strconv.Atoi(second)
	} else if second, exists := linkMap["_event_sec"]; exists {
		eventDateTime.Second, _ = strconv.Atoi(second)
	}
	if microsecond, exists := linkMap["event_usec"]; exists {
		eventDateTime.Microsecond, _ = strconv.Atoi(microsecond)
	} else if microsecond, exists := linkMap["_event_usec"]; exists {
		eventDateTime.Microsecond, _ = strconv.Atoi(microsecond)
	}
	link.DateTime = eventDateTime

	// Decode timestamp from eventMap
	if timestamp, exists := linkMap["timestamp"]; exists {
		link.Timestamp = timestamp
	} else if timestamp, exists := linkMap["_timestamp"]; exists {
		link.Timestamp = timestamp
	}

	// Loop over coordinate n fields and concatenate the values into the Location string.
	// Example response: \t_coordinate_01=TERRA\t_coordinate_02=47.619463\t_coordinate_03=-122.518691
	// Location separator is "|" by default.
	location := ""
	for i := 1; i <= 9; i++ {
		if coord, exists := linkMap[fmt.Sprintf("_coordinate_0%d", i)]; exists {
			location += coord + "|"
		} else if coord, exists := linkMap[fmt.Sprintf("coordinate_0%d", i)]; exists {
			location += coord + "|"
		}
	}
	link.Location = strings.TrimRight(location, "|")
	link.LocationSeparator = "|"

	return link, true
}

// transformMaptoMessageStruct transforms a header map into the fields of the Message struct
// The header is verb-specific; Header information is specific to the Event and is dependent on the requested action.
// Note: Intent is set by DecodeMessage using IntentFromMessageTypeAndCommand after calling this function.
func transformHeaderMaptoMessageStruct(headerMap map[string]string, msg *Message) (m *Message, ok bool) {
	// Initialize Response and Event for all response fields
	resp := ensureResponse(msg)
	event := ensureEvent(msg)
	payload := ensurePayload(msg)

	// Map status to Response.Status with status message if provided.
	if status, exists := headerMap["_status"]; exists {
		resp.Status = status
	}
	if msgText, exists := headerMap["_msg"]; exists {
		resp.Message = msgText
	}

	// Map counts to TotalEvents; each of which is peculiar to the db_command type.
	if totalEventHits, exists := headerMap["_total_event_hits"]; exists {
		if totalEventHitsInt, err := strconv.Atoi(totalEventHits); err == nil {
			resp.TotalEvents = totalEventHitsInt
		}
	} else if totalEventHits, exists := headerMap["_count"]; exists {
		if totalEventHitsInt, err := strconv.Atoi(totalEventHits); err == nil {
			resp.TotalEvents = totalEventHitsInt
		}
	} else if totalEventHits, exists := headerMap["total_link_requests_found"]; exists {
		if totalEventHitsInt, err := strconv.Atoi(totalEventHits); err == nil {
			resp.TotalEvents = totalEventHitsInt
		}
	} else if totalEventHits, exists := headerMap["_total_link_requests_found"]; exists {
		if totalEventHitsInt, err := strconv.Atoi(totalEventHits); err == nil {
			resp.TotalEvents = totalEventHitsInt
		}
	}

	// Map action successful counts to StorageSuccessCount.
	if storageSuccessCount, exists := headerMap["links_ok"]; exists {
		if storageSuccessCountInt, err := strconv.Atoi(storageSuccessCount); err == nil {
			resp.StorageSuccessCount = storageSuccessCountInt
		}
	} else if storageSuccessCount, exists := headerMap["_links_ok"]; exists {
		if storageSuccessCountInt, err := strconv.Atoi(storageSuccessCount); err == nil {
			resp.StorageSuccessCount = storageSuccessCountInt
		}
	}

	// Map action unsuccessful counts to StorageErrorCount.
	if storageErrorCount, exists := headerMap["links_with_errors"]; exists {
		if storageErrorCountInt, err := strconv.Atoi(storageErrorCount); err == nil {
			resp.StorageErrorCount = storageErrorCountInt
		}
	} else if storageErrorCount, exists := headerMap["_links_with_errors"]; exists {
		if storageErrorCountInt, err := strconv.Atoi(storageErrorCount); err == nil {
			resp.StorageErrorCount = storageErrorCountInt
		}
	}

	// Map start and end result indices to Response.StartResult and Response.EndResult.
	if startResult, exists := headerMap["_start_result"]; exists {
		if startResultInt, err := strconv.Atoi(startResult); err == nil {
			resp.StartResult = startResultInt
		}
	}
	if endResult, exists := headerMap["_end_result"]; exists {
		if endResultInt, err := strconv.Atoi(endResult); err == nil {
			resp.EndResult = endResultInt
		}
	}

	// Map returned events to Response.ReturnedEvents.
	if returnedEvents, exists := headerMap["_returned_event_hits"]; exists {
		if returnedEventsInt, err := strconv.Atoi(returnedEvents); err == nil {
			resp.ReturnedEvents = returnedEventsInt
		}
	}

	// Map total links to Response.LinkCount.
	if totalLinks, exists := headerMap["_set_link_count"]; exists {
		if totalLinksInt, err := strconv.Atoi(totalLinks); err == nil {
			resp.LinkCount = totalLinksInt
		}
	} else if totalLinks, exists := headerMap["_link_count"]; exists {
		if totalLinksInt, err := strconv.Atoi(totalLinks); err == nil {
			resp.LinkCount = totalLinksInt
		}
	}

	// Map tag count to Response.TagCount.
	if tagCount, exists := headerMap["_tag_count"]; exists {
		if tagCountInt, err := strconv.Atoi(tagCount); err == nil {
			resp.TagCount = tagCountInt
		}
	}

	// Map link_event to Response.LinkId for LinkEventResponse.
	if linkEvent, exists := headerMap["link_event"]; exists {
		resp.LinkId = linkEvent
	}

	// Map message ID to Envelope
	if msgId, exists := headerMap["_msg_id"]; exists {
		msg.MessageId = msgId
	}

	// Handle both _event_id and event_id (some responses use event_id without underscore)
	_, ok = decodeEventFields(headerMap, event)
	if !ok {
		return nil, false
	}

	// Decode payload fields held in header map.
	if mime, exists := headerMap["mime"]; exists {
		payload.MimeType = mime
	} else if mime, exists := headerMap["_mimetype"]; exists {
		payload.MimeType = mime
	}
	if dataType, exists := headerMap["data_type"]; exists {
		if dt, err := strconv.Atoi(dataType); err == nil {
			payload.DataType = DataType(dt)
		}
	}
	if dataSize, exists := headerMap["_datasize"]; exists {
		if ds, err := strconv.Atoi(dataSize); err == nil {
			payload.DataSize = ds
		}
	}

	msg.Payload = payload
	msg.Event = event
	msg.Response = resp

	return msg, true
}

// logRawMessage logs the raw message bytes for debugging (limited to reasonable size).
// Only logs when decoderLogger has Debug level enabled.
func logRawMessage(message []byte) {
	l := log.LoggerOrNoOp(decoderLogger)
	if !l.Enabled(log.LevelDebug) {
		return
	}
	const maxLogBytes = 200 // Limit to first 200 bytes for readability
	logBytes := len(message)
	if logBytes > maxLogBytes {
		logBytes = maxLogBytes
	}
	l.Debug("raw message", "first_bytes", logBytes, "total_bytes", len(message), "preview", string(message[:logBytes]))
	if len(message) > maxLogBytes {
		l.Debug("message truncated", "remaining_bytes", len(message)-maxLogBytes)
	}
}

// setResponseError sets error status on the Response field
func setResponseError(msg *Message, errMsg string) {
	resp := ensureResponse(msg)
	resp.Status = "ERROR"
	resp.Message = errMsg
}

// DecodeMessage decodes a raw message byte array into a Message struct; and includes Response fields in Response struct.
// Returns an error if decoding fails. The returned Message may have Response.Status="ERROR"
// if the message was partially decoded but contains protocol errors.
func DecodeMessage(message []byte) (*Message, error) {
	// TODO: the efficiency of this method is dubious and must be re-evaluated.
	var msg Message
	l := log.LoggerOrNoOp(decoderLogger)
	if l.Enabled(log.LevelDebug) {
		l.Debug("decoding message", "message", string(message))
	}

	// Enforce a hard upper bound on total message size to protect against
	// bad actors and excessive memory usage.
	if int64(len(message)) > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("message size %d bytes exceeds maximum allowed %d bytes", len(message), MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodePayloadTooLarge, errMsg, "message")
	}

	// Validate minimum message size: 7 fields * 9 bytes each = 63 bytes
	const minMessageSize = 63
	if len(message) < minMessageSize {
		errMsg := fmt.Sprintf("message too short, expected at least %d bytes, got %d bytes", minMessageSize, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "message")
	}

	// Read the first 9 * 7 chars, in 9 char chunks to determine lengths or values.
	totalLength, err := decodeMessageSizeParam(message[0:9])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode totalLength: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	if totalLength <= 0 {
		errMsg := fmt.Sprintf("invalid totalLength: %d (must be positive)", totalLength)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidSizeParam, errMsg, "totalLength")
	}
	if totalLength > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("totalLength %d bytes exceeds maximum allowed %d bytes", totalLength, MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodePayloadTooLarge, errMsg, "message")
	}

	toLength, err := decodeMessageSizeParam(message[9:18])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode toLength: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	if toLength < 0 || toLength > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("invalid toLength: %d (must be between 0 and %d)", toLength, MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidSizeParam, errMsg, "toLength")
	}

	fromLength, err := decodeMessageSizeParam(message[18:27])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode fromLength: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	if fromLength < 0 || fromLength > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("invalid fromLength: %d (must be between 0 and %d)", fromLength, MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidSizeParam, errMsg, "fromLength")
	}

	headerLength, err := decodeMessageSizeParam(message[27:36])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode headerLength: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	if headerLength < 0 || headerLength > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("invalid headerLength: %d (must be between 0 and %d)", headerLength, MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidSizeParam, errMsg, "headerLength")
	}
	_, err = decodeMessageSizeParam(message[36:45])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode messageType: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	_, err = decodeMessageSizeParam(message[45:54])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode dataType: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	payloadDataLength, err := decodeMessageSizeParam(message[54:63])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode payloadDataLength: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	if payloadDataLength < 0 || payloadDataLength > MaxMessageSizeBytes {
		errMsg := fmt.Sprintf("invalid payloadDataLength: %d (must be between 0 and %d)", payloadDataLength, MaxMessageSizeBytes)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidSizeParam, errMsg, "payloadDataLength")
	}
	messageSizeLength := int64(9)
	toSizeLength := int64(9)
	fromSizeLength := int64(9)
	headerSizeLength := int64(9)
	messageTypeLength := int64(9)
	dataTypeLength := int64(9)

	var lengthsSize int64 = 9 * 7

	// Calculate positions of the to, from, and header fields. These calculations
	// are validated before any slicing to protect against bad actor messages
	// with inconsistent or malicious length fields.
	toStart := lengthsSize
	toEnd := lengthsSize + toLength
	fromStart := toEnd
	fromEnd := fromStart + fromLength
	headerStart := fromEnd
	headerEnd := headerStart + headerLength

	// Validate that we have enough bytes for the to, from, and header fields
	if int64(len(message)) < toEnd {
		errMsg := fmt.Sprintf("message too short for to field, expected at least %d bytes, got %d bytes", toEnd, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "to")
	}
	if int64(len(message)) < fromEnd {
		errMsg := fmt.Sprintf("message too short for from field, expected at least %d bytes, got %d bytes", fromEnd, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "from")
	}
	if int64(len(message)) < headerEnd {
		errMsg := fmt.Sprintf("message too short for header, expected at least %d bytes, got %d bytes", headerEnd, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "header")
	}

	// Set Envelope fields
	msg.To = string(message[toStart:toEnd])
	
	// Handle From field with potential routing data
	// Pod-OS may return routing info like: address|gateway,client,timestamp
	// We only want the address part (position 0 when split on '|')
	fromStr := string(message[fromStart:fromEnd])
	if pipeIndex := strings.IndexByte(fromStr, '|'); pipeIndex != -1 {
		msg.From = fromStr[:pipeIndex]
	} else {
		msg.From = fromStr
	}

	// Decode the header map from the header bytes.
	headerMap, err := decodeHeader(string(message[headerStart:headerEnd]))
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode header: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidHeader, errMsg, err)
	}

	// Parse messageType, trimming null bytes
	messageTypeStart := messageSizeLength + toSizeLength + fromSizeLength + headerSizeLength
	messageTypeEnd := messageTypeStart + messageTypeLength
	if int64(len(message)) < messageTypeEnd {
		errMsg := fmt.Sprintf("message too short for messageType, expected at least %d bytes, got %d bytes", messageTypeEnd, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "messageType")
	}
	messageTypeBytes := message[messageTypeStart:messageTypeEnd]
	messageTypeStr := strings.TrimRight(string(messageTypeBytes), "\x00")
	messageTypeStr = strings.TrimSpace(messageTypeStr)
	messageType, err := strconv.Atoi(messageTypeStr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse messageType: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidMessageType, errMsg, err)
	}
	// Transform header map to Message struct; this handles the different header fields returned for each db_command type.
	_, ok := transformHeaderMaptoMessageStruct(headerMap, &msg)
	if !ok {
		errMsg := "header transformation failed"
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeHeaderTransformationFailed, errMsg, "header")
	}

	// Determine the Intent from messageType and _type/_command header field.
	// For MEM_REQ (1000) or MEM_REPLY (1001), use the command from header to get the specific intent.
	command := ""
	if cmd, exists := headerMap["_type"]; exists {
		command = cmd
	} else if cmd, exists := headerMap["_command"]; exists {
		command = cmd
	} else if cmd, exists := headerMap["_db_cmd"]; exists {
		command = cmd
	}

	intent, found := IntentFromMessageTypeAndCommand(messageType, command)
	if !found {
		// Fallback to messageType-only lookup for non-Neural Memory messages
		intent, found = intentFromMessageTypeInt(messageType)
		if !found {
			errMsg := fmt.Sprintf("unknown messageType: %d with command: %s", messageType, command)
			l.Error("decode error", "error", errMsg)
			logRawMessage(message)
			setResponseError(&msg, errMsg)
			return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidMessageType, errMsg, "messageType")
		}
	}
	msg.Intent = intent
	// Parse dataType, trimming null bytes
	dataTypeStart := messageTypeEnd
	dataTypeEnd := dataTypeStart + dataTypeLength
	if int64(len(message)) < dataTypeEnd {
		errMsg := fmt.Sprintf("message too short for dataType, expected at least %d bytes, got %d bytes", dataTypeEnd, len(message))
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "dataType")
	}
	dataTypeBytes := message[dataTypeStart:dataTypeEnd]
	dataTypeStr := strings.TrimRight(string(dataTypeBytes), "\x00")
	dataTypeStr = strings.TrimSpace(dataTypeStr)
	dataType, err := strconv.Atoi(dataTypeStr)
	if err != nil {
		errMsg := fmt.Sprintf("failed to parse dataType: %s", err)
		l.Error("decode error", "error", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidDataType, errMsg, err)
	}
	msg.Payload.DataType = DataType(dataType)

	// Manage payload data; this is the data returned from the Actor.
	if payloadDataLength > 0 {
		payloadStart := headerEnd
		payloadEnd := payloadStart + payloadDataLength

		// Validate that we have enough bytes for the payload
		if int64(len(message)) < payloadEnd {
			errMsg := fmt.Sprintf("message too short for payload, expected at least %d bytes, got %d bytes", payloadEnd, len(message))
			l.Error("decode error", "error", errMsg)
			logRawMessage(message)
			setResponseError(&msg, errMsg)
			return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "payload")
		}

		payloadBytes := message[payloadStart:payloadEnd]

		// TODO: previous lines cast bytes to string; if this is an octet stream, we should not cast to string.
		if msg.Payload.MimeType == "application/json" {
			msg.Payload.Data = string(payloadBytes)
		} else if msg.Payload.MimeType == "application/octet-stream" {
			msg.Payload.Data = payloadBytes
		} else if msg.Payload.MimeType == "text/plain" {
			msg.Payload.Data = string(payloadBytes)
		} else {
			msg.Payload.Data = string(payloadBytes)
		}

		// Parse payload for specific intents
		// Handle both Request and Response intent names for payload parsing
		switch msg.Intent.Name {
		case "GetEvent", "GetEventResponse":
			// For GetEventResponse, parse Tags and Links from payload if not BLOB data.
			// BLOB data is when SendData=true was in the request and get_links=N.
			// When get_links=Y takes precedence over send_data, Pod-OS returns link records
			// as text even if the MIME type header still says application/octet-stream, so we
			// also parse when the payload content starts with link record markers.
			if msg.Payload.MimeType != "application/octet-stream" || payloadContainsLinkRecords(msg.Payload.Data) {

				// Parse structured data (Tags and Links)
				tags, links, ok := parseGetEventResponse(&msg, &headerMap)
				if ok {
					if msg.Event != nil {
						msg.Event.Tags = tags
						msg.Event.Links = links
						msg.Response.EventRecords = append(msg.Response.EventRecords, *msg.Event)
						if len(msg.Response.EventRecords) > 0 {
							msg.Response.EventRecords[0].Tags = tags
							msg.Response.EventRecords[0].Links = links
						}
					}
				}
			}

		case "GetEventsForTags", "GetEventsForTagsResponse":
			msg.Response.EventRecords, _ = parseGetEventsForTagsPayload(&msg)

		case "StoreBatchEvents", "StoreBatchEventsResponse":
			if batchRecord, ok := parseStoreBatchEventsPayload(&msg); ok && batchRecord != nil {
				msg.Response.StoreBatchEventRecord = *batchRecord
			}

		case "StoreBatchLinks", "StoreBatchLinksResponse":
			if linkRecord, ok := parseLinkEventBatchPayload(&msg); ok && linkRecord != nil {
				msg.Response.StoreLinkBatchEventRecord = *linkRecord
			}

		case "StoreBatchTags", "StoreBatchTagsResponse":
			// StoreBatchTagsResponse: Payload is unused per spec
			// Header fields contain all response data (_status, _count, etc.)

		case "StoreEvent", "StoreEventResponse":
			// StoreEventResponse: Payload is unused per spec
			// Header fields contain all response data (_status, LocalId, _count, etc.)

		case "LinkEvent", "LinkEventResponse":
			// LinkEventResponse: Payload is unused per spec
			// Header fields contain link_event (LinkFields.Id) and other response data

		case "UnlinkEvent", "UnlinkEventResponse":
			// UnlinkEventResponse: Payload is unused per spec
			// Header fields contain all response data

		case "ActorResponse":
			// ActorResponse: General actor response, payload handling depends on actor type
			// Payload data is already set above based on MimeType
		}
	}

	// For GetEvent responses: tags are returned in the response HEADER as event_tag: fields,
	// NOT in the payload. When payloadDataLength==0 the switch block above is bypassed entirely,
	// so we must call parseGetEventResponse here to extract header-based tags even with no payload.
	if payloadDataLength == 0 && (msg.Intent.Name == "GetEvent" || msg.Intent.Name == "GetEventResponse") {
		if msg.Payload.MimeType != "application/octet-stream" || payloadContainsLinkRecords(msg.Payload.Data) {
			tags, links, ok := parseGetEventResponse(&msg, &headerMap)
			if ok && msg.Event != nil {
				msg.Event.Tags = tags
				msg.Event.Links = links
				msg.Response.EventRecords = append(msg.Response.EventRecords, *msg.Event)
				if len(msg.Response.EventRecords) > 0 {
					msg.Response.EventRecords[0].Tags = tags
					msg.Response.EventRecords[0].Links = links
				}
			}
		}
	}

	return &msg, nil
}
