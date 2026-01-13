package message

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// SocketFieldTag is the struct tag key for Pod-OS socket field names
const SocketFieldTag = "podos"

// FieldTransformer transforms a Go value to its socket representation
type FieldTransformer func(interface{}) string

// DefaultTransformers provides common transformation functions
var DefaultTransformers = map[reflect.Kind]FieldTransformer{
	reflect.Bool: func(v interface{}) string {
		if b, ok := v.(bool); ok && b {
			return "Y"
		}
		return "N"
	},
	reflect.Int: func(v interface{}) string {
		if i, ok := v.(int); ok {
			return strconv.Itoa(i)
		}
		return "0"
	},
	reflect.String: func(v interface{}) string {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	},
}

// NullIntTransformer handles NullInt type
func NullIntTransformer(v interface{}) string {
	if ni, ok := v.(NullInt); ok {
		if !ni.Valid {
			return "1" // default
		}
		return strconv.Itoa(ni.Value)
	}
	return "1"
}

// HeaderBuilder builds Pod-OS socket headers from Message structs
type HeaderBuilder struct {
	intent string
}

// NewHeaderBuilder creates a new HeaderBuilder for the given intent
func NewHeaderBuilder(intent string) *HeaderBuilder {
	return &HeaderBuilder{intent: intent}
}

// BuildGetEventHeader constructs the socket header for GetEvent intent using the new composition structure
func (hb *HeaderBuilder) BuildGetEventHeader(msg *Message) string {
	var header strings.Builder

	// Add base command
	header.WriteString("_db_cmd=get\t")
	header.WriteString("id:name=" + msg.ClientName + "\t")

	// Get event fields from composition
	if msg.Event != nil {
		if msg.Event.Id != "" {
			header.WriteString("event=" + msg.Event.Id + "\t")
		}
		if msg.Event.UniqueId != "" {
			header.WriteString("unique_id=" + msg.Event.UniqueId + "\t")
		}
	}

	// Get GetEvent options from NeuralMemory
	if opts := msg.GetEventOpts(); opts != nil {
		// Use reflection to iterate over GetEventOptions fields
		v := reflect.ValueOf(*opts)
		t := reflect.TypeOf(*opts)

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			value := v.Field(i)

			// Check if field has podos tag
			socketName := field.Tag.Get(SocketFieldTag)
			if socketName == "" {
				continue
			}

			// Skip zero values for optional fields (except bools and NullInt)
			if !hb.shouldIncludeField(field, value) {
				continue
			}

			// Transform value based on type
			socketValue := hb.transformValue(field.Type, value.Interface())
			if socketValue != "" {
				header.WriteString(socketName + "=" + socketValue + "\t")
			}
		}
	}

	// Add message ID (generate if not provided)
	messageId := msg.MessageId
	if messageId == "" {
		messageId = uuid.New().String()
	}
	header.WriteString("_msg_id=" + messageId)

	return strings.TrimSuffix(header.String(), "\t")
}

// transformValue transforms a Go value to its socket representation
func (hb *HeaderBuilder) transformValue(fieldType reflect.Type, value interface{}) string {
	// Handle special types first
	if fieldType == reflect.TypeOf(NullInt{}) {
		return NullIntTransformer(value)
	}

	// Handle pointer types
	if fieldType.Kind() == reflect.Ptr {
		if reflect.ValueOf(value).IsNil() {
			return ""
		}
		fieldType = fieldType.Elem()
		value = reflect.ValueOf(value).Elem().Interface()
	}

	// Use default transformers
	if transformer, ok := DefaultTransformers[fieldType.Kind()]; ok {
		return transformer(value)
	}

	// Fallback: convert to string
	return fmt.Sprintf("%v", value)
}

