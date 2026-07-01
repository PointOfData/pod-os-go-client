package config

import (
	"time"

	"github.com/PointOfData/pod-os-go-client/connection"
	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
)

// Config holds configuration for a Pod-OS client
type Config struct {
	// Connection settings
	Network          string // "tcp", "udp", "unix"
	Host             string
	Port             string
	GatewayActorName string // Connection gateway FQN (socket identity). E.g. "zeroth.pod-os.com". Used for GatewayId/StreamOn and From addresses—not the To routing domain when peer-routing.

	// Client identification (for ID message)
	ClientName string // Name of the client connecting to the Actor (required for ID message)
	Passcode   string // Optional passcode for connection identification

	// Retry settings
	RetryConfig RetryConfig

	// Timeout settings
	DialTimeout    time.Duration
	ReceiveTimeout time.Duration
	SendTimeout    time.Duration

	// Connection pool settings
	PoolConfig PoolConfig

	// Streaming settings
	// EnableStreaming controls whether to send STREAM ON message.
	// nil (default) or true = enable streaming (STREAM ON), false = disable streaming.
	EnableStreaming *bool

	// Concurrent mode settings
	// EnableConcurrentMode enables background receiver for MessageId-based response correlation.
	// When enabled, multiple goroutines can send messages simultaneously without blocking each other.
	// Default: false (synchronous send-then-receive pattern)
	EnableConcurrentMode bool

	// ResponseTimeout is the timeout for waiting for a response in concurrent mode.
	// If not set, ReceiveTimeout is used. Only applies when EnableConcurrentMode is true.
	ResponseTimeout time.Duration

	// UnmatchedMessageHandler is invoked for inbound messages that do not match a pending
	// outbound request when EnableConcurrentMode is true. Wired before StartReceiver in NewClient.
	UnmatchedMessageHandler func(*message.Message)

	// Reconnection settings for automatic recovery when gateway restarts or connection is lost.
	// ReconnectConfig holds all reconnection-related configuration.
	ReconnectConfig ReconnectConfig

	// KeepaliveInterval controls how often the client sends an app-level AIP Keepalive
	// (message_type 18) on connections it owns. Zero or negative disables keepalive.
	// When unset (zero), GetKeepaliveInterval returns the default (30 seconds).
	KeepaliveInterval time.Duration

	// LogLevel: 0=disabled, 1=Error, 2=Warn, 3=Info, 4=Debug.
	// Production: 1-2. Development: 3-4.
	LogLevel int

	// Logger: injectable. If nil, uses NoOpLogger (zero overhead).
	Logger log.Logger

	// Optional: OpenTelemetry
	EnableTracing bool
	TracerName    string

	// Tracer: injectable. If nil, uses NoOpTracer (zero overhead).
	// Wire EnableTracing/TracerName when providing an OTLP-configured Tracer.
	Tracer connection.Tracer

	// WireHook: injectable wire-level observer. Called for every raw frame sent
	// or received on the underlying TCP connection, including the GatewayId and
	// GatewayStreamOn handshake frames that are sent inside NewClient before it
	// returns to the caller.
	// Nil = disabled (zero overhead).
	WireHook connection.WireHook
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	Retries            int
	Backoff            time.Duration
	BackoffMultiplier  float64
	DisableBackoffCaps bool
}

// PoolConfig holds connection pool configuration
type PoolConfig struct {
	InitialCapacity int
	MaxCapacity     int
}

// ReconnectConfig holds automatic reconnection configuration.
// When enabled, the client will automatically attempt to reconnect
// when the gateway restarts or the connection is lost.
type ReconnectConfig struct {
	// Enabled controls whether automatic reconnection is enabled.
	// Default: true (nil pointer or true = enabled)
	Enabled *bool

	// MaxRetries is the maximum number of reconnection attempts.
	// Default: 10. Set to 0 for unlimited retries.
	MaxRetries int

	// InitialBackoff is the initial backoff duration between reconnection attempts.
	// Default: 1 second
	InitialBackoff time.Duration

	// BackoffMultiplier is the multiplier applied to backoff after each failed attempt.
	// Default: 2.0
	BackoffMultiplier float64

	// MaxBackoff is the maximum backoff duration cap.
	// Default: 60 seconds
	MaxBackoff time.Duration
}

const defaultKeepaliveInterval = 30 * time.Second

// DefaultKeepaliveInterval is the default app-level AIP Keepalive period.
func DefaultKeepaliveInterval() time.Duration {
	return defaultKeepaliveInterval
}

// GetKeepaliveInterval returns the configured keepalive interval, or the default
// when unset. Returns zero when keepalive is explicitly disabled (negative value).
func (c *Config) GetKeepaliveInterval() time.Duration {
	if c.KeepaliveInterval < 0 {
		return 0
	}
	if c.KeepaliveInterval == 0 {
		return defaultKeepaliveInterval
	}
	return c.KeepaliveInterval
}

// DefaultReconnectConfig returns the default reconnection configuration.
func DefaultReconnectConfig() ReconnectConfig {
	enabled := true
	return ReconnectConfig{
		Enabled:           &enabled,
		MaxRetries:        10,
		InitialBackoff:    1 * time.Second,
		BackoffMultiplier: 2.0,
		MaxBackoff:        60 * time.Second,
	}
}

// IsEnabled returns whether reconnection is enabled.
// Returns true if Enabled is nil (default) or explicitly set to true.
func (r *ReconnectConfig) IsEnabled() bool {
	return r.Enabled == nil || *r.Enabled
}

// GetInitialBackoff returns the initial backoff or default of 1 second.
func (r *ReconnectConfig) GetInitialBackoff() time.Duration {
	if r.InitialBackoff <= 0 {
		return 1 * time.Second
	}
	return r.InitialBackoff
}

// GetBackoffMultiplier returns the backoff multiplier or default of 2.0.
func (r *ReconnectConfig) GetBackoffMultiplier() float64 {
	if r.BackoffMultiplier <= 0 {
		return 2.0
	}
	return r.BackoffMultiplier
}

// GetMaxBackoff returns the max backoff or default of 60 seconds.
func (r *ReconnectConfig) GetMaxBackoff() time.Duration {
	if r.MaxBackoff <= 0 {
		return 60 * time.Second
	}
	return r.MaxBackoff
}
