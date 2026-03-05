package message

import (
	"fmt"
)

const lengthsSize = 9 * 7

// ReplaceFromInRawMessage replaces only the From segment and updates fromLength, headerLength,
// and totalLength in the raw message so the byte-span lengths match the actual layout.
// The header begins immediately after From and must start with the same bytes (e.g. _db_cmd=).
// It does not decode or re-encode the message, so Intent, MessageType, header content, and
// payload are unchanged. Use this when fixing client-name mismatch to avoid changing Intent.
func ReplaceFromInRawMessage(raw []byte, newFrom string) ([]byte, error) {
	if len(raw) < lengthsSize {
		return nil, fmt.Errorf("message too short for length block (need %d bytes)", lengthsSize)
	}
	toLength, err := decodeMessageSizeParam(raw[9:18])
	if err != nil {
		return nil, fmt.Errorf("invalid toLength: %w", err)
	}
	fromLength, err := decodeMessageSizeParam(raw[18:27])
	if err != nil {
		return nil, fmt.Errorf("invalid fromLength: %w", err)
	}
	payloadLength, err := decodeMessageSizeParam(raw[54:63])
	if err != nil {
		return nil, fmt.Errorf("invalid payload length: %w", err)
	}

	fromStart := int(lengthsSize) + int(toLength)
	fromEnd := fromStart + int(fromLength)
	if fromEnd > len(raw) {
		return nil, fmt.Errorf("message too short for From segment")
	}
	// Header span: from fromEnd for (len(raw)-fromEnd)-payload bytes
	headerSpanLen := len(raw) - fromEnd - int(payloadLength)
	if headerSpanLen < 0 {
		return nil, fmt.Errorf("invalid layout: header span length would be negative")
	}

	newFromLen := len(newFrom)
	deltaFrom := newFromLen - int(fromLength)
	newTotal := int64(len(raw)) + int64(deltaFrom)
	if newTotal < 0 {
		return nil, fmt.Errorf("replacement From would make total length negative")
	}

	// Same encoding as encoder: "x" + 8 hex digits for lengths
	totalEncoded := "x" + fmt.Sprintf("%08x", newTotal)
	fromLenEncoded := "x" + fmt.Sprintf("%08x", newFromLen)
	headerLenEncoded := "x" + fmt.Sprintf("%08x", headerSpanLen)
	if len(totalEncoded) != 9 || len(fromLenEncoded) != 9 || len(headerLenEncoded) != 9 {
		return nil, fmt.Errorf("length encoding produced wrong size")
	}

	out := make([]byte, 0, len(raw)+deltaFrom)
	out = append(out, []byte(totalEncoded)...)
	out = append(out, raw[9:18]...)           // toLength unchanged
	out = append(out, []byte(fromLenEncoded)...)
	out = append(out, []byte(headerLenEncoded)...) // correct header byte-span length
	out = append(out, raw[36:fromStart]...)   // messageType, dataType, payloadLen + To
	out = append(out, newFrom...)
	out = append(out, raw[fromEnd:]...)       // header + payload
	return out, nil
}
