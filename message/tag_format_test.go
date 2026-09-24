package message

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixtures in testdata/tag_format are raw Pod-OS response frames captured live from a
// kind cluster (see knowledge/docs/intent_field_validation_f3381d2a.plan.md, "Tag formats").
const (
	fixtureOwnerKey   = "+1790266973.206042\x01TERRA\x0247.6\x02-122.5"
	fixtureTargetKey  = "+1790266973.420335\x01TERRA\x0247.6\x02-122.5"
	fixtureOwnedKey   = "+1790266973.723095\x01TERRA\x0247.6\x02-122.5"
	fixtureOwnerUID   = "tagprobe-owner-dlnoodm3eox6"
	fixtureTargetUID  = "tagprobe-target-dlnoodm3eox6"
	fixtureOwnedUID   = "tagprobe-owned-dlnoodm3eox6"
	fixtureProbeValue = "dlnoodm3eox6"
)

func decodeTagFixture(t *testing.T, name string) *Message {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "tag_format", name+".bin"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	msg, err := DecodeMessage(raw)
	if err != nil {
		t.Fatalf("DecodeMessage(%s): %v", name, err)
	}
	return msg
}

func findTag(tags []TagOutput, key, value string) []TagOutput {
	var out []TagOutput
	for _, tag := range tags {
		if tag.Key == key && (value == "" || tag.Value == value) {
			out = append(out, tag)
		}
	}
	return out
}

func eventByKey(t *testing.T, msg *Message, key string) EventFields {
	t.Helper()
	for _, ev := range msg.Response.EventRecords {
		if ev.Id == key {
			return ev
		}
	}
	t.Fatalf("event %q not found in %d records", key, len(msg.Response.EventRecords))
	return EventFields{}
}

func TestDecode_GetEvent_TagFormat0(t *testing.T) {
	msg := decodeTagFixture(t, "get_event_tag_format_0")
	tags := msg.Event.Tags
	if len(tags) != 36 {
		t.Fatalf("len(tags) = %d, want 36", len(tags))
	}
	for i, tag := range tags {
		if tag.TagNumber != i+1 {
			t.Fatalf("tags[%d].TagNumber = %d, want %d (tags must be ordered by tag number)", i, tag.TagNumber, i+1)
		}
		if tag.Timestamp != "" || tag.Owner != "" {
			t.Errorf("tag_format=0 tag %d has Timestamp=%q Owner=%q, want empty", i, tag.Timestamp, tag.Owner)
		}
	}
	color := findTag(tags, "color", "red")
	if len(color) != 1 || color[0].Frequency != 3 || color[0].TagNumber != 18 {
		t.Errorf("color tag = %+v, want freq 3, tag number 18", color)
	}
	if got := findTag(tags, "size", ""); len(got) != 2 || got[0].Value != "a:b=c" {
		t.Errorf("size tags = %+v, want two with value a:b=c", got)
	}
}

func TestDecode_GetEvent_TagFormat1(t *testing.T) {
	for _, name := range []string{"get_event_tag_format_1", "get_event_tag_format_1_output_tag_owner_y"} {
		t.Run(name, func(t *testing.T) {
			msg := decodeTagFixture(t, name)
			tags := msg.Event.Tags
			if len(tags) != 36 {
				t.Fatalf("len(tags) = %d, want 36", len(tags))
			}
			if len(msg.Response.EventRecords) != 1 || len(msg.Response.EventRecords[0].Tags) != 36 {
				t.Fatalf("EventRecords[0].Tags not populated")
			}
			size := findTag(tags, "size", "a:b=c")
			if len(size) != 2 {
				t.Fatalf("size tags = %+v", size)
			}
			if size[0].TagNumber != 19 || size[0].Frequency != 5 || size[0].Timestamp != "1790266974.325930" {
				t.Errorf("size[0] = %+v", size[0])
			}
			if size[1].Timestamp != "1790266974.325980" {
				t.Errorf("size[1].Timestamp = %q", size[1].Timestamp)
			}
			ts, ok := size[0].Time()
			if !ok || !ts.Equal(time.Unix(1790266974, 325930000)) || ts.Location() != time.UTC {
				t.Errorf("Time() = %v, %v", ts, ok)
			}
			w := findTag(tags, "$W", "")
			if len(w) != 1 || w[0].Value != "TERRA\x0247.6\x02-122.5" || w[0].TagNumber != 1 {
				t.Errorf("$W tag = %+v", w)
			}
			// This Pod-OS build omits the owner segment on GetEvent even with output_tag_owner=Y.
			for _, tag := range tags {
				if tag.Owner != "" {
					t.Errorf("unexpected owner %q on %s", tag.Owner, tag.Key)
				}
			}
		})
	}
}

