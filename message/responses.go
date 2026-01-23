package message

import (
	"strconv"
	"strings"
)

// ParseTagsFromPayload parses tags from a payload string.
// Payload Format: <frequency> <tab> <tag category> <tab> <tag value>
// Example: 1	*	word1
// Each line represents one tag entry.
// This is used by GetEvent with GetTags=true to parse tags from payload.
func ParseTagsFromPayload(payload string) []TagOutput {
	var results []TagOutput

	// Split by newlines to get individual tag records
	lines := strings.Split(payload, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue // Skip empty lines
		}

		// Parse each line as tab-separated: frequency <tab> category <tab> value
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 {
			tag := TagOutput{}

			// Parse frequency (first field)
			if freq, err := strconv.Atoi(strings.TrimSpace(fields[0])); err == nil {
				tag.Frequency = freq
			} else {
				tag.Frequency = 1 // Default to 1 if parsing fails
			}

			// Parse category (second field)
			tag.Category = strings.TrimSpace(fields[1])

			// Parse value (third field)
			tag.Value = strings.TrimSpace(fields[2])

			results = append(results, tag)
		} else if len(fields) == 2 {
			// Handle case with only frequency and value (no category)
			tag := TagOutput{}
			if freq, err := strconv.Atoi(strings.TrimSpace(fields[0])); err == nil {
				tag.Frequency = freq
			} else {
				tag.Frequency = 1
			}
			tag.Value = strings.TrimSpace(fields[1])
			results = append(results, tag)
		}
	}

	return results
}

// parseGetEventsForTagsPayload parses the buffered payload for GetEventsForTags response
// Uses single-pass indexing for O(N) complexity regardless of payload size.
// The payload contains tab-separated field=value pairs, newline-terminated records.
// Line types by prefix:
//   - _event_id=: Event object with inline tags
//   - _link=: Link between events (source field identifies parent event)
//   - _linktag=: Tags for a link (first field is link ID)
//   - _targettag=: Tags describing link's target event (first field is target ID)
//   - _brief_hit=: Brief hit record (when include_brief_hits=Y)
func parseGetEventsForTagsPayload(msg *Message) (eventResults []EventFields, ok bool) {
	payloadStr, isString := msg.Payload.Data.(string)
	if !isString {
		return nil, false
	}

	lines := strings.Split(payloadStr, "\n")

	// Check if this is a brief hits response by examining the first non-empty line.
	// If _brief_hit exists, no other output types (_event_id, _link, _linktag, _targettag) are included.
	isBriefHitsResponse := false
	for _, line := range lines {
		line = strings.TrimRight(line, "\x00")
		if line == "" || line == "\x0F" || line == "\x00" {
			continue
		}
		isBriefHitsResponse = strings.HasPrefix(line, "_brief_hit=")
		break
	}

	// Handle brief hits response - only parse _brief_hit lines
	if isBriefHitsResponse {
		for _, line := range lines {
			line = strings.TrimRight(line, "\x00")
			if line == "" || line == "\x0F" || line == "\x00" {
				continue
			}
			if strings.HasPrefix(line, "_brief_hit=") {
				parseBriefHitLine(line, msg)
			}
		}
		return nil, true
	}

	// Pre-allocate maps with estimated capacity to reduce rehashing
	estimatedEvents := len(lines) / 10 // rough estimate
	if estimatedEvents < 16 {
		estimatedEvents = 16
	}

	eventsMap := make(map[string]*EventFields, estimatedEvents)
	eventOrder := make([]string, 0, estimatedEvents) // preserve insertion order

	linksMap := make(map[string]*LinkFields, estimatedEvents*2)
	linksBySource := make(map[string][]string, estimatedEvents) // source_event_id -> []link_ids

	linkTagsMap := make(map[string][]TagOutput, estimatedEvents*2)
	targetTagsMap := make(map[string][]TagOutput, estimatedEvents*2)

	// SINGLE PASS: categorize and index all lines
	for _, line := range lines {
		line = strings.TrimRight(line, "\x00")
		if line == "" || line == "\x0F" || line == "\x00" {
			continue
		}

		// Determine line type by prefix
		switch {
		case strings.HasPrefix(line, "_event_id="):
			eventId, event := parseEventIdLine(line, msg)
			if eventId != "" && event != nil {
				eventsMap[eventId] = event
				eventOrder = append(eventOrder, eventId)
			}

		case strings.HasPrefix(line, "_link="):
			linkId, link := parseLinkLine(line)
			if linkId != "" && link != nil {
				linksMap[linkId] = link
				// Index by source for O(1) lookup during assembly
				if link.EventA != "" {
					linksBySource[link.EventA] = append(linksBySource[link.EventA], linkId)
				}
			}

		case strings.HasPrefix(line, "_linktag="):
			linkId, tag := parseLinkTagLine(line)
			if linkId != "" && tag != nil {
				linkTagsMap[linkId] = append(linkTagsMap[linkId], *tag)
			}

		case strings.HasPrefix(line, "_targettag="):
			targetId, tag := parseTargetTagLine(line)
			if targetId != "" && tag != nil {
				targetTagsMap[targetId] = append(targetTagsMap[targetId], *tag)
			}
		}
	}

	// ASSEMBLY PHASE: Build final results using map lookups only
	results := make([]EventFields, 0, len(eventOrder))

	for _, eventId := range eventOrder {
		event := eventsMap[eventId]
		if event == nil {
			continue
		}

		// Get all links for this event via index
		linkIds := linksBySource[eventId]
		links := make([]LinkFields, 0, len(linkIds))

		for _, linkId := range linkIds {
			link := linksMap[linkId]
			if link == nil {
				continue
			}

			// Attach link tags (O(1) lookup)
			if tags, exists := linkTagsMap[linkId]; exists {
				link.Tags = tags
			}

			// Attach target tags using the link's target event ID (O(1) lookup)
			if link.EventB != "" {
				if targetTags, exists := targetTagsMap[link.EventB]; exists {
					link.TargetTags = targetTags
				}
			}

			links = append(links, *link)
		}

		event.Links = links
		results = append(results, *event)
	}

	return results, true
}

