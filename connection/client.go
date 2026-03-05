package connection

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PointOfData/pod-os-go-client/errors"
	"github.com/PointOfData/pod-os-go-client/log"
)

// Tracer interface for optional OpenTelemetry support
type Tracer interface {
	Start(ctx context.Context, name string) (context.Context, Span)
}

// Span interface for tracing spans
type Span interface {
	End()
	RecordError(err error)
	AddEvent(name string)
}

// NoOpTracer is a no-op implementation of Tracer
type NoOpTracer struct{}

// NoOpSpan is a no-op implementation of Span
type NoOpSpan struct{}

func (n NoOpTracer) Start(ctx context.Context, name string) (context.Context, Span) {
	return ctx, NoOpSpan{}
}

func (n NoOpSpan) End()                  {}
func (n NoOpSpan) RecordError(err error) {}
func (n NoOpSpan) AddEvent(name string)  {}

// ClientConfig holds configuration for a Client
type ClientConfig struct {
	TracerName     string
	Tracer         Tracer        // Optional tracer, defaults to NoOpTracer
	Logger         log.Logger   // Optional logger, defaults to NoOpLogger
	DialTimeout    time.Duration // Timeout for establishing connection
	SendTimeout    time.Duration // Timeout for send operations (SendDeadline)
	ReceiveTimeout time.Duration // Timeout for receive operations
}

// IClient interface defines the client operations
type IClient interface {
	Send(data []byte) (int, *errors.GatewayDError)
	Receive(ctx context.Context) (int, []byte, *errors.GatewayDError)
	Reconnect() error
	Close()
	IsConnected() bool
	RemoteAddr() string
	LocalAddr() string
	Retry() *Retry
}

// Client represents a network client connection
type Client struct {
	Conn      net.Conn
	ctx       context.Context //nolint:containedctx
	connected atomic.Bool
	mu        sync.Mutex
	retry     IRetry

	GroupName string
	BlockName string

	TCPKeepAlive       bool
	TCPKeepAlivePeriod time.Duration
	ReceiveChunkSize   int
	ReceiveDeadline    time.Duration
	SendDeadline       time.Duration
	ReceiveTimeout     time.Duration
	DialTimeout        time.Duration
	Network            string // tcp/udp/unix

	Host      string
	Port      string
	Protocol  string
	Uuid      string
	ActorName string // this functions as the ID; aka what to search for for a specific client connection.

	tracer Tracer
	logger log.Logger
}

var _ IClient = (*Client)(nil)