// shouldIncludeField determines if a field should be included in the header
func (hb *HeaderBuilder) shouldIncludeField(field reflect.StructField, value reflect.Value) bool {
	// Always include bools (they have meaningful false values)
	if field.Type.Kind() == reflect.Bool {
		return true
	}

	// Always include NullInt (handled by transformer with default)
	if field.Type == reflect.TypeOf(NullInt{}) {
		return true
	}

	// For int fields, check if they have special inclusion rules
	if field.Type.Kind() == reflect.Int {
		// For RequestFormat, only include if value is 2
		if field.Name == "RequestFormat" {
			return value.Int() == 2
		}
		// For FirstLink, include if >= 0
		if field.Name == "FirstLink" {
			return value.Int() >= 0
		}
		// For LinkCount, include if > 0
		if field.Name == "LinkCount" {
			return value.Int() > 0
		}
		// Always include other ints (they'll be checked in transform)
		return true
	}

	// Skip zero values for strings
	if field.Type.Kind() == reflect.String {
		return value.String() != ""
	}

	// Default: include non-zero values
	return !value.IsZero()
}

// getCommand maps intent name to Pod-OS command
func (hb *HeaderBuilder) getCommand() string {
	commandMap := map[string]string{
		"GetEvent":         "get",
		"StoreEvent":       "store",
		"LinkEvents":       "link",
		"UnLinkEvents":     "unlink",
		"GetEventsForTags": "events_for_tag",
		"StoreBatchEvents": "store_batch",
	}
	if cmd, ok := commandMap[hb.intent]; ok {
		return cmd
	}
	return ""
}

// BuildGetEventsForTagsHeader constructs the socket header for GetEventsForTags intent using the new composition structure
func (hb *HeaderBuilder) BuildGetEventsForTagsHeader(msg *Message) string {
	var header strings.Builder

	// Add base command
	header.WriteString("_db_cmd=events_for_tag\t")
	header.WriteString("id:name=" + msg.ClientName + "\t")

	// Get GetEventsForTags options from NeuralMemory
	if opts := msg.GetEventsForTagsOpts(); opts != nil {
		// Use reflection to iterate over GetEventsForTagsOptions fields
		v := reflect.ValueOf(*opts)
		t := reflect.TypeOf(*opts)

		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			value := v.Field(i)

			// Check if field has podos tag
			socketName := field.Tag.Get(SocketFieldTag)
			if socketName == "" {
				continue
			}

			// Skip zero values for optional fields (except bools)
			if !hb.shouldIncludeEventsForTagsField(field, value) {
				continue
			}

			// Transform value based on type
			socketValue := hb.transformValue(field.Type, value.Interface())
			if socketValue != "" && socketValue != "N" { // Skip "N" for bools that are false
				header.WriteString(socketName + "=" + socketValue + "\t")
			}
		}

		// Add common fields from options
		if opts.BufferResults {
			header.WriteString("buffer_results=Y\t")
		} else {
			header.WriteString("buffer_results=N\t")
		}

		if opts.IncludeTagStats {
			header.WriteString("include_tag_stats=Y\t")
		}

		if opts.HitTagFilter != "" {
			header.WriteString("hit_tag_filter=" + opts.HitTagFilter + "\t")
		}

		if opts.InvertHitTagFilter {
			header.WriteString("invert_hit_tag_filter=Y\t")
		}

		bufferFormat := opts.BufferFormat
		if bufferFormat == "" {
			bufferFormat = "0"
		}
		header.WriteString("buffer_format=" + bufferFormat + "\t")
	} else {
		// Default buffer settings when no options provided
		header.WriteString("buffer_results=N\t")
		header.WriteString("buffer_format=0\t")
	}

	// Add message ID (generate if not provided)
	messageId := msg.MessageId
	if messageId == "" {
		messageId = uuid.New().String()
	}
	header.WriteString("_msg_id=" + messageId)

	return strings.TrimSuffix(header.String(), "\t")
}

// shouldIncludeEventsForTagsField determines if a GetEventsForTags field should be included in the header
func (hb *HeaderBuilder) shouldIncludeEventsForTagsField(field reflect.StructField, value reflect.Value) bool {
	// For bool fields, only include if true (they represent "Y" flags)
	if field.Type.Kind() == reflect.Bool {
		return value.Bool()
	}

	// For int fields, include if > 0 (except for EventsPerMessage which can be -1)
	if field.Type.Kind() == reflect.Int {
		if field.Name == "EventsPerMessage" {
			return value.Int() != 0 // Include if non-zero (can be -1 or positive)
		}
		return value.Int() > 0
	}

	// Skip zero values for strings
	if field.Type.Kind() == reflect.String {
		return value.String() != ""
	}

	// Default: include non-zero values
	return !value.IsZero()
}