func TestParseEventTagHeader_Formats(t *testing.T) {
	tests := []struct {
		name, header, value string
		want                TagOutput
	}{
		{"format 0", "event_tag:000000017:4", "k=v",
			TagOutput{TagNumber: 17, Frequency: 4, Key: "k", Value: "v"}},
		{"format 1 without owner", "event_tag:000000002:3:1790266974.325930", "k=a=b",
			TagOutput{TagNumber: 2, Frequency: 3, Timestamp: "1790266974.325930", Key: "k", Value: "a=b"}},
		{"format 1 with event key owner", "event_tag:000000003:1:1790266974.000001:" + fixtureOwnerKey, "k=v",
			TagOutput{TagNumber: 3, Frequency: 1, Timestamp: "1790266974.000001", Owner: fixtureOwnerKey, Key: "k", Value: "v"}},
		{"format 1 owner containing colons", "event_tag:000000004:1:1790266974.000001:urn:a:b", "k=v",
			TagOutput{TagNumber: 4, Frequency: 1, Timestamp: "1790266974.000001", Owner: "urn:a:b", Key: "k", Value: "v"}},
		{"format 1 NULL owner", "event_tag:000000005:1:1790266974.000001:NULL", "k=v",
			TagOutput{TagNumber: 5, Frequency: 1, Timestamp: "1790266974.000001", Key: "k", Value: "v"}},
		{"format 1 null event key owner", "event_tag:000000006:1:1790266974.000001:+0000000000.000000\x01000000.000000", "k=v",
			TagOutput{TagNumber: 6, Frequency: 1, Timestamp: "1790266974.000001", Key: "k", Value: "v"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseEventTagHeader(tt.header, tt.value)
			if !ok || got != tt.want {
				t.Errorf("parseEventTagHeader() = %+v, %v; want %+v", got, ok, tt.want)
			}
		})
	}
	if _, ok := parseEventTagHeader("unique_id", "x"); ok {
		t.Error("non-tag header parsed as tag")
	}
}

func TestDecode_GetEventsForTags_BufferFormat0(t *testing.T) {
	msg := decodeTagFixture(t, "events_for_tag_buffer_format_0")
	if len(msg.Response.EventRecords) != 2 {
		t.Fatalf("records = %d, want 2", len(msg.Response.EventRecords))
	}
	target := eventByKey(t, msg, fixtureTargetKey)
	if len(target.Tags) != 21 || target.UniqueId != fixtureTargetUID {
		t.Errorf("target: %d tags, unique id %q", len(target.Tags), target.UniqueId)
	}
	for _, tag := range target.Tags {
		if tag.Timestamp != "" {
			t.Errorf("buffer_format=0 tag %s has timestamp %q", tag.Key, tag.Timestamp)
		}
	}
}

func TestDecode_GetEventsForTags_BufferFormat1(t *testing.T) {
	msg := decodeTagFixture(t, "events_for_tag_buffer_format_1")
	if len(msg.Response.EventRecords) != 2 {
		t.Fatalf("records = %d, want 2", len(msg.Response.EventRecords))
	}
	target := eventByKey(t, msg, fixtureTargetKey)
	owned := eventByKey(t, msg, fixtureOwnedKey)
	if len(target.Tags) != 21 || len(owned.Tags) != 18 {
		t.Fatalf("tag counts: target %d (want 21), owned %d (want 18)", len(target.Tags), len(owned.Tags))
	}
	if target.UniqueId != fixtureTargetUID || owned.UniqueId != fixtureOwnedUID {
		t.Errorf("unique ids = %q, %q", target.UniqueId, owned.UniqueId)
	}
	color := findTag(target.Tags, "color", "red")
	if len(color) != 1 || color[0].Frequency != 3 || color[0].Timestamp != "1790266973.722260" || color[0].Owner != "" {
		t.Errorf("color = %+v", color)
	}
	probe := findTag(owned.Tags, "probe_sfx", fixtureProbeValue)
	if len(probe) != 1 || probe[0].Frequency != 4 || probe[0].Timestamp != "1790266974.024690" {
		t.Errorf("probe_sfx = %+v", probe)
	}
}