// NewClient creates a new client.
func NewClient(ctx context.Context, cfg ClientConfig, network string, host string, port string, actorName string, retry *Retry) *Client {
	var tracer Tracer = NoOpTracer{}
	if cfg.Tracer != nil {
		tracer = cfg.Tracer
	}
	logger := log.LoggerOrNoOp(cfg.Logger)

	clientCtx, span := tracer.Start(ctx, "NewClient")
	defer span.End()

	var client Client

	// TODO: improve handling for nil and empty client configurations...
	if host == "" {
		return nil
	}

	client.connected.Store(false)
	client.tracer = tracer
	client.logger = logger

	// Try to resolve the address and log an error if it can't be resolved.
	addr, err := Resolve(network, net.JoinHostPort(host, port))
	if err != nil {
		logger.Error("failed to resolve address", "error", err)
		span.RecordError(err)
	}
	logger.Info("address resolved", "addr", addr)

	// Set default timeouts if not provided in config
	dialTimeout := cfg.DialTimeout
	if dialTimeout == 0 {
		dialTimeout = 5 * time.Second
	}

	// Create a resolved client.
	client = Client{
		ctx:         clientCtx,
		mu:          sync.Mutex{},
		retry:       retry,
		tracer:      tracer,  // Preserve tracer that was set earlier
		logger:      logger,  // Preserve logger — must not be dropped here
		Network:     network,
		Host:        host,
		Port:        port,
		DialTimeout: dialTimeout,
	}

	var clientErrors error
	// Create a new connection and retry a few times if needed.
	if conn, err := client.retry.Retry(func() (any, error) {
		if client.DialTimeout > 0 {
			return net.DialTimeout(client.Network, net.JoinHostPort(host, port), client.DialTimeout)
		} else {
			return net.Dial(client.Network, net.JoinHostPort(host, port))
		}
	}); err != nil {
		clientErrors = err
	} else {
		if netConn, ok := conn.(net.Conn); ok {
			client.Conn = netConn
		} else {
			clientErrors = fmt.Errorf("unexpected connection type in NewClient(): %T", conn)
		}
	}
	if clientErrors != nil || client.Conn == nil {
		err := errors.ErrClientConnectionFailed.Wrap(clientErrors)
		logger.Error("failed to create connection", "error", err)
		span.RecordError(err)
		return nil
	}

	client.connected.Store(true)

	// Set the TCP keep alive.
	client.TCPKeepAlive = false
	client.TCPKeepAlivePeriod = 30 * time.Second

	if c, ok := client.Conn.(*net.TCPConn); ok {
		if err := c.SetKeepAlive(client.TCPKeepAlive); err != nil {
			logger.Warn("failed to set keep alive", "error", err)
			span.RecordError(err)
		} else {
			if err := c.SetKeepAlivePeriod(client.TCPKeepAlivePeriod); err != nil {
				logger.Warn("failed to set keep alive period", "error", err)
				span.RecordError(err)
			}
		}
	}

	// Set timeouts to avoid indefinite blocking on IO
	// Use config values or defaults
	receiveTimeout := cfg.ReceiveTimeout
	if receiveTimeout == 0 {
		receiveTimeout = 5 * time.Second
	}
	client.ReceiveTimeout = receiveTimeout

	// Set the receive deadline (timeout) once; per-call timeouts are enforced in Receive via context
	client.ReceiveDeadline = 0
	if client.ReceiveDeadline > 0 {
		if err := client.Conn.SetReadDeadline(time.Now().Add(client.ReceiveDeadline)); err != nil {
			logger.Warn("failed to set receive deadline", "error", err)
			span.RecordError(err)
		} else if logger.Enabled(log.LevelDebug) {
			logger.Debug("set receive deadline")
		}
	}

	// Set the send deadline (timeout) from config
	sendTimeout := cfg.SendTimeout
	if sendTimeout == 0 {
		sendTimeout = 5 * time.Second
	}
	client.SendDeadline = sendTimeout
	if client.SendDeadline > 0 {
		if err := client.Conn.SetWriteDeadline(time.Now().Add(client.SendDeadline)); err != nil {
			logger.Warn("failed to set send deadline", "error", err)
			span.RecordError(err)
		} else if logger.Enabled(log.LevelDebug) {
			logger.Debug("set send deadline", "deadline", client.SendDeadline)
		}
	}

	// Set the receive chunk size. This is the size of the buffer that is read from the connection
	// in chunks. This is incremented as needed.
	client.ReceiveChunkSize = 512

	logger.Info("client created", "addr", addr)
	client.ActorName = actorName

	return &client
}

// Send sends data to the server.
func (c *Client) Send(data []byte) (int, *errors.GatewayDError) {
	_, span := c.tracer.Start(c.ctx, "Send")
	defer span.End()

	if !c.connected.Load() {
		span.RecordError(errors.ErrClientNotConnected)
		return 0, errors.ErrClientNotConnected
	}
	// Defensive: Conn can be nil despite connected flag (race, init bug, or Close)
	if c.Conn == nil {
		span.RecordError(errors.ErrClientNotConnected)
		return 0, errors.ErrClientNotConnected
	}

	// Refresh write deadline before each send operation
	// This ensures the deadline is fresh for each write, preventing timeouts on idle connections or longer writes
	if c.SendDeadline > 0 {
		if err := c.Conn.SetWriteDeadline(time.Now().Add(c.SendDeadline)); err != nil {
			c.logger.Warn("failed to set write deadline", "error", err)
			span.RecordError(err)
			// Continue anyway - the deadline might still be valid
		}
	}

	sent := 0
	dataSize := len(data)
	maxZeroWrites := 3
	zeroWriteCount := 0

	for {
		// If we've sent all the data, we must break the loop.
		if sent >= dataSize {
			break
		}

		// Write only the remaining bytes
		written, err := c.Conn.Write(data[sent:])
		if err != nil {
			// Log the error but don't panic - return error for caller to handle
			c.logger.Error("failed to send data", "error", err)
			span.RecordError(err)
			return sent, errors.ErrClientSendFailed.Wrap(err)
		}

		// Guard against infinite loop if Write returns 0 with no error
		if written == 0 {
			zeroWriteCount++
			if zeroWriteCount >= maxZeroWrites {
				err := fmt.Errorf("write returned 0 bytes %d times consecutively", maxZeroWrites)
				c.logger.Error("unexpected write behavior", "error", err)
				span.RecordError(err)
				return sent, errors.ErrClientSendFailed.Wrap(err)
			}
			continue
		}
		zeroWriteCount = 0 // Reset on successful write

		sent += written
	}
	if c.logger.Enabled(log.LevelDebug) {
		c.logger.Debug("sent data", "host", c.Host, "bytes", sent)
	}

	span.AddEvent("sent data to server")

	return sent, nil
}

