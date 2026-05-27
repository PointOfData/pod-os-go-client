package config

import (
	"strconv"
	"strings"
	"time"
)

// ConfigFromINI populates a Config from INI key-value pairs as parsed by
// Pod-OS actor binaries (flat key=value format, no section headers).
//
// Recognized keys:
//
//	host, port, agent (GatewayActorName), client (ClientName),
//	stream_messages, concurrent_mode,
//	reconnect_enabled, reconnect_max_retries, reconnect_initial_backoff,
//	reconnect_backoff_multiplier, reconnect_max_backoff,
//	dial_timeout, send_timeout, receive_timeout,
//	retry_count, retry_backoff, retry_backoff_multiplier,
//	passcode, log_level
//
// Unrecognized keys are silently ignored. Numeric durations are in seconds.
func ConfigFromINI(kvs map[string]string) Config {
	cfg := Config{
		Network: "tcp",
	}

	for key, value := range kvs {
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "host":
			cfg.Host = value
		case "port":
			cfg.Port = value
		case "agent":
			cfg.GatewayActorName = value
		case "client":
			cfg.ClientName = value
		case "passcode":
			cfg.Passcode = value
		case "stream_messages":
			b := parseBoolINI(value)
			cfg.EnableStreaming = &b
		case "concurrent_mode":
			cfg.EnableConcurrentMode = parseBoolINI(value)
		case "dial_timeout":
			if secs := parseIntINI(value); secs > 0 {
				cfg.DialTimeout = time.Duration(secs) * time.Second
			}
		case "send_timeout":
			if secs := parseIntINI(value); secs > 0 {
				cfg.SendTimeout = time.Duration(secs) * time.Second
			}
		case "receive_timeout":
			if secs := parseIntINI(value); secs > 0 {
				cfg.ReceiveTimeout = time.Duration(secs) * time.Second
			}
		case "log_level":
			cfg.LogLevel = parseIntINI(value)

		// Reconnection settings
		case "reconnect_enabled":
			b := parseBoolINI(value)
			cfg.ReconnectConfig.Enabled = &b
		case "reconnect_max_retries":
			cfg.ReconnectConfig.MaxRetries = parseIntINI(value)
		case "reconnect_initial_backoff":
			if secs := parseIntINI(value); secs > 0 {
				cfg.ReconnectConfig.InitialBackoff = time.Duration(secs) * time.Second
			}
		case "reconnect_backoff_multiplier":
			if f := parseFloatINI(value); f > 0 {
				cfg.ReconnectConfig.BackoffMultiplier = f
			}
		case "reconnect_max_backoff":
			if secs := parseIntINI(value); secs > 0 {
				cfg.ReconnectConfig.MaxBackoff = time.Duration(secs) * time.Second
			}

		// Retry settings (initial dial)
		case "retry_count":
			cfg.RetryConfig.Retries = parseIntINI(value)
		case "retry_backoff":
			if secs := parseIntINI(value); secs > 0 {
				cfg.RetryConfig.Backoff = time.Duration(secs) * time.Second
			}
		case "retry_backoff_multiplier":
			if f := parseFloatINI(value); f > 0 {
				cfg.RetryConfig.BackoffMultiplier = f
			}
		}
	}

	return cfg
}

func parseBoolINI(v string) bool {
	v = strings.TrimSpace(strings.ToUpper(v))
	return v == "Y" || v == "YES" || v == "TRUE" || v == "1"
}

func parseIntINI(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

func parseFloatINI(v string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f
}