// parseEventIdLine parses an _event_id line and extracts the event with inline tags
func parseEventIdLine(line string, msg *Message) (string, *EventFields) {
	recordMap := parseTabDelimitedLine(line)

	eventId, exists := recordMap["_event_id"]
	if !exists {
		return "", nil
	}

	event := &EventFields{Id: eventId}
	decodeEventFields(recordMap, event)

	// Parse datasize/mimetype
	if datasize, exists := recordMap["_datasize"]; exists {
		if ds, err := strconv.Atoi(datasize); err == nil {
			event.PayloadData.DataSize = ds
		}
	}
	if mimetype, exists := recordMap["_mimetype"]; exists {
		event.PayloadData.MimeType = mimetype
	}

	// Parse inline tags (tag:freq:key=value format)
	for key, value := range recordMap {
		if strings.HasPrefix(key, "tag:") {
			parts := strings.Split(key, ":")
			if len(parts) == 3 {
				freq, _ := strconv.Atoi(parts[1])
				event.Tags = append(event.Tags, TagOutput{
					Frequency: freq,
					Key:       parts[2],
					Value:     value,
				})
			}
		}
		// Handle _event_tag format
		if strings.HasPrefix(key, "_event_tag") {
			tag := parseEventTagPayloadField(recordMap)
			if tag != nil {
				event.Tags = append(event.Tags, *tag)
			}
		}
	}

	// Extract unique_id from tags if present
	for _, tag := range event.Tags {
		if tag.Key == "_unique_id" || tag.Key == "unique_id" {
			event.UniqueId = tag.Value
			break
		}
	}

	return eventId, event
}

