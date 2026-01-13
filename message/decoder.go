package message

import (
	"fmt"
	"log"
	"strconv"
	"strings"
)

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

// parseEventTagHeaders parses event_tag headers from GetEvent response
// Format: event_tag:<freq>:<timestamp>=tag_value
// Returns a slice of TagOutput parsed from the headers
func parseEventTagHeaders(headerMap map[string]string) []TagOutput {
	var results []TagOutput

	for key, value := range headerMap {
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
			if freq, err := strconv.Atoi(parts[1]); err == nil {
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
	}
	if eventType, exists := eventMap["event_type"]; exists {
		event.Type = eventType
	} else if eventType, exists := eventMap["_type"]; exists {
		event.Type = eventType
	}
	if user, exists := eventMap["_user"]; exists {
		event.Owner = user
	}
	if owner, exists := eventMap["_owner_id"]; exists {
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
		linkEventFields.Id = forceASCII(eventId)
	} else if eventId, exists := linkMap["event_id"]; exists {
		linkEventFields.Id = forceASCII(eventId)
	}
	if localId, exists := linkMap["local_id"]; exists {
		linkEventFields.LocalId = localId
	} else if eventLocalId, exists := linkMap["_event_local_id"]; exists {
		linkEventFields.LocalId = eventLocalId
	}

	if uniqueId, exists := linkMap["unique_id"]; exists {
		linkEventFields.UniqueId = uniqueId
	} else if uniqueId, exists := linkMap["_unique_id"]; exists {
		linkEventFields.UniqueId = uniqueId
	}
	if eventType, exists := linkMap["event_type"]; exists {
		linkEventFields.Type = eventType
	} else if eventType, exists := linkMap["_type"]; exists {
		linkEventFields.Type = eventType
	}
	if user, exists := linkMap["_user"]; exists {
		linkEventFields.Owner = user
	}
	if owner, exists := linkMap["_owner_id"]; exists {
		linkEventFields.Owner = owner
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
	linkEventFields.DateTime = eventDateTime

	// Decode timestamp from eventMap
	if timestamp, exists := linkMap["timestamp"]; exists {
		linkEventFields.Timestamp = timestamp
	} else if timestamp, exists := linkMap["_timestamp"]; exists {
		linkEventFields.Timestamp = timestamp
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
	linkEventFields.Location = strings.TrimRight(location, "|")
	linkEventFields.LocationSeparator = "|"

	return linkEventFields, true
}

// transformMaptoMessageStruct transforms a header map into the fields of the Message struct
// The header is verb-specific; Header information is specific to the Event and is dependent on the requested action.
func transformHeaderMaptoMessageStruct(headerMap map[string]string, msg *Message) (m *Message, ok bool) {
	// Map _command or _db_cmd to Intent if present (for response messages)
	// Server responses may use either _command or _db_cmd to indicate the command type
	command := ""
	if cmd, exists := headerMap["_command"]; exists {
		command = cmd
	} else if cmd, exists := headerMap["_db_cmd"]; exists {
		command = cmd
	}
	if command != "" {
		if intent, found := IntentFromCommand(command); found {
			msg.Intent = intent
		}
	}

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
	}

	// Map action successful counts to StorageSuccessCount.
	if storageSuccessCount, exists := headerMap["links_ok"]; exists {
		if storageSuccessCountInt, err := strconv.Atoi(storageSuccessCount); err == nil {
			resp.StorageSuccessCount = storageSuccessCountInt
		}
	}

	// Map action unsuccessful counts to StorageErrorCount.
	if storageErrorCount, exists := headerMap["links_with_errors"]; exists {
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

	msg.Payload = payload
	msg.Event = event
	msg.Response = resp

	return msg, true
}

// logRawMessage logs the raw message bytes for debugging (limited to reasonable size)
func logRawMessage(message []byte) {
	const maxLogBytes = 200 // Limit to first 200 bytes for readability
	logBytes := len(message)
	if logBytes > maxLogBytes {
		logBytes = maxLogBytes
	}
	log.Printf("DEBUG: Raw message (first %d of %d bytes): %q", logBytes, len(message), message[:logBytes])
	if len(message) > maxLogBytes {
		log.Printf("DEBUG: ... (truncated, %d more bytes)", len(message)-maxLogBytes)
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
	log.Printf("DEBUG: Decoding message: %s", string(message))

	// Validate minimum message size: 7 fields * 9 bytes each = 63 bytes
	const minMessageSize = 63
	if len(message) < minMessageSize {
		errMsg := fmt.Sprintf("message too short, expected at least %d bytes, got %d bytes", minMessageSize, len(message))
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "message")
	}

	// Read the first 9 * 7 chars, in 9 char chunks to determine lengths or values.
	_, err := decodeMessageSizeParam(message[0:9])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode totalLength: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	toLength, err := decodeMessageSizeParam(message[9:18])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode toLength: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	fromLength, err := decodeMessageSizeParam(message[18:27])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode fromLength: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	headerLength, err := decodeMessageSizeParam(message[27:36])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode headerLength: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	_, err = decodeMessageSizeParam(message[36:45])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode messageType: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	_, err = decodeMessageSizeParam(message[45:54])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode dataType: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	payloadDataLength, err := decodeMessageSizeParam(message[54:63])
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode payloadDataLength: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidSizeParam, errMsg, err)
	}
	messageSizeLength := int64(9)
	toSizeLength := int64(9)
	fromSizeLength := int64(9)
	headerSizeLength := int64(9)
	messageTypeLength := int64(9)
	dataTypeLength := int64(9)

	var lengthsSize int64 = 9 * 7

	// Validate that we have enough bytes for the to, from, and header fields
	toStart := lengthsSize
	toEnd := lengthsSize + toLength
	fromStart := toEnd
	fromEnd := fromStart + fromLength
	headerStart := fromEnd
	headerEnd := headerStart + headerLength

	if int64(len(message)) < headerEnd {
		errMsg := fmt.Sprintf("message too short for header, expected at least %d bytes, got %d bytes", headerEnd, len(message))
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeMessageTooShort, errMsg, "header")
	}

	// Set Envelope fields
	// TODO: does the Intent come from the Message Type or from _command or _type in the header?
	msg.To = string(message[toStart:toEnd])
	msg.From = string(message[fromStart:fromEnd])

	// Decode the header map from the header bytes.
	headerMap, err := decodeHeader(string(message[headerStart:headerEnd]))
	if err != nil {
		errMsg := fmt.Sprintf("failed to decode header: %s", err)
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidHeader, errMsg, err)
	}

	// Parse messageType, trimming null bytes
	messageTypeStart := messageSizeLength + toSizeLength + fromSizeLength + headerSizeLength
	messageTypeEnd := messageTypeStart + messageTypeLength
	if int64(len(message)) < messageTypeEnd {
		errMsg := fmt.Sprintf("message too short for messageType, expected at least %d bytes, got %d bytes", messageTypeEnd, len(message))
		log.Printf("ERROR: %s", errMsg)
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
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, WrapDecodeError(ErrCodeDecodeInvalidMessageType, errMsg, err)
	}
	// Find the Intent.Name corresponding to this messageType

	// Transform header map to Message struct; this handles the different header fields returned for each db_command type.
	_, ok := transformHeaderMaptoMessageStruct(headerMap, &msg)
	if !ok {
		errMsg := "header transformation failed"
		log.Printf("ERROR: %s", errMsg)
		logRawMessage(message)
		setResponseError(&msg, errMsg)
		return &msg, DecodeErrorWithField(ErrCodeDecodeHeaderTransformationFailed, errMsg, "header")
	}

	// Parse dataType, trimming null bytes
	dataTypeStart := messageTypeEnd
	dataTypeEnd := dataTypeStart + dataTypeLength
	if int64(len(message)) < dataTypeEnd {
		errMsg := fmt.Sprintf("message too short for dataType, expected at least %d bytes, got %d bytes", dataTypeEnd, len(message))
		log.Printf("ERROR: %s", errMsg)
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
		log.Printf("ERROR: %s", errMsg)
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
			log.Printf("ERROR: %s", errMsg)
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
		// Determine the Intent from the messageType or _command header field.
		// TODO: handle the Intent better; the payload processing relies on the original intent name; not the messageType in the response.
		// if the message type is 1000 (database command), use the _command header field to determine the Intent.

		intent, found := IntentFromMessageType(&msg.Event.Type)
		if !found {
			errMsg := fmt.Sprintf("unknown messageType: %d", messageType)
			log.Printf("ERROR: %s", errMsg)
			logRawMessage(message)
			setResponseError(&msg, errMsg)
			return &msg, DecodeErrorWithField(ErrCodeDecodeInvalidMessageType, errMsg, "messageType")
		}
		msg.Intent = intent

		// Parse payload for specific intents
		// list of Intent types for which we need to parse the payload: StoreBatchEvents, StoreBatchTags, StoreBatchLinks, LinkEvent, UnlinkEvent, GetEventsForTags, GetEvent.
		switch msg.Intent.Name {
		case "GetEventsForTags":
			msg.Response.EventRecords, _ = parseGetEventsForTagsPayload(&msg)
		case "StoreBatchEvents":
			msg.Response.StoreBatchEventRecords, _ = parseStoreBatchEventsPayload(&msg)
		case "StoreBatchLinks":
			msg.Response.StoreLinkBatchEventRecords, _ = parseLinkEventBatchPayload(&msg)
		}
	}
	return &msg, nil
}
