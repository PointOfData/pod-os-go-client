package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// ConfigFromEnv builds a Config from PODOS_* environment variables.
// This is intended for Category 1 (self-registering) containers that receive
// gateway connection details via environment rather than INI files.
//
// Recognized variables:
//
//	PODOS_GATEWAY_HOST, PODOS_GATEWAY_PORT, PODOS_GATEWAY_FQN,
//	PODOS_ACTOR_NAME, PODOS_PASSCODE,
//	PODOS_RECONNECT_ENABLED, PODOS_RECONNECT_MAX_RETRIES,
//	PODOS_RECONNECT_INITIAL_BACKOFF, PODOS_RECONNECT_MAX_BACKOFF,
//	PODOS_RECONNECT_BACKOFF_MULTIPLIER,
//	PODOS_CONCURRENT_MODE,
//	PODOS_DIAL_TIMEOUT, PODOS_SEND_TIMEOUT, PODOS_RECEIVE_TIMEOUT,
//	PODOS_LOG_LEVEL
//
// Unset variables are left at zero value. Numeric durations are in seconds.
func ConfigFromEnv() Config {
	cfg := Config{
		Network:          "tcp",
		Host:             os.Getenv("PODOS_GATEWAY_HOST"),
		Port:             os.Getenv("PODOS_GATEWAY_PORT"),
		GatewayActorName: os.Getenv("PODOS_GATEWAY_FQN"),
		ClientName:       os.Getenv("PODOS_ACTOR_NAME"),
		Passcode:         os.Getenv("PODOS_PASSCODE"),
	}

	if v := os.Getenv("PODOS_CONCURRENT_MODE"); v != "" {
		cfg.EnableConcurrentMode = parseBoolEnv(v)
	}

	if v := os.Getenv("PODOS_DIAL_TIMEOUT"); v != "" {
		if secs := parseIntEnv(v); secs > 0 {
			cfg.DialTimeout = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("PODOS_SEND_TIMEOUT"); v != "" {
		if secs := parseIntEnv(v); secs > 0 {
			cfg.SendTimeout = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("PODOS_RECEIVE_TIMEOUT"); v != "" {
		if secs := parseIntEnv(v); secs > 0 {
			cfg.ReceiveTimeout = time.Duration(secs) * time.Second
		}
	}

	if v := os.Getenv("PODOS_LOG_LEVEL"); v != "" {
		cfg.LogLevel = parseIntEnv(v)
	}

	// Reconnection settings
	if v := os.Getenv("PODOS_RECONNECT_ENABLED"); v != "" {
		b := parseBoolEnv(v)
		cfg.ReconnectConfig.Enabled = &b
	}
	if v := os.Getenv("PODOS_RECONNECT_MAX_RETRIES"); v != "" {
		cfg.ReconnectConfig.MaxRetries = parseIntEnv(v)
	}
	if v := os.Getenv("PODOS_RECONNECT_INITIAL_BACKOFF"); v != "" {
		if secs := parseIntEnv(v); secs > 0 {
			cfg.ReconnectConfig.InitialBackoff = time.Duration(secs) * time.Second
		}
	}
	if v := os.Getenv("PODOS_RECONNECT_BACKOFF_MULTIPLIER"); v != "" {
		if f := parseFloatEnv(v); f > 0 {
			cfg.ReconnectConfig.BackoffMultiplier = f
		}
	}
	if v := os.Getenv("PODOS_RECONNECT_MAX_BACKOFF"); v != "" {
		if secs := parseIntEnv(v); secs > 0 {
			cfg.ReconnectConfig.MaxBackoff = time.Duration(secs) * time.Second
		}
	}

	return cfg
}

func parseBoolEnv(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "true" || v == "1" || v == "yes" || v == "y"
}

func parseIntEnv(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func parseFloatEnv(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}