// isValidLengthPrefix checks if the given 9-byte prefix is a valid message length prefix
// Valid formats:
//   - 'x' followed by 8 hex digits (0-9, a-f, A-F)
//   - 9 decimal digits (0-9)
func isValidLengthPrefix(prefix []byte) bool {
	if len(prefix) != 9 {
		return false
	}

	if prefix[0] == 'x' {
		// Hex format: check that bytes 1-8 are valid hex digits
		for i := 1; i < 9; i++ {
			b := prefix[i]
			if !((b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')) {
				return false
			}
		}
		return true
	}

	// Decimal format: check that all 9 bytes are decimal digits
	for i := 0; i < 9; i++ {
		b := prefix[i]
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

// Receive receives data from the server, respecting the provided context timeout.
// If the context has no deadline, the client's ReceiveTimeout configuration is used.
// If ReceiveTimeout is also 0, a default timeout of 60 seconds per read operation is used.
// For large responses, per-read timeouts are allowed up to 60 seconds, but never exceed
// the context deadline. If a read times out but the context hasn't expired, reading continues
// with a fresh deadline to handle slow network transfers on large responses.
// Additionally, an activity timeout ensures that if no data is received for a period
// based on the expected transfer rate, the read will timeout even if individual reads haven't.
func (c *Client) Receive(ctx context.Context) (int, []byte, *errors.GatewayDError) {
	// If context has no deadline and client has ReceiveTimeout configured, use it
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && c.ReceiveTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.ReceiveTimeout)
		defer cancel()
	}
	_, span := c.tracer.Start(c.ctx, "Receive")
	defer span.End()

	if !c.connected.Load() {
		span.RecordError(errors.ErrClientNotConnected)
		return 0, nil, errors.ErrClientNotConnected
	}

	// First, read the first 9 bytes to get the total message length
	lengthPrefix := make([]byte, 9)
	totalRead := 0
	for totalRead < 9 {
		// Check if context has expired before attempting to read
		if ctx.Err() != nil {
			span.RecordError(ctx.Err())
			return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("context expired before attempting to read length prefix: %w", ctx.Err()))
		}

		// Calculate read deadline from context timeout
		// For the initial length prefix, use the full context deadline if available
		// Don't cap it at 60 seconds - the server may take time to prepare the response
		deadline, hasDeadline := ctx.Deadline()
		var readDeadline time.Time
		if hasDeadline {
			// Use the full context deadline for the initial read
			// The server may take time to prepare the response before sending the length prefix
			timeUntilDeadline := time.Until(deadline)
			if timeUntilDeadline <= 0 {
				span.RecordError(ctx.Err())
				return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("receive timeout: %w", ctx.Err()))
			}
			// Use the full remaining time for the initial length prefix read
			readDeadline = deadline
		} else {
			// No context deadline, check ReceiveTimeout first, then ReceiveDeadline, then default
			var perReadTimeout time.Duration
			if c.ReceiveTimeout > 0 {
				// Use ReceiveTimeout for the initial read if no context deadline
				perReadTimeout = c.ReceiveTimeout
			} else if c.ReceiveDeadline > 0 {
				// Fall back to ReceiveDeadline if set
				perReadTimeout = c.ReceiveDeadline
			} else {
				// Default to 5 minutes for initial read (server may take time to respond)
				perReadTimeout = 5 * time.Minute
			}
			readDeadline = time.Now().Add(perReadTimeout)
		}

		if err := c.Conn.SetReadDeadline(readDeadline); err != nil {
			c.logger.Warn("failed to set read deadline", "error", err)
			// Continue anyway
		}

		read, err := c.Conn.Read(lengthPrefix[totalRead:])
		if err != nil {
			// Check if context expired
			if ctx.Err() != nil {
				span.RecordError(ctx.Err())
				return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("receive timeout: %w", ctx.Err()))
			}
			// Handle timeout errors - if context hasn't expired, continue reading
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if we still have time in the context
				if ctx.Err() == nil {
					// Context hasn't expired, continue reading with a fresh deadline
					if c.logger.Enabled(log.LevelDebug) {
						c.logger.Debug("read timeout on length prefix, context still valid")
					}
					continue
				}
			}
			c.logger.Error("failed to read length prefix", "host", c.Host, "error", err)
			span.RecordError(err)
			return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(err)
		}
		totalRead += read
	}
	if totalRead < 9 {
		span.RecordError(errors.ErrClientReceiveFailed)
		return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("could not read complete length prefix"))
	}

	// Validate that we're reading a valid length prefix before parsing
	// This helps detect when the connection is out of sync
	if !isValidLengthPrefix(lengthPrefix) {
		// Connection appears to be out of sync - we're reading from the wrong position
		prefixStr := string(lengthPrefix)
		c.logger.Error("connection out of sync", "prefix", prefixStr, "msg", "invalid length prefix - previous message may not have been fully consumed")
		span.RecordError(fmt.Errorf("connection out of sync: invalid length prefix"))
		return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("connection out of sync: expected message length prefix but received %q - this usually indicates the previous message wasn't fully consumed or the connection buffer has leftover data", prefixStr))
	}

	// Decode the total message length
	// Format: "x" followed by 8 hex digits, OR 9 decimal digits
	// Parse exactly 9 bytes without trimming
	var totalMessageLength int64
	var err error
	if lengthPrefix[0] == 'x' {
		// Hex format: 'x' (byte 0) + 8 hex digits (bytes 1-8)
		// Parse bytes 1-8 as hex
		hexDigits := string(lengthPrefix[1:9]) // Exactly 8 bytes
		totalMessageLength, err = strconv.ParseInt(hexDigits, 16, 32)
		if err != nil {
			span.RecordError(err)
			return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("failed to parse hex message length from %q: %w", hexDigits, err))
		}
	} else {
		// Decimal format: 9 decimal digits (bytes 0-8)
		// Parse all 9 bytes as decimal
		decimalDigits := string(lengthPrefix[0:9]) // Exactly 9 bytes
		totalMessageLength, err = strconv.ParseInt(decimalDigits, 10, 32)
		if err != nil {
			span.RecordError(err)
			return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("failed to parse decimal message length from %q: %w", decimalDigits, err))
		}
	}

	// Now read the remaining bytes (totalMessageLength includes the 9-byte length prefix)
	remainingBytes := int(totalMessageLength) - 9
	if remainingBytes < 0 {
		span.RecordError(errors.ErrClientReceiveFailed)
		return totalRead, nil, errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("invalid message length: %d", totalMessageLength))
	}

	buffer := bytes.NewBuffer(lengthPrefix) // Start with the length prefix we already read
	// totalRead already includes the 9 bytes we read for the length prefix

	// Activity timeout tracking: track last time we received data
	// If no data is received for a period based on expected transfer rate, timeout
	lastActivityTime := time.Now()
	var activityTimeout time.Duration

	// Calculate activity timeout based on expected transfer rate
	// Use a conservative estimate: if we haven't received data in time proportional
	// to the remaining bytes at a reasonable transfer rate, something is wrong
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		timeRemaining := time.Until(deadline)
		if timeRemaining > 0 {
			// Calculate expected transfer rate: remaining bytes / remaining time
			// Use a minimum transfer rate of 1KB/s to avoid overly aggressive timeouts
			expectedTransferRate := float64(remainingBytes) / timeRemaining.Seconds()
			minTransferRate := 1024.0 // 1KB/s minimum
			if expectedTransferRate < minTransferRate {
				expectedTransferRate = minTransferRate
			}
			// Activity timeout: time to transfer one chunk at expected rate, with a minimum of 30s
			// and maximum of 2 minutes, but never more than remaining context time
			chunkTransferTime := time.Duration(float64(c.ReceiveChunkSize) / expectedTransferRate * float64(time.Second))
			activityTimeout = chunkTransferTime * 3 // Allow 3x chunk transfer time for activity timeout
			if activityTimeout < 30*time.Second {
				activityTimeout = 30 * time.Second
			}
			if activityTimeout > 2*time.Minute {
				activityTimeout = 2 * time.Minute
			}
			// Never exceed remaining context time
			if activityTimeout > timeRemaining {
				activityTimeout = timeRemaining
			}
		} else {
			// No time remaining, use a short timeout
			activityTimeout = 10 * time.Second
		}
	} else {
		// No context deadline, use a default activity timeout
		activityTimeout = 2 * time.Minute
	}

	// Read the remaining bytes in chunks
	// Respect context timeout - if context expires, abort immediately
	for remainingBytes > 0 {
		// Check if context has expired before attempting to read
		if ctx.Err() != nil {
			span.RecordError(ctx.Err())
			return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("receive timeout: %w", ctx.Err()))
		}

		// Check activity timeout: if we haven't received data for too long, timeout
		timeSinceLastActivity := time.Since(lastActivityTime)
		if timeSinceLastActivity > activityTimeout {
			span.RecordError(fmt.Errorf("activity timeout: no data received for %v", timeSinceLastActivity))
			return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("activity timeout: no data received for %v (expected transfer rate not met, remaining bytes: %d)", timeSinceLastActivity, remainingBytes))
		}

		// Calculate read deadline from context timeout
		deadline, hasDeadline := ctx.Deadline()
		var readDeadline time.Time
		if hasDeadline {
			// Use context deadline, but ensure we have at least a small window
			timeUntilDeadline := time.Until(deadline)
			if timeUntilDeadline <= 0 {
				span.RecordError(ctx.Err())
				return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("receive timeout: %w", ctx.Err()))
			}
			// Use a per-read timeout that's smaller than the context deadline
			// For large responses, allow longer per-read timeouts (up to 60 seconds)
			// but never exceed the context deadline
			perReadTimeout := timeUntilDeadline
			// Increase cap for large responses - allow up to 60 seconds per read
			// This helps with very large responses that may have slow network transfer
			maxPerReadTimeout := 60 * time.Second
			if perReadTimeout > maxPerReadTimeout {
				perReadTimeout = maxPerReadTimeout
			}
			// Ensure read deadline never exceeds the context deadline
			calculatedDeadline := time.Now().Add(perReadTimeout)
			if calculatedDeadline.After(deadline) {
				readDeadline = deadline
			} else {
				readDeadline = calculatedDeadline
			}
		} else {
			// No context deadline, use ReceiveDeadline or default
			perReadTimeout := c.ReceiveDeadline
			if perReadTimeout == 0 {
				perReadTimeout = 60 * time.Second
			}
			readDeadline = time.Now().Add(perReadTimeout)
		}

		if err := c.Conn.SetReadDeadline(readDeadline); err != nil {
			c.logger.Warn("failed to set read deadline", "error", err)
			// Continue anyway
		}

		chunkSize := c.ReceiveChunkSize
		if chunkSize > remainingBytes {
			chunkSize = remainingBytes
		}

		chunk := make([]byte, chunkSize)
		read, err := c.Conn.Read(chunk)
		if read > 0 {
			totalRead += read
			buffer.Write(chunk[:read])
			remainingBytes -= read
			lastActivityTime = time.Now() // Update activity time when we receive data
		}
		if err != nil {
			// Check if context expired first
			if ctx.Err() != nil {
				span.RecordError(ctx.Err())
				return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("receive timeout: %w", ctx.Err()))
			}
			if err == io.EOF && remainingBytes == 0 {
				// Successfully read all bytes
				break
			}
			// Handle timeout errors - if context hasn't expired, continue reading
			// This allows for slow network transfers on large responses
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Check if we still have time in the context
				if ctx.Err() == nil {
					// Check activity timeout before continuing
					timeSinceLastActivity := time.Since(lastActivityTime)
					if timeSinceLastActivity > activityTimeout {
						span.RecordError(fmt.Errorf("activity timeout: no data received for %v", timeSinceLastActivity))
						return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("activity timeout: no data received for %v (expected transfer rate not met, remaining bytes: %d)", timeSinceLastActivity, remainingBytes))
					}
					// Context hasn't expired and activity timeout hasn't been exceeded, continue reading with a fresh deadline
					if c.logger.Enabled(log.LevelDebug) {
						c.logger.Debug("read timeout on chunk, continuing", "remaining_bytes", remainingBytes)
					}
					continue
				}
			}
			c.logger.Error("failed to receive data", "host", c.Host, "error", err)
			span.RecordError(err)
			return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(err)
		}
	}

	if remainingBytes > 0 {
		span.RecordError(errors.ErrClientReceiveFailed)
		return totalRead, buffer.Bytes(), errors.ErrClientReceiveFailed.Wrap(fmt.Errorf("incomplete message: expected %d bytes, got %d", int(totalMessageLength), totalRead))
	}

	span.AddEvent("Received data from server")

	return totalRead, buffer.Bytes(), nil
}