// parseBriefHitLine parses a _brief_hit line and adds it to msg.Response.BriefHits
func parseBriefHitLine(line string, msg *Message) {
	if msg.Response == nil {
		return
	}

	recordMap := parseTabDelimitedLine(line)

	briefHit, exists := recordMap["_brief_hit"]
	if !exists {
		return
	}

	hits := 0
	if hitsStr, hitsExists := recordMap["_hits"]; hitsExists {
		hits, _ = strconv.Atoi(hitsStr)
	}

	msg.Response.BriefHits = append(msg.Response.BriefHits, BriefHitRecord{
		EventId:   briefHit,
		TotalHits: hits,
	})
}

// parseLinkLine parses a _link line
func parseLinkLine(line string) (string, *LinkFields) {
	recordMap := parseTabDelimitedLine(line)

	linkId, exists := recordMap["_link"]
	if !exists {
		return "", nil
	}

	link := &LinkFields{Id: linkId}

	if source, exists := recordMap["source"]; exists {
		link.EventA = source
	}
	if target, exists := recordMap["target"]; exists {
		link.EventB = target
	}
	if uniqueId, exists := recordMap["unique_id"]; exists {
		link.UniqueId = uniqueId
	}
	if sourceUniqueId, exists := recordMap["source_unique_id"]; exists {
		link.UniqueIdA = sourceUniqueId
	}
	if targetUniqueId, exists := recordMap["target_unique_id"]; exists {
		link.UniqueIdB = targetUniqueId
	}
	if strength, exists := recordMap["strength"]; exists {
		if s, err := strconv.ParseFloat(strength, 64); err == nil {
			link.StrengthB = s
		}
	}
	if category, exists := recordMap["category"]; exists {
		link.Category = category
	}

	return linkId, link
}

// parseLinkTagLine parses a _linktag line
func parseLinkTagLine(line string) (string, *TagOutput) {
	recordMap := parseTabDelimitedLine(line)

	linkId, exists := recordMap["_linktag"]
	if !exists {
		return "", nil
	}

	tag := &TagOutput{}
	if freqStr, exists := recordMap["freq"]; exists {
		tag.Frequency, _ = strconv.Atoi(freqStr)
	}
	if value, exists := recordMap["value"]; exists {
		if eqIdx := strings.Index(value, "="); eqIdx > 0 {
			tag.Key = value[:eqIdx]
			tag.Value = value[eqIdx+1:]
		} else {
			tag.Value = value
		}
	}

	return linkId, tag
}

// parseTargetTagLine parses a _targettag line
func parseTargetTagLine(line string) (string, *TagOutput) {
	recordMap := parseTabDelimitedLine(line)

	targetId, exists := recordMap["_targettag"]
	if !exists {
		return "", nil
	}

	tag := &TagOutput{}
	if freqStr, exists := recordMap["freq"]; exists {
		tag.Frequency, _ = strconv.Atoi(freqStr)
	}
	if value, exists := recordMap["value"]; exists {
		if eqIdx := strings.Index(value, "="); eqIdx > 0 {
			tag.Key = value[:eqIdx]
			tag.Value = value[eqIdx+1:]
		} else {
			tag.Value = value
		}
	}

	return targetId, tag
}

// parseTabDelimitedLine parses a tab-delimited line of key=value pairs into a map
func parseTabDelimitedLine(line string) map[string]string {
	recordMap := make(map[string]string)
	fields := strings.Split(line, "\t")

	for _, field := range fields {
		if field == "" {
			continue
		}
		if parts := strings.SplitN(field, "=", 2); len(parts) == 2 {
			recordMap[parts[0]] = parts[1]
		}
	}
	return recordMap
}

