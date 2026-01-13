package message

import (
	"errors"
	"fmt"
)

// ErrCode represents error codes for message encoding/decoding errors
type ErrCode uint32

const (
	ErrCodeDecodeMessageTooShort ErrCode = iota + 1000
	ErrCodeDecodeInvalidSizeParam
	ErrCodeDecodeInvalidHeader
	ErrCodeDecodeInvalidMessageType
	ErrCodeDecodeInvalidDataType
	ErrCodeDecodePayloadTooLarge
	ErrCodeDecodeHeaderTransformationFailed
	ErrCodeEncodeNilMessage
	ErrCodeEncodePayloadTooLarge
	ErrCodeEncodeInvalidData
	ErrCodeEncodeInvalidFromAddress
	ErrCodeEncodeInvalidGatewayName
	ErrCodeEncodeInvalidActorName
	ErrCodeEncodeInvalidDomainName
	ErrCodeEncodeInvalidToAddress
)

// DecodeError represents an error that occurred during message decoding
type DecodeError struct {
	Code    ErrCode
	Message string
	Field   string // Optional field name or context
	Err     error  // Original error for wrapping
}

// Error implements the error interface
func (e *DecodeError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("decode error [%d]: %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("decode error [%d]: %s", e.Code, e.Message)
}

// Unwrap returns the original error for error wrapping support
func (e *DecodeError) Unwrap() error {
	return e.Err
}

// NewDecodeError creates a new DecodeError
func NewDecodeError(code ErrCode, message string) *DecodeError {
	return &DecodeError{
		Code:    code,
		Message: message,
	}
}

// WrapDecodeError wraps an existing error in a DecodeError
func WrapDecodeError(code ErrCode, message string, err error) *DecodeError {
	return &DecodeError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// DecodeErrorWithField creates a DecodeError with field context
func DecodeErrorWithField(code ErrCode, message, field string) *DecodeError {
	return &DecodeError{
		Code:    code,
		Message: message,
		Field:   field,
	}
}

// EncodeError represents an error that occurred during message encoding
type EncodeError struct {
	Code    ErrCode
	Message string
	Field   string // Optional field name or context
	Err     error  // Original error for wrapping
}

// Error implements the error interface
func (e *EncodeError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("encode error [%d]: %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("encode error [%d]: %s", e.Code, e.Message)
}

// Unwrap returns the original error for error wrapping support
func (e *EncodeError) Unwrap() error {
	return e.Err
}

// NewEncodeError creates a new EncodeError
func NewEncodeError(code ErrCode, message string) *EncodeError {
	return &EncodeError{
		Code:    code,
		Message: message,
	}
}

// WrapEncodeError wraps an existing error in an EncodeError
func WrapEncodeError(code ErrCode, message string, err error) *EncodeError {
	return &EncodeError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// EncodeErrorWithField creates an EncodeError with field context
func EncodeErrorWithField(code ErrCode, message, field string) *EncodeError {
	return &EncodeError{
		Code:    code,
		Message: message,
		Field:   field,
	}
}

// IsDecodeError checks if an error is a DecodeError
func IsDecodeError(err error) bool {
	var decodeErr *DecodeError
	return errors.As(err, &decodeErr)
}

// IsEncodeError checks if an error is an EncodeError
func IsEncodeError(err error) bool {
	var encodeErr *EncodeError
	return errors.As(err, &encodeErr)
}