// Reconnect reconnects to the server.
func (c *Client) Reconnect() error {
	_, span := c.tracer.Start(c.ctx, "Reconnect")
	defer span.End()

	// Save the current address and network.
	host := c.Host
	port := c.Port
	actorName := c.ActorName

	network := c.Network

	if c.Conn != nil {
		c.Close()
	}
	c.connected.Store(false)

	// Restore the address and network.
	c.Host = host
	c.Port = port
	c.ActorName = actorName
	c.Network = network

	var _aiperrors error
	// Create a new connection and retry a few times if needed.
	if conn, err := c.retry.Retry(func() (any, error) {
		if c.DialTimeout > 0 {
			return net.DialTimeout(c.Network, net.JoinHostPort(c.Host, c.Port), c.DialTimeout)
		} else {
			return net.Dial(c.Network, net.JoinHostPort(c.Host, c.Port))
		}
	}); err != nil {
		_aiperrors = err
	} else {
		if netConn, ok := conn.(net.Conn); ok {
			c.Conn = netConn
		} else {
			_aiperrors = fmt.Errorf("unexpected connection type: %T", conn)
		}
	}
	if _aiperrors != nil {
		c.logger.Error("failed to reconnect", "addr", net.JoinHostPort(c.Host, c.Port), "error", _aiperrors)
		span.RecordError(_aiperrors)
		return errors.ErrClientConnectionFailed.Wrap(_aiperrors)
	}

	c.connected.Store(true)
	c.logger.Info("reconnected to server", "addr", net.JoinHostPort(c.Host, c.Port), "actor", c.ActorName)
	span.AddEvent("Reconnected to server")

	return nil
}