// parseEventTagPayloadField parses tag fields from GetEventsForTags payload record
// Fields: _event_tag (Tag.Id), tag_freq (Tag.Frequency), tag_value (Tag.Key=Tag.Value), tag_timestamp
func parseEventTagPayloadField(recordMap map[string]string) *TagOutput {
	tag := &TagOutput{}

	if tagId, exists := recordMap["_event_tag"]; exists {
		// Tag ID is stored for reference but not in TagOutput
		_ = tagId
	}

	if freqStr, exists := recordMap["tag_freq"]; exists {
		if freq, err := strconv.Atoi(freqStr); err == nil {
			tag.Frequency = freq
		}
	}

	if tagValue, exists := recordMap["tag_value"]; exists {
		eqIdx := strings.Index(tagValue, "=")
		if eqIdx > 0 {
			tag.Key = tagValue[:eqIdx]
			tag.Value = tagValue[eqIdx+1:]
		} else {
			tag.Value = tagValue
		}
	}

	// tag_timestamp is parsed but TagOutput doesn't have a Timestamp field
	// If needed, it would be: tag.Timestamp = recordMap["tag_timestamp"]

	return tag
}

// ParseStoreBatchEventsPayload parses the payload for StoreBatchEvents response
func parseStoreBatchEventsPayload(msg *Message) (storeBatchResults []StoreBatchEventRecord, ok bool) {
	var results []StoreBatchEventRecord

	// Split by newlines to get individual records
	lines := strings.Split(msg.Payload.Data.(string), "\n")
	// remove trailing \x00 from line
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\x00")
	}

	for _, line := range lines {
		// Skip empty lines
		if line == "" {
			continue
		} else if line == "\x0F" { // EOF marker
			break
		} else if line == "\x00" { // END marker
			break
		}
		resStoreBatchRecord := &StoreBatchEventRecord{}

		// Parse each line as an equals-sign separated key=value pairs; capture the storage status and message
		recordMap := make(map[string]string)
		fields := strings.Split(line, "\t")

		for _, field := range fields {
			if field == "" {
				continue
			}
			parts := strings.Split(field, "=")
			if len(parts) == 2 {
				recordMap[parts[0]] = parts[1]
			}
		}

		// Capture the storage status and message from the record
		resStoreBatchRecord.Status = recordMap["_status"]
		resStoreBatchRecord.Message = recordMap["_msg"]

		// Extract Event fields from the payload
		resEvent, ok := decodeEventFields(recordMap, &resStoreBatchRecord.EventFields)
		if !ok {
			return nil, false
		}
		resStoreBatchRecord.EventFields = *resEvent
		results = append(results, *resStoreBatchRecord)
	}
	return results, true
}

