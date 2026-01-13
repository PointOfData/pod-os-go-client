package connection

import (
	"fmt"
	"log"
	"math"
	"time"
)

const (
	BackoffMultiplierCap = 10
	BackoffDurationCap   = time.Minute
)

// ErrRetriesExhausted is returned when all retry attempts have been exhausted.
// This error wraps the last error encountered during retry attempts.
type ErrRetriesExhausted struct {
	Attempts int
	LastErr  error
}

func (e *ErrRetriesExhausted) Error() string {
	if e.LastErr != nil {
		return fmt.Sprintf("retry attempts exhausted after %d attempts: %v", e.Attempts, e.LastErr)
	}
	return fmt.Sprintf("retry attempts exhausted after %d attempts", e.Attempts)
}

func (e *ErrRetriesExhausted) Unwrap() error {
	return e.LastErr
}

type RetryCallback func() (any, error)

type IRetry interface {
	Retry(_ RetryCallback) (any, error)
}

type Retry struct {
	Retries            int
	Backoff            time.Duration
	BackoffMultiplier  float64
	DisableBackoffCaps bool
}

var _ IRetry = (*Retry)(nil)

// Retry runs the callback function and retries it if it fails.
// It'll wait for the duration of the backoff between retries.
// If all retries are exhausted, it returns an ErrRetriesExhausted error
// instead of panicking, allowing the application to handle the failure gracefully.
func (r *Retry) Retry(callback RetryCallback) (any, error) {
	var (
		object any
		err    error
		retry  int
	)

	if callback == nil {
		return nil, fmt.Errorf("callback is nil")
	}

	// If the number of retries is 0, just run the callback once (first attempt).
	if r == nil {
		return callback()
	}

	// The first attempt counts as a retry.
	for ; retry <= r.Retries; retry++ {
		// Wait for the backoff duration before retrying. The backoff duration is
		// calculated by multiplying the backoff duration by the backoff multiplier
		// raised to the power of the number of retries. For example, if the backoff
		// duration is 1 second and the backoff multiplier is 2, the backoff duration
		// will be 1 second, 2 seconds, 4 seconds, 8 seconds, etc. The backoff duration
		// is capped at 1 minute and the backoff multiplier is capped at 10, so the
		// backoff duration will be 1 minute after 6 retries. The backoff multiplier
		// is capped at 10 to prevent the backoff duration from growing too quickly,
		// unless the backoff caps are disabled.
		backoffDuration := r.Backoff * time.Duration(
			math.Pow(r.BackoffMultiplier, float64(retry)),
		)

		if !r.DisableBackoffCaps && backoffDuration > BackoffDurationCap {
			backoffDuration = BackoffDurationCap
		}

		if retry > 0 {
			log.Printf("Retry attempt %d, delay: %s", retry, backoffDuration.String())
		}

		// Try and retry the callback.
		object, err = callback()
		if err == nil {
			return object, nil
		}

		time.Sleep(backoffDuration)
	}

	// Log the failure but don't panic - return error for graceful handling
	log.Printf("WARNING: Retry attempts exhausted after %d attempts, last error: %v", retry, err)

	return nil, &ErrRetriesExhausted{
		Attempts: retry,
		LastErr:  err,
	}
}

// NewRetry creates a new Retry instance with the given configuration.
func NewRetry(
	rty Retry,
) *Retry {
	retry := Retry{
		Retries:            rty.Retries,
		Backoff:            rty.Backoff,
		BackoffMultiplier:  rty.BackoffMultiplier,
		DisableBackoffCaps: rty.DisableBackoffCaps,
	}

	// If the number of retries is less than 0, set it to 0 to disable retries.
	if retry.Retries < 0 {
		retry.Retries = 0
	}

	if !retry.DisableBackoffCaps && retry.BackoffMultiplier > BackoffMultiplierCap {
		retry.BackoffMultiplier = BackoffMultiplierCap
	}

	return &retry
}

