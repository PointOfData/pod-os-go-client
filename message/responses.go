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

// ParseEventsForTagsBufferedPayload parses the buffered payload for GetEventsForTags response
// The payload contains tab-separated field=value pairs, newline-terminated records
func parseGetEventsForTagsPayload(msg *Message) (eventResults []EventFields, ok bool) {
	var results []EventFields

	// Split by newlines to get individual records
	lines := strings.Split(msg.Payload.Data.(string), "\n")

	for _, line := range lines {
		// Output the processed line
		resEvent := &EventFields{}

		// Skip empty lines
		if line == "" {
			continue
		} else if line == "\x0F" { // EOF marker
			break
		} else if line == "\x00" { // END marker
			break
		}

		// Parse each line as an equals-sign separated key=value pairs
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

		// Extract Event fields from the payload
		resEvent, ok := decodeEventFields(recordMap, resEvent)
		if !ok {
			return nil, false
		}

		// Parse tags from the record (may have multiple tag fields)
		tagOutputList := make([]TagOutput, 0)
		for key, value := range recordMap {
			if strings.HasPrefix(key, "tag:") {
				// parse this example: tag:1:namespace=pod-os-com
				parts := strings.Split(key, ":")
				if len(parts) == 3 {
					freq, err := strconv.Atoi(parts[1])
					if err != nil {
						continue
					}
					tagOutputList = append(tagOutputList, TagOutput{
						Frequency: freq,
						Key:       parts[2],
						Value:     value,
					})
				}
			}
		}
		resEvent.Tags = tagOutputList

		// Get EventUniqueId from the tagOutputList
		for _, tag := range tagOutputList {
			if tag.Key == "_unique_id" {
				resEvent.UniqueId = tag.Value
				break
			} else if tag.Key == "unique_id" {
				resEvent.UniqueId = tag.Value
				break
			}
		}

		// TODO: process links from the record (may have multiple link fields)
		/* 		linkOutputList := make([]LinkFields, 0)
		   		for key, value := range recordMap {
		   			if strings.HasPrefix(key, "link:") {
		   				// parse this example: link:1:event_id_a=event1|event_id_b=event2
		   				parts := strings.Split(key, ":")
		   				if len(parts) == 3 {
		   					linkOutputList = append(linkOutputList, LinkFields{
		   						EventA: parts[2],
		   						EventB: parts[3],
		   					})
		   				}
		   			}
		   		}
		   		resEvent.Links = linkOutputList */

		results = append(results, *resEvent)
	}

	return results, true
}

// ParseStoreBatchEventsPayload parses the payload for StoreBatchEvents response
func parseStoreBatchEventsPayload(msg *Message) (storeBatchResults []StoreBatchEventRecord, ok bool) {
	var results []StoreBatchEventRecord

	// Split by newlines to get individual records
	lines := strings.Split(msg.Payload.Data.(string), "\n")

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

// ParseLinkEventBatchPayload parses the payload for LinkEventBatch response
func parseLinkEventBatchPayload(msg *Message) (storeLinkBatchResults []StoreLinkBatchEventRecord, ok bool) {
	var results []StoreLinkBatchEventRecord

	// Split by newlines to get individual records
	lines := strings.Split(msg.Payload.Data.(string), "\n")
	for _, line := range lines {
		// Capture the storage status and message
		resLinkRecord := &StoreLinkBatchEventRecord{}

		// Skip empty lines
		if line == "" {
			continue
		} else if line == "\x0F" { // EOF marker
			break
		} else if line == "\x00" { // END marker
			break
		}

		// Parse each line as an equals-sign separated key=value pairs
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

		// Extract LinkFields from the payload
		_, ok := decodeLinkEventFields(recordMap, &resLinkRecord.LinkFields)
		if !ok {
			return nil, false
		}

		results = append(results, *resLinkRecord)
	}

	return results, true
}