// parseGetEventPayload parses the payload for GetEvent response
// The payload can contain Tags and Links data based on the request options:
// - Tags: event_tag:nnnnnnnnn:f=key=value format
// - Links: _link={Id} with associated fields
// - LinkTags: _linktag with associated fields
// - TargetTags: _target_event_tag with associated fields
// If SendData=true was in the request, the payload is BLOB data and this function should not be called.
func parseGetEventPayload(msg *Message) (tags []TagOutput, links []LinkFields, ok bool) {
	payloadStr, isString := msg.Payload.Data.(string)
	if !isString {
		return nil, nil, false
	}

	var resultTags []TagOutput
	var resultLinks []LinkFields
	linkMap := make(map[string]*LinkFields) // Consolidate links by ID
	linkTagsMap := make(map[string][]TagOutput)
	targetTagsMap := make(map[string][]TagOutput)

	// Split by tabs to get individual fields
	fields := strings.Split(payloadStr, "\t")

	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}

		// Parse event tags: event_tag:nnnnnnnnn:f=key=value
		if strings.HasPrefix(field, "event_tag:") {
			tag := parseEventTagField(field)
			if tag != nil {
				resultTags = append(resultTags, *tag)
			}
			continue
		}

		// Parse links: _link={Id}
		if strings.HasPrefix(field, "_link=") {
			linkId := strings.TrimPrefix(field, "_link=")
			if _, exists := linkMap[linkId]; !exists {
				linkMap[linkId] = &LinkFields{Id: linkId}
			}
			continue
		}

		// Parse link fields that follow a _link field
		// These include: unique_id=, target_event=, target_unique_id=, strength=, category=
		if strings.Contains(field, "=") {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]

				// Check if this is a link-related field and update the most recent link
				// (In practice, the link ID context is maintained by ordering)
				for linkId, link := range linkMap {
					switch key {
					case "unique_id":
						link.UniqueId = value
						linkMap[linkId] = link
					case "target_event":
						link.EventB = value
						linkMap[linkId] = link
					case "target_unique_id":
						link.UniqueIdB = value
						linkMap[linkId] = link
					case "strength":
						if s, err := strconv.ParseFloat(value, 64); err == nil {
							link.StrengthB = s
						}
						linkMap[linkId] = link
					case "category":
						link.Category = value
						linkMap[linkId] = link
					}
				}
			}
		}

		// Parse link tags: _linktag (no = and no value for the field itself)
		if strings.HasPrefix(field, "_linktag") && !strings.Contains(field, "=") {
			// The next fields contain link tag details
			continue
		}

		// Parse target tags: _target_event_tag
		if strings.HasPrefix(field, "_target_event_tag") && !strings.Contains(field, "=") {
			// The next fields contain target tag details
			continue
		}
	}

	// Consolidate links with their tags
	for _, link := range linkMap {
		if tags, exists := linkTagsMap[link.Id]; exists {
			// Add tags to link (would need LinkFields.Tags field)
			_ = tags
		}
		if tags, exists := targetTagsMap[link.Id]; exists {
			// Add target tags (would need additional field)
			_ = tags
		}
		resultLinks = append(resultLinks, *link)
	}

	return resultTags, resultLinks, true
}

// parseEventTagField parses an event_tag field from GetEvent payload
// Format: event_tag:nnnnnnnnn:f=key=value where f is frequency
func parseEventTagField(field string) *TagOutput {
	// Remove "event_tag:" prefix
	remainder := strings.TrimPrefix(field, "event_tag:")

	// Format: nnnnnnnnn:f=key=value
	parts := strings.SplitN(remainder, ":", 2)
	if len(parts) < 2 {
		return nil
	}

	// parts[0] is tag number (not needed for output)
	// parts[1] is f=key=value

	freqKeyValue := parts[1]
	eqIdx := strings.Index(freqKeyValue, "=")
	if eqIdx < 0 {
		return nil
	}

	freqStr := freqKeyValue[:eqIdx]
	freq, err := strconv.Atoi(freqStr)
	if err != nil {
		freq = 1 // Default
	}

	// The rest is key=value
	keyValue := freqKeyValue[eqIdx+1:]
	keyEqIdx := strings.Index(keyValue, "=")
	if keyEqIdx < 0 {
		return &TagOutput{
			Frequency: freq,
			Value:     keyValue,
		}
	}

	return &TagOutput{
		Frequency: freq,
		Key:       keyValue[:keyEqIdx],
		Value:     keyValue[keyEqIdx+1:],
	}
}

// parseLinkTagFields parses link tag fields from payload
// Format: event_id={linkId} unique={uniqueId} freq={freq} timestamp={ts} value={key=value}
func parseLinkTagFields(fields []string) *TagOutput {
	tag := &TagOutput{}

	for _, field := range fields {
		if strings.HasPrefix(field, "freq=") {
			if f, err := strconv.Atoi(strings.TrimPrefix(field, "freq=")); err == nil {
				tag.Frequency = f
			}
		} else if strings.HasPrefix(field, "value=") {
			kv := strings.TrimPrefix(field, "value=")
			eqIdx := strings.Index(kv, "=")
			if eqIdx > 0 {
				tag.Key = kv[:eqIdx]
				tag.Value = kv[eqIdx+1:]
			} else {
				tag.Value = kv
			}
		}
	}

	return tag
}

