package podos

import (
	"testing"

	"github.com/PointOfData/pod-os-go-client/config"
)

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{
			name:     "EOF error",
			errStr:   "EOF",
			expected: true,
		},
		{
			name:     "EOF error with prefix",
			errStr:   "couldn't receive data from the server, OriginalError: EOF",
			expected: true,
		},
		{
			name:     "connection reset error",
			errStr:   "read: connection reset by peer",
			expected: true,
		},
		{
			name:     "broken pipe error",
			errStr:   "write: broken pipe",
			expected: true,
		},
		{
			name:     "connection refused error",
			errStr:   "dial tcp 127.0.0.1:8080: connection refused",
			expected: true,
		},
		{
			name:     "closed network connection",
			errStr:   "use of closed network connection",
			expected: true,
		},
		{
			name:     "timeout error is not connection error",
			errStr:   "i/o timeout",
			expected: false,
		},
		{
			name:     "deadline exceeded is not connection error",
			errStr:   "context deadline exceeded",
			expected: false,
		},
		{
			name:     "other error",
			errStr:   "some random error",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConnectionError(tt.errStr)
			if result != tt.expected {
				t.Errorf("isConnectionError(%q) = %v, want %v", tt.errStr, result, tt.expected)
			}
		})
	}
}

func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{
			name:     "timeout error",
			errStr:   "timeout",
			expected: true,
		},
		{
			name:     "deadline exceeded",
			errStr:   "context deadline exceeded",
			expected: true,
		},
		{
			name:     "i/o timeout",
			errStr:   "i/o timeout",
			expected: true,
		},
		{
			name:     "EOF is not timeout",
			errStr:   "EOF",
			expected: false,
		},
		{
			name:     "connection reset is not timeout",
			errStr:   "connection reset by peer",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.errStr)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%q) = %v, want %v", tt.errStr, result, tt.expected)
			}
		})
	}
}

func TestReconnectConfigDefaults(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		cfg := config.DefaultReconnectConfig()

		if !cfg.IsEnabled() {
			t.Error("Expected reconnection to be enabled by default")
		}
		if cfg.MaxRetries != 10 {
			t.Errorf("Expected MaxRetries=10, got %d", cfg.MaxRetries)
		}
		if cfg.GetInitialBackoff().Seconds() != 1 {
			t.Errorf("Expected InitialBackoff=1s, got %v", cfg.GetInitialBackoff())
		}
		if cfg.GetBackoffMultiplier() != 2.0 {
			t.Errorf("Expected BackoffMultiplier=2.0, got %v", cfg.GetBackoffMultiplier())
		}
		if cfg.GetMaxBackoff().Seconds() != 60 {
			t.Errorf("Expected MaxBackoff=60s, got %v", cfg.GetMaxBackoff())
		}
	})

	t.Run("zero values use defaults", func(t *testing.T) {
		cfg := config.ReconnectConfig{}

		// nil Enabled means enabled
		if !cfg.IsEnabled() {
			t.Error("Expected IsEnabled() to return true for nil Enabled")
		}
		if cfg.GetInitialBackoff().Seconds() != 1 {
			t.Errorf("Expected GetInitialBackoff() to return 1s for zero value, got %v", cfg.GetInitialBackoff())
		}
		if cfg.GetBackoffMultiplier() != 2.0 {
			t.Errorf("Expected GetBackoffMultiplier() to return 2.0 for zero value, got %v", cfg.GetBackoffMultiplier())
		}
		if cfg.GetMaxBackoff().Seconds() != 60 {
			t.Errorf("Expected GetMaxBackoff() to return 60s for zero value, got %v", cfg.GetMaxBackoff())
		}
	})

	t.Run("disabled reconnection", func(t *testing.T) {
		disabled := false
		cfg := config.ReconnectConfig{
			Enabled: &disabled,
		}

		if cfg.IsEnabled() {
			t.Error("Expected IsEnabled() to return false when explicitly disabled")
		}
	})
}

func TestErrConnectionLost(t *testing.T) {
	if ErrConnectionLost == nil {
		t.Fatal("ErrConnectionLost should not be nil")
	}
	if ErrConnectionLost.Error() != "connection to gateway was lost during request" {
		t.Errorf("Unexpected error message: %s", ErrConnectionLost.Error())
	}
}
