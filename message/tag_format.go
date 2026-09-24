package message

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// nullOwnerKeyPrefix is the event key Pod-OS reports as the owner of tags that have no
// owning event (e.g. tags created under the $sys owner).
const nullOwnerKeyPrefix = "+0000000000.000000"

// parseEventTagHeader parses one GetEvent response header field carrying a tag.
//
//	tag_format=0: event_tag:nnnnnnnnn:fffffffff=key=value
//	tag_format=1: event_tag:nnnnnnnnn:fffffffff:ssssssssss.uuuuuu[:owner_id]=key=value
//
// The owner is split off with SplitN so owner IDs containing ':' stay intact.
func parseEventTagHeader(name, value string) (TagOutput, bool) {
	rest, ok := strings.CutPrefix(name, "event_tag:")
	if !ok {
		return TagOutput{}, false
	}
	parts := strings.SplitN(rest, ":", 4)
	if len(parts) < 2 {
		return TagOutput{}, false
	}

	tag := TagOutput{Frequency: 1}
	tag.TagNumber, _ = strconv.Atoi(parts[0])
	if freq, err := strconv.Atoi(parts[1]); err == nil {
		tag.Frequency = freq
	}
	if len(parts) >= 3 {
		tag.Timestamp = parts[2]
	}
	if len(parts) == 4 {
		tag.Owner = normalizeTagOwner(parts[3])
	}
	tag.Key, tag.Value = splitTagKeyValue(value)
	return tag, true
}

// sortTagsByNumber orders GetEvent tags by their database tag counter.
func sortTagsByNumber(tags []TagOutput) {
	sort.SliceStable(tags, func(i, j int) bool { return tags[i].TagNumber < tags[j].TagNumber })
}

// splitTagKeyValue splits a "key=value" tag string. A string without a key keeps the
// whole text as the value.
func splitTagKeyValue(s string) (key, value string) {
	if eqIdx := strings.Index(s, "="); eqIdx > 0 {
		return s[:eqIdx], s[eqIdx+1:]
	}
	return "", s
}

// normalizeTagOwner maps Pod-OS "no owner" markers to an empty string.
func normalizeTagOwner(owner string) string {
	if owner == "NULL" || strings.HasPrefix(owner, nullOwnerKeyPrefix) {
		return ""
	}
	return owner
}

// normalizeTagOwnerLines repairs buffer_format=1 payloads requested with get_tag_owner or
// get_tag_owner_unique_id. Pod-OS writes each tag's owner after the tag line's newline, so
// it runs into the next record:
//
//	_event_tag=K\t...\ttag_timestamp=T\n\towner=O_event_tag=K\t...
//
// Each owner is rejoined to the tag line it belongs to and the record break is restored.
// Payloads already in the documented form (owner before the newline) are unchanged.
func normalizeTagOwnerLines(payload string) string {
	if !strings.Contains(payload, "\n\towner=") {
		return payload
	}
	payload = strings.ReplaceAll(payload, "\n\towner=", "\towner=")

	const marker = "\towner="
	var b strings.Builder
	b.Grow(len(payload) + 64)
	for {
		i := strings.Index(payload, marker)
		if i < 0 {
			b.WriteString(payload)
			break
		}
		b.WriteString(payload[:i+len(marker)])
		payload = payload[i+len(marker):]

		owner := payload
		if end := strings.IndexAny(owner, "\t\n"); end >= 0 {
			owner = owner[:end]
		}
		if j := strings.Index(owner, "_event_tag="); j >= 0 {
			b.WriteString(payload[:j])
			b.WriteByte('\n')
			payload = payload[j:]
		}
	}
	return b.String()
}

// parseEventTagLine parses a standalone buffer_format=1 tag line:
//
//	_event_tag=<event key>\ttag_freq=n\ttag_value=key=value\ttag_timestamp=s.u[\towner=...]
func parseEventTagLine(line string) (eventKey string, tag *TagOutput) {
	recordMap := parseTabDelimitedLine(line)
	eventKey = recordMap["_event_tag"]
	if eventKey == "" {
		return "", nil
	}
	return eventKey, parseEventTagPayloadField(recordMap)
}

// parsePosixTimestamp parses a Pod-OS "[+|-]ssssssssss.uuuuuu" timestamp into UTC.
func parsePosixTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	neg := false
	switch s[0] {
	case '+':
		s = s[1:]
	case '-':
		neg = true
		s = s[1:]
	}
	secStr, fracStr, _ := strings.Cut(s, ".")
	sec, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	var usec int64
	if fracStr != "" {
		if len(fracStr) > 6 {
			fracStr = fracStr[:6]
		}
		fracStr += strings.Repeat("0", 6-len(fracStr))
		if usec, err = strconv.ParseInt(fracStr, 10, 64); err != nil {
			return time.Time{}, false
		}
	}
	if neg {
		sec, usec = -sec, -usec
	}
	return time.Unix(sec, usec*1000).UTC(), true
}

// tagOwnerOutputOf returns the TagOwnerOutput requested by a GetEvent or GetEventsForTags message.
func tagOwnerOutputOf(msg *Message) TagOwnerOutput {
	switch msg.Intent.Name {
	case IntentType.GetEvent.Name:
		if opts := msg.GetEventOpts(); opts != nil {
			return opts.TagOwnerOutput
		}
	case IntentType.GetEventsForTags.Name:
		if opts := msg.GetEventsForTagsOpts(); opts != nil {
			return opts.TagOwnerOutput
		}
	}
	return TagOwnerNone
}

// ApplyTagOwnerOutput moves decoded tag owners from TagOutput.Owner to TagOutput.OwnerUniqueID
// when req asked for owners by unique ID (TagOwnerUniqueID). Pod-OS responses do not say which
// owner form they carry, so DecodeMessage always fills Owner. Client.SendMessage applies this
// automatically; call it yourself when decoding raw responses with DecodeMessage.
func ApplyTagOwnerOutput(req, resp *Message) {
	if req == nil || resp == nil || tagOwnerOutputOf(req) != TagOwnerUniqueID {
		return
	}
	move := func(tags []TagOutput) {
		for i := range tags {
			if tags[i].Owner != "" {
				tags[i].OwnerUniqueID = tags[i].Owner
				tags[i].Owner = ""
			}
		}
	}
	if resp.Event != nil {
		move(resp.Event.Tags)
	}
	if resp.Response != nil {
		for i := range resp.Response.EventRecords {
			move(resp.Response.EventRecords[i].Tags)
		}
	}
}
