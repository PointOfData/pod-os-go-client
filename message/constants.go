package message

// MaxMessageSizeBytes defines the maximum allowed size of a full Pod-OS message
// in bytes, including length prefix, header, tags, and payload.
//
// The wire-format length prefix is a 9-byte field that can express values up to
// ~999,999,999 bytes in decimal or 0xffffffff (~4GB) in hex. However, this
// client constrains the total message size to 2 GiB to avoid excessive memory
// usage and to provide a consistent upper bound for both encoding and decoding.
//
// MaxMessageSizeBytes is a var (not a const) so that tests can temporarily
// lower the limit to exercise oversize logic without allocating multi-gigabyte
// buffers. Application code should treat it as effectively constant.
var MaxMessageSizeBytes int64 = 2 * 1024 * 1024 * 1024 // 2 GiB

// MaxMessageSize returns the maximum allowed message size in bytes. Callers can
// use this helper to proactively validate data before constructing messages.
func MaxMessageSize() int64 {
	return MaxMessageSizeBytes
}