func TestDecode_GetEventsForTags_BufferFormat1_OwnerEventKey(t *testing.T) {
	msg := decodeTagFixture(t, "events_for_tag_buffer_format_1_get_tag_owner")
	target := eventByKey(t, msg, fixtureTargetKey)
	owned := eventByKey(t, msg, fixtureOwnedKey)
	if len(target.Tags) != 21 || len(owned.Tags) != 18 {
		t.Fatalf("tag counts: target %d, owned %d", len(target.Tags), len(owned.Tags))
	}
	if c := findTag(target.Tags, "color", "red"); len(c) != 1 || c[0].Owner != "" {
		t.Errorf("$sys-owned color tag owner = %+v, want empty", c)
	}
	for _, s := range findTag(target.Tags, "size", "a:b=c") {
		if s.Owner != fixtureOwnerKey {
			t.Errorf("size owner = %q, want %q", s.Owner, fixtureOwnerKey)
		}
	}
	if u := findTag(target.Tags, "_unique_id", ""); len(u) != 1 || u[0].Owner != fixtureTargetKey {
		t.Errorf("_unique_id owner = %+v, want %q", u, fixtureTargetKey)
	}
	if w := findTag(owned.Tags, "$W", ""); len(w) != 1 || w[0].Owner != fixtureOwnerKey || w[0].Timestamp != "1790266974.024420" {
		t.Errorf("owned $W = %+v", w)
	}
	for _, tag := range append(target.Tags, owned.Tags...) {
		if strings.Contains(tag.Owner, "_event_tag") || strings.Contains(tag.Timestamp, "owner") {
			t.Fatalf("owner segment not separated from next record: %+v", tag)
		}
	}
}

func TestDecode_GetEventsForTags_BufferFormat1_OwnerUniqueID(t *testing.T) {
	req := &Message{
		Envelope: Envelope{Intent: IntentType.GetEventsForTags},
		NeuralMemory: &NeuralMemoryFields{GetEventsForTags: &GetEventsForTagsOptions{
			BufferFormat: "1", TagOwnerOutput: TagOwnerUniqueID,
		}},
	}
	msg := decodeTagFixture(t, "events_for_tag_buffer_format_1_get_tag_owner_unique_id")
	ApplyTagOwnerOutput(req, msg)

	target := eventByKey(t, msg, fixtureTargetKey)
	owned := eventByKey(t, msg, fixtureOwnedKey)
	if c := findTag(target.Tags, "color", "red"); len(c) != 1 || c[0].Owner != "" || c[0].OwnerUniqueID != "" {
		t.Errorf("NULL owner not normalized: %+v", c)
	}
	for _, s := range findTag(target.Tags, "size", "a:b=c") {
		if s.OwnerUniqueID != fixtureOwnerUID || s.Owner != "" {
			t.Errorf("size = %+v, want OwnerUniqueID %q", s, fixtureOwnerUID)
		}
	}
	if u := findTag(owned.Tags, "_unique_id", ""); len(u) != 1 || u[0].OwnerUniqueID != fixtureOwnedUID {
		t.Errorf("owned _unique_id = %+v", u)
	}
}

func TestApplyTagOwnerOutput_NoopForEventKey(t *testing.T) {
	req := &Message{
		Envelope:     Envelope{Intent: IntentType.GetEvent},
		NeuralMemory: &NeuralMemoryFields{GetEvent: &GetEventOptions{TagOwnerOutput: TagOwnerEventKey}},
	}
	resp := &Message{Event: &EventFields{Tags: []TagOutput{{Key: "k", Owner: fixtureOwnerKey}}}}
	ApplyTagOwnerOutput(req, resp)
	if resp.Event.Tags[0].Owner != fixtureOwnerKey || resp.Event.Tags[0].OwnerUniqueID != "" {
		t.Errorf("tags = %+v", resp.Event.Tags)
	}
}

func TestNormalizeTagOwnerLines(t *testing.T) {
	spec := "_event_tag=E\ttag_freq=1\ttag_value=a=b\ttag_timestamp=1.000001\towner=O1\n" +
		"_event_tag=E\ttag_freq=2\ttag_value=c=d\ttag_timestamp=1.000002\towner=O2\n"
	if got := normalizeTagOwnerLines(spec); got != spec {
		t.Errorf("documented form changed:\n%q", got)
	}

	server := "_event_id=E\n_event_tag=E\ttag_freq=1\ttag_value=a=b\ttag_timestamp=1.000001\n" +
		"\towner=O1_event_tag=E\ttag_freq=2\ttag_value=c=d\ttag_timestamp=1.000002\n\towner=O2\n\n"
	want := "_event_id=E\n_event_tag=E\ttag_freq=1\ttag_value=a=b\ttag_timestamp=1.000001\towner=O1\n" +
		"_event_tag=E\ttag_freq=2\ttag_value=c=d\ttag_timestamp=1.000002\towner=O2\n\n"
	if got := normalizeTagOwnerLines(server); got != want {
		t.Errorf("normalizeTagOwnerLines()\n got %q\nwant %q", got, want)
	}
}

