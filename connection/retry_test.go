package connection

import (
	"errors"
	"testing"
	"time"
)

func TestRetryExhaustedDoesNotPanic(t *testing.T) {
	// This test verifies that when retries are exhausted,
	// the function returns an error instead of panicking

	retry := NewRetry(Retry{
		Retries:           3,
		Backoff:           1 * time.Millisecond, // Fast for testing
		BackoffMultiplier: 1.0,                  // No exponential increase for faster test
	})

	callCount := 0
	expectedErr := errors.New("always fails")

	// Callback that always fails
	callback := func() (any, error) {
		callCount++
		return nil, expectedErr
	}

	// This should NOT panic
	result, err := retry.Retry(callback)

	// Verify the result
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Verify it's the correct error type
	var exhaustedErr *ErrRetriesExhausted
	if !errors.As(err, &exhaustedErr) {
		t.Errorf("Expected ErrRetriesExhausted, got %T: %v", err, err)
	}

	// Verify the attempt count (initial + retries = 4 total attempts)
	if exhaustedErr.Attempts != 4 {
		t.Errorf("Expected 4 attempts, got %d", exhaustedErr.Attempts)
	}

	// Verify the last error is wrapped
	if !errors.Is(exhaustedErr.LastErr, expectedErr) {
		t.Errorf("Expected last error to be %v, got %v", expectedErr, exhaustedErr.LastErr)
	}

	// Verify the callback was called the expected number of times
	if callCount != 4 {
		t.Errorf("Expected callback to be called 4 times, got %d", callCount)
	}
}

func TestRetrySucceedsOnFirstAttempt(t *testing.T) {
	retry := NewRetry(Retry{
		Retries:           3,
		Backoff:           1 * time.Millisecond,
		BackoffMultiplier: 1.0,
	})

	callCount := 0
	expectedResult := "success"

	callback := func() (any, error) {
		callCount++
		return expectedResult, nil
	}

	result, err := retry.Retry(callback)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != expectedResult {
		t.Errorf("Expected result %v, got %v", expectedResult, result)
	}

	if callCount != 1 {
		t.Errorf("Expected callback to be called once, got %d", callCount)
	}
}

func TestRetrySucceedsAfterFailures(t *testing.T) {
	retry := NewRetry(Retry{
		Retries:           5,
		Backoff:           1 * time.Millisecond,
		BackoffMultiplier: 1.0,
	})

	callCount := 0
	expectedResult := "success"

	// Fails first 2 times, succeeds on 3rd attempt
	callback := func() (any, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("temporary failure")
		}
		return expectedResult, nil
	}

	result, err := retry.Retry(callback)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if result != expectedResult {
		t.Errorf("Expected result %v, got %v", expectedResult, result)
	}

	if callCount != 3 {
		t.Errorf("Expected callback to be called 3 times, got %d", callCount)
	}
}

func TestErrRetriesExhaustedError(t *testing.T) {
	t.Run("with last error", func(t *testing.T) {
		lastErr := errors.New("connection refused")
		err := &ErrRetriesExhausted{
			Attempts: 5,
			LastErr:  lastErr,
		}

		expected := "retry attempts exhausted after 5 attempts: connection refused"
		if err.Error() != expected {
			t.Errorf("Expected error message %q, got %q", expected, err.Error())
		}

		// Test Unwrap
		if !errors.Is(err, lastErr) {
			t.Error("Expected Unwrap to return last error")
		}
	})

	t.Run("without last error", func(t *testing.T) {
		err := &ErrRetriesExhausted{
			Attempts: 3,
			LastErr:  nil,
		}

		expected := "retry attempts exhausted after 3 attempts"
		if err.Error() != expected {
			t.Errorf("Expected error message %q, got %q", expected, err.Error())
		}
	})
}

func TestRetryNilCallback(t *testing.T) {
	retry := NewRetry(Retry{
		Retries: 3,
	})

	result, err := retry.Retry(nil)

	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}

	if err == nil {
		t.Fatal("Expected error for nil callback")
	}

	if err.Error() != "callback is nil" {
		t.Errorf("Expected 'callback is nil' error, got %v", err)
	}
}
