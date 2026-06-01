package podos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/PointOfData/pod-os-go-client/config"
	gatewayerrors "github.com/PointOfData/pod-os-go-client/errors"
)

// Connection-loss is now classified by typed error codes rather than substring
// matching. These tests cover the typed classifiers and the public sentinel.
func TestConnectionLostClassification(t *testing.T) {
	connLost := gatewayerrors.ErrConnectionLost.Wrap(io.EOF)
	idle := gatewayerrors.ErrReceiveIdleTimeout.Wrap(context.DeadlineExceeded)

	if !gatewayerrors.IsConnectionLost(connLost) {
		t.Error("expected wrapped ErrConnectionLost to classify as connection lost")
	}
	if gatewayerrors.IsConnectionLost(idle) {
		t.Error("idle timeout must not classify as connection lost")
	}
	if !gatewayerrors.IsIdleTimeout(idle) {
		t.Error("expected wrapped ErrReceiveIdleTimeout to classify as idle timeout")
	}
	if gatewayerrors.IsIdleTimeout(connLost) {
		t.Error("connection lost must not classify as idle timeout")
	}

	// convertGatewayError must preserve the connection-lost classification so
	// callers can detect it with errors.Is(err, ErrConnectionLost).
	converted := convertGatewayError(connLost)
	if !errors.Is(converted, ErrConnectionLost) {
		t.Errorf("converted connection-lost error should match ErrConnectionLost, got %v", converted)
	}
	if !isFatalConnError(converted) {
		t.Error("isFatalConnError should report true for a converted connection-lost error")
	}

	// A plain application error must not be treated as a connection loss.
	other := fmt.Errorf("some random error")
	if isFatalConnError(other) {
		t.Error("isFatalConnError should report false for a non-connection error")
	}
	convertedIdle := convertGatewayError(idle)
	if errors.Is(convertedIdle, ErrConnectionLost) {
		t.Error("idle timeout must not convert to ErrConnectionLost")
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