func TestParsePosixTimestamp(t *testing.T) {
	tests := []struct {
		in   string
		want time.Time
		ok   bool
	}{
		{"1790266974.325930", time.Unix(1790266974, 325930000), true},
		{"+1790266974.5", time.Unix(1790266974, 500000000), true},
		{"1790266974", time.Unix(1790266974, 0), true},
		{"", time.Time{}, false},
		{"abc", time.Time{}, false},
	}
	for _, tt := range tests {
		got, ok := parsePosixTimestamp(tt.in)
		if ok != tt.ok || (ok && !got.Equal(tt.want)) {
			t.Errorf("parsePosixTimestamp(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestGetEventMessageHeader_TagFormat(t *testing.T) {
	build := func(opts *GetEventOptions) string {
		return GetEventMessageHeader(&Message{Event: &EventFields{Id: "e1"}, NeuralMemory: &NeuralMemoryFields{GetEvent: opts}})
	}
	if h := build(&GetEventOptions{GetTags: true}); !strings.Contains(h, "tag_format=0\t") || strings.Contains(h, "output_tag_owner") {
		t.Errorf("default header = %q", h)
	}
	tf1 := NullInt{Value: 1, Valid: true}
	if h := build(&GetEventOptions{GetTags: true, TagFormat: tf1}); !strings.Contains(h, "tag_format=1\t") {
		t.Errorf("tag_format=1 header = %q", h)
	}
	if h := build(&GetEventOptions{TagFormat: tf1, TagOwnerOutput: TagOwnerEventKey}); !strings.Contains(h, "output_tag_owner=Y\t") {
		t.Errorf("event key owner header = %q", h)
	}
	if h := build(&GetEventOptions{TagFormat: tf1, TagOwnerOutput: TagOwnerUniqueID}); !strings.Contains(h, "output_tag_owner=N\t") {
		t.Errorf("unique id owner header = %q", h)
	}
}

func TestGetEventsForTagMessageHeader_TagOwner(t *testing.T) {
	build := func(opts *GetEventsForTagsOptions) string {
		return GetEventsForTagMessageHeader(&Message{NeuralMemory: &NeuralMemoryFields{GetEventsForTags: opts}})
	}
	if h := build(&GetEventsForTagsOptions{BufferFormat: "1"}); !strings.Contains(h, "buffer_format=1\t") || strings.Contains(h, "get_tag_owner") {
		t.Errorf("no-owner header = %q", h)
	}
	if h := build(&GetEventsForTagsOptions{BufferFormat: "1", TagOwnerOutput: TagOwnerEventKey}); !strings.Contains(h, "get_tag_owner=Y\t") {
		t.Errorf("event key header = %q", h)
	}
	if h := build(&GetEventsForTagsOptions{BufferFormat: "1", TagOwnerOutput: TagOwnerUniqueID}); !strings.Contains(h, "get_tag_owner_unique_id=Y\t") {
		t.Errorf("unique id header = %q", h)
	}
}

func TestValidate_TagFormat(t *testing.T) {
	enable := validationEnabled
	validationEnabled = true
	defer func() { validationEnabled = enable }()

	getEvent := func(opts *GetEventOptions) *Message {
		return &Message{
			Envelope:     Envelope{To: "test@zeroth.pod-os.com", From: "c@zeroth.pod-os.com", Intent: IntentType.GetEvent},
			Event:        &EventFields{Id: "e1"},
			NeuralMemory: &NeuralMemoryFields{GetEvent: opts},
		}
	}
	eventsForTags := func(opts *GetEventsForTagsOptions) *Message {
		return &Message{
			Envelope:     Envelope{To: "test@zeroth.pod-os.com", From: "c@zeroth.pod-os.com", Intent: IntentType.GetEventsForTags},
			Payload:      &PayloadFields{Data: "clause_type:S\tboolean:or\tlow:k=v"},
			NeuralMemory: &NeuralMemoryFields{GetEventsForTags: opts},
		}
	}
	tf1 := NullInt{Value: 1, Valid: true}

	tests := []struct {
		name      string
		msg       *Message
		wantField string
		wantSev   string
	}{
		{"valid tag_format=1 with owner", getEvent(&GetEventOptions{GetTags: true, TagFormat: tf1, TagOwnerOutput: TagOwnerEventKey}), "", ""},
		{"tag_format=2", getEvent(&GetEventOptions{GetTags: true, TagFormat: NullInt{Value: 2, Valid: true}}), "NeuralMemory.GetEvent.TagFormat", "error"},
		{"owner without tag_format=1", getEvent(&GetEventOptions{GetTags: true, TagOwnerOutput: TagOwnerUniqueID}), "NeuralMemory.GetEvent.TagOwnerOutput", "error"},
		{"invalid owner value", getEvent(&GetEventOptions{GetTags: true, TagFormat: tf1, TagOwnerOutput: "owner"}), "NeuralMemory.GetEvent.TagOwnerOutput", "error"},
		{"tag_format=1 without get_tags", getEvent(&GetEventOptions{TagFormat: tf1}), "NeuralMemory.GetEvent.TagFormat", "warn"},
		{"valid buffer_format=1 with owner", eventsForTags(&GetEventsForTagsOptions{BufferResults: true, BufferFormat: "1", TagOwnerOutput: TagOwnerUniqueID}), "", ""},
		{"buffer_format=2", eventsForTags(&GetEventsForTagsOptions{BufferResults: true, BufferFormat: "2"}), "NeuralMemory.GetEventsForTags.BufferFormat", "error"},
		{"owner without buffer_format=1", eventsForTags(&GetEventsForTagsOptions{BufferResults: true, TagOwnerOutput: TagOwnerEventKey}), "NeuralMemory.GetEventsForTags.TagOwnerOutput", "warn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.msg.Validate()
			if tt.wantField == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected validation errors:\n%s", errs.Error())
				}
				socket, err := EncodeMessage(tt.msg, "conv")
				if err != nil {
					t.Fatal(err)
				}
				if raw := ValidateRawMessage(socket.MessageBytes); len(raw) != 0 {
					t.Fatalf("unexpected wire validation errors:\n%s", raw.Error())
				}
				return
			}
			for _, e := range errs {
				if e.Field == tt.wantField && e.Severity == tt.wantSev {
					if e.Fix == "" || e.ExampleCode == "" {
						t.Errorf("error lacks fix/example: %+v", e)
					}
					return
				}
			}
			t.Fatalf("want %s on %s, got:\n%s", tt.wantSev, tt.wantField, errs.Error())
		})
	}
}

