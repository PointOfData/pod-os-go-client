package message

import (
	"fmt"
	"time"
)

// GetTimestamp
// Returns the Now() timestamp in microseconds formatted as a string with 6 decimal places with + or - sign relative to January 1, 1970 00:00:00 UTC
func GetTimestamp() string {
	now := time.Now()
	timestamp := float64(now.UnixMicro()) / 1000000.0
	if timestamp > 0 {
		return fmt.Sprintf("+%.6f", timestamp)
	}
	return fmt.Sprintf("%.6f", timestamp)
}

// GetTimeStampFromTime
// Returns the timestamp from a time.Time object
// param t - the time.Time object
// return the timestamp in microseconds formatted as a string with 6 decimal places with + or - sign relative to January 1, 1970 00:00:00 UTC
// Example usages:
// Specific date/time: specificTime := time.Date(2024, time.December, 25, 15, 30, 45, 123456789, time.UTC)
// Parse a string: parsedTime, err := time.Parse(time.RFC3339, "2024-12-25T15:30:45.123456789Z")
func GetTimeStampFromTime(t time.Time) string {
	timestamp := float64(t.UnixMicro()) / 1000000.0
	if timestamp > 0 {
		return fmt.Sprintf("+%.6f", timestamp)
	}
	return fmt.Sprintf("%.6f", timestamp)
}