// Close closes the connection to the server.
func (c *Client) Close() {
	_, span := c.tracer.Start(c.ctx, "Close")
	defer span.End()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Set the deadline to now so that the connection is closed immediately.
	// This will stop all the Conn.Read() and Conn.Write() calls.
	// Ref: https://groups.google.com/g/golang-nuts/c/VPVWFrpIEyo
	if c.Conn != nil {
		if err := c.Conn.SetDeadline(time.Now()); err != nil {
			c.logger.Warn("failed to set deadline", "error", err)
			span.RecordError(err)
		}
	}

	c.connected.Store(false)
	c.logger.Info("closing connection", "addr", net.JoinHostPort(c.Host, c.Port), "actor", c.ActorName)
	if c.Conn != nil {
		if err := c.Conn.Close(); err != nil {
			c.logger.Error("failed to close connection", "addr", net.JoinHostPort(c.Host, c.Port), "actor", c.ActorName, "error", err)
			span.RecordError(err)
		}
	}
	c.Uuid = ""
	c.Conn = nil
	c.Host = ""
	c.Port = ""
	c.ActorName = ""
	c.Network = ""

	span.AddEvent("Closed connection to server")
}

// IsConnected checks if the client is still connected to the server.
func (c *Client) IsConnected() bool {
	if c == nil {
		return false
	}

	if c.ctx.Err() != nil {
		_, span := c.tracer.Start(c.ctx, "IsConnected")
		defer span.End()
	}

	if c.Conn == nil {
		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("connection closed", "addr", net.JoinHostPort(c.Host, c.Port), "actor", c.ActorName)
		}
		return false
	}

	return c.connected.Load()
}

// RemoteAddr returns the remote address of the client safely.
func (c *Client) RemoteAddr() string {
	if !c.connected.Load() {
		return ""
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Conn != nil && c.Conn.RemoteAddr() != nil {
		return c.Conn.RemoteAddr().String()
	}

	return ""
}

// LocalAddr returns the local address of the client safely.
func (c *Client) LocalAddr() string {
	if !c.connected.Load() {
		return ""
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Conn != nil && c.Conn.LocalAddr() != nil {
		return c.Conn.LocalAddr().String()
	}

	return ""
}

// Retry returns the retry object.
//
//nolint:revive
func (c *Client) Retry() *Retry {
	if retry, ok := c.retry.(*Retry); !ok {
		return nil
	} else {
		return retry
	}
}