func TestValidateRawMessage_TagFormatHeaders(t *testing.T) {
	enable := validationEnabled
	validationEnabled = true
	defer func() { validationEnabled = enable }()

	msg := &Message{
		Envelope:     Envelope{To: "test@zeroth.pod-os.com", From: "c@zeroth.pod-os.com", Intent: IntentType.GetEventsForTags},
		Payload:      &PayloadFields{Data: "clause_type:S\tboolean:or\tlow:k=v"},
		NeuralMemory: &NeuralMemoryFields{GetEventsForTags: &GetEventsForTagsOptions{BufferResults: true, BufferFormat: "1"}},
	}
	socket, err := EncodeMessage(msg, "conv")
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Replace(string(socket.MessageBytes), "buffer_format=1\t", "buffer_format=1\tget_tag_owner=yes\t", 1)
	raw = reframeHeaderLength(t, raw, len("get_tag_owner=yes\t"))
	errs := ValidateRawMessage([]byte(raw))
	found := false
	for _, e := range errs {
		if e.WireField == "get_tag_owner" && e.Rule == "header_value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected get_tag_owner header_value error, got:\n%s", errs.Error())
	}
}

// reframeHeaderLength grows the total and header length prefixes of an encoded frame by delta.
func reframeHeaderLength(t *testing.T, raw string, delta int) string {
	t.Helper()
	total, err := decodeMessageSizeParam([]byte(raw[0:9]))
	if err != nil {
		t.Fatal(err)
	}
	header, err := decodeMessageSizeParam([]byte(raw[27:36]))
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("x%08x", total+int64(delta)) + raw[9:27] + fmt.Sprintf("x%08x", header+int64(delta)) + raw[36:]
}