// parseLinkEventBatchPayload parses the payload for StoreBatchLinks response
// Payload format: newline-terminated records of tab-delimited fields:
// _status, _status_info (Message), unique_id, owner_unique_id, owner_id, owner, timestamp,
// loc, loc_delim, type, event_id_a, event_id_b, unique_id_a, unique_id_b,
// strength_a, strength_b, category
func parseLinkEventBatchPayload(msg *Message) (storeLinkBatchResults []StoreLinkBatchEventRecord, ok bool) {
	var results []StoreLinkBatchEventRecord

	payloadStr, isString := msg.Payload.Data.(string)
	if !isString {
		return nil, false
	}

	// Split by newlines to get individual records
	lines := strings.Split(payloadStr, "\n")
	// remove trailing \x00 from line
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, "\x00")
	}

	for _, line := range lines {
		// Skip empty lines
		if line == "" {
			continue
		} else if line == "\x0F" { // EOF marker
			break
		} else if line == "\x00" { // END marker
			break
		}

		// Capture the storage status and message
		resLinkRecord := &StoreLinkBatchEventRecord{}

		// Parse each line as tab-separated key=value pairs
		recordMap := make(map[string]string)
		fields := strings.Split(line, "\t")

		for _, field := range fields {
			if field == "" {
				continue
			}
			parts := strings.SplitN(field, "=", 2)
			if len(parts) == 2 {
				recordMap[parts[0]] = parts[1]
			}
		}

		// Parse status fields
		if status, exists := recordMap["_status"]; exists {
			resLinkRecord.Status = status
		}
		if statusInfo, exists := recordMap["_status_info"]; exists {
			resLinkRecord.Message = statusInfo
		} else if msg, exists := recordMap["_msg"]; exists {
			resLinkRecord.Message = msg
		}

		// Parse all LinkFields
		link := &resLinkRecord.LinkFields

		// Link IDs
		if uniqueId, exists := recordMap["unique_id"]; exists {
			link.UniqueId = uniqueId
		}
		if eventId, exists := recordMap["event_id"]; exists {
			link.Id = eventId
		}

		// Owner fields
		if ownerUniqueId, exists := recordMap["owner_unique_id"]; exists {
			link.OwnerUniqueID = ownerUniqueId
		}
		if ownerId, exists := recordMap["owner_id"]; exists {
			link.OwnerID = ownerId
		} else if ownerId, exists := recordMap["owner_event_id"]; exists {
			link.OwnerID = ownerId
		}
		if owner, exists := recordMap["owner"]; exists {
			link.Owner = owner
		}

		// Timestamp
		if timestamp, exists := recordMap["timestamp"]; exists {
			link.Timestamp = timestamp
		}

		// Location fields
		if loc, exists := recordMap["loc"]; exists {
			link.Location = loc
		}
		if locDelim, exists := recordMap["loc_delim"]; exists {
			link.LocationSeparator = locDelim
		}

		// Type
		if linkType, exists := recordMap["type"]; exists {
			link.Type = linkType
		}

		// Event A/B fields
		if eventA, exists := recordMap["event_id_a"]; exists {
			link.EventA = eventA
		}
		if eventB, exists := recordMap["event_id_b"]; exists {
			link.EventB = eventB
		}
		if uniqueIdA, exists := recordMap["unique_id_a"]; exists {
			link.UniqueIdA = uniqueIdA
		}
		if uniqueIdB, exists := recordMap["unique_id_b"]; exists {
			link.UniqueIdB = uniqueIdB
		}

		// Strength fields
		if strengthA, exists := recordMap["strength_a"]; exists {
			if s, err := strconv.ParseFloat(strengthA, 64); err == nil {
				link.StrengthA = s
			}
		}
		if strengthB, exists := recordMap["strength_b"]; exists {
			if s, err := strconv.ParseFloat(strengthB, 64); err == nil {
				link.StrengthB = s
			}
		}

		// Category
		if category, exists := recordMap["category"]; exists {
			link.Category = category
		}

		results = append(results, *resLinkRecord)
	}

	return results, true
}
