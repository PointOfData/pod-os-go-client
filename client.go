package podos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/PointOfData/pod-os-go-client/config"
	"github.com/PointOfData/pod-os-go-client/connection"
	gatewayerrors "github.com/PointOfData/pod-os-go-client/errors"
	"github.com/PointOfData/pod-os-go-client/log"
	"github.com/PointOfData/pod-os-go-client/message"
	"github.com/google/uuid"
)

// clientRegistry stores active clients by ClientName + ActorName (primary key)
// actorRegistry provides lookup by ActorName only (secondary index)
var (
	clientRegistry = make(map[string]*Client)
	actorRegistry  = make(map[string]*Client) // ActorName -> *Client
	registryMu     sync.RWMutex
)

// ErrConnectionLost is returned when a request fails because the connection
// to the gateway was lost. Callers can check for this error and retry their request.
var ErrConnectionLost = errors.New("connection to gateway was lost during request")

// getClientKey creates a unique key from ClientName and GatewayActorName
func getClientKey(clientName, gatewayActorName string) string {
	return clientName + ":" + gatewayActorName
}

// GetClientByGatewayActorName returns a client for the given gatewayActorName (gateway.domain_name).
// Returns nil if no client exists for that gatewayActorName.
func GetClientByGatewayActorName(gatewayActorName string) *Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return actorRegistry[gatewayActorName]
}

// RegisterClient stores a client in the registry by GatewayActorName.
// If a client already exists for this GatewayActorName, it will be replaced
// (the old client should be closed first by the caller).
func RegisterClient(client *Client) error {
	if client == nil {
		return fmt.Errorf("cannot register nil client")
	}
	if client.gatewayActorName == "" {
		return fmt.Errorf("client gatewayActorName cannot be empty")
	}
	if client.clientName == "" {
		return fmt.Errorf("client ClientName cannot be empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	// Store in both registries
	key := getClientKey(client.clientName, client.gatewayActorName) // client.gatewayActorName is the same as client.ActorName()
	client.key = key
	clientRegistry[key] = client
	actorRegistry[client.gatewayActorName] = client

	log.LoggerOrNoOp(client.logger).Info("registered client", "actor", client.gatewayActorName, "client_name", client.clientName)
	return nil
}

// RemoveClientByGatewayActorName removes a client from the registry by GatewayActorName.
// Does not close the client - caller should close it if needed.
func RemoveClientByGatewayActorName(gatewayActorName string) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if client, exists := actorRegistry[gatewayActorName]; exists {
		delete(actorRegistry, gatewayActorName)
		if client.key != "" {
			delete(clientRegistry, client.key)
		}
		log.LoggerOrNoOp(client.logger).Info("removed client from registry", "actor", gatewayActorName)
	}
}

// GetClientCount returns the number of registered clients.
func GetClientCount() int {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return len(actorRegistry)
}

// convertGatewayError converts a GatewayDError to a standard error
// This prevents internal GatewayDError types from leaking to client code
func convertGatewayError(gatewayErr *gatewayerrors.GatewayDError) error {
	if gatewayErr == nil {
		return nil
	}
	if gatewayErr.OriginalError != nil {
		return fmt.Errorf("%s: %w", gatewayErr.Message, gatewayErr.OriginalError)
	}
	return fmt.Errorf("%s", gatewayErr.Message)
}

// Client represents a Pod-OS client connection with support for concurrent operations.
// When concurrent mode is enabled, a background receiver handles response routing
// using MessageId correlation, allowing multiple goroutines to send messages
// simultaneously without blocking each other.
type Client struct {
	conn             *connection.Client
	pool             *connection.ChannelPool
	cfg              config.Config
	gatewayActorName string // Name of the gateway Actor. E.g. "zeroth.pod-os.com"
	clientName       string
	key              string // registry key for this client

	// Concurrent message handling
	sendMu         sync.Mutex                       // Protects send operations
	receiverCtx    context.Context                  // Context for background receiver
	receiverCancel context.CancelFunc               // Cancel function for receiver
	receiverWg     sync.WaitGroup                   // Wait group for receiver goroutine
	receiverActive bool                             // Whether background receiver is running
	pendingMu      sync.RWMutex                     // Protects pending map
	pending        map[string]chan *message.Message // messageId -> response channel

	// Reconnection state
	reconnecting     bool       // Whether a reconnection is in progress
	reconnectingMu   sync.Mutex // Protects reconnecting flag
	reconnectAttempt int        // Current reconnection attempt number

	logger log.Logger
}

// NewClient creates a new Pod-OS client or returns an existing one if ClientName + ActorName already exists.
// ClientName is required and must be provided in the config.
func NewClient(ctx context.Context, cfg config.Config) (*Client, error) {
	// Validate required ClientName
	if cfg.ClientName == "" {
		return nil, fmt.Errorf("ClientName is required and cannot be empty")
	}
	if cfg.GatewayActorName == "" {
		return nil, fmt.Errorf("GatewayActorName is required and cannot be empty")
	}

	logger := log.LoggerFromConfig(cfg.Logger, log.Level(cfg.LogLevel))
	message.SetDecoderLogger(logger)

	// Create unique key for this client
	key := getClientKey(cfg.ClientName, cfg.GatewayActorName)

	// Check if client already exists (with write lock to prevent race conditions)
	registryMu.Lock()
	if existingClient, exists := clientRegistry[key]; exists {
		registryMu.Unlock()
		// Verify existing client is still connected
		if existingClient.IsConnected() {
			logger.Info("returning existing client", "key", key)
			return existingClient, nil
		}
		// Client exists but is not connected, remove it and create a new one
		logger.Info("existing client not connected, creating new one", "key", key)
		registryMu.Lock()
		delete(clientRegistry, key)
		registryMu.Unlock()
	} else {
		registryMu.Unlock()
	}

	// Create retry configuration
	retry := connection.NewRetry(connection.Retry{
		Retries:            cfg.RetryConfig.Retries,
		Backoff:            cfg.RetryConfig.Backoff,
		BackoffMultiplier:  cfg.RetryConfig.BackoffMultiplier,
		DisableBackoffCaps: cfg.RetryConfig.DisableBackoffCaps,
		Logger:             logger,
	})

	// Create connection client
	clientConfig := connection.ClientConfig{
		TracerName:     cfg.TracerName,
		Tracer:         cfg.Tracer,
		Logger:         logger,
		DialTimeout:    cfg.DialTimeout,
		SendTimeout:    cfg.SendTimeout,
		ReceiveTimeout: cfg.ReceiveTimeout,
	}

	conn := connection.NewClient(ctx, clientConfig, cfg.Network, cfg.Host, cfg.Port, cfg.GatewayActorName, retry)
	if conn == nil {
		return nil, fmt.Errorf("failed to create connection client")
	}

	// Validate connection
	if !conn.IsConnected() {
		conn.Close()
		return nil, fmt.Errorf("connection client is not connected")
	}

	clientName := cfg.ClientName

	// Automatically send ID message to identify the connection
	// This is required before any other messages will be recognized by Pod-OS
	conversationUUID := uuid.New().String()
	idMsg := &message.Message{
		Envelope: message.Envelope{
			To:         "$system@" + cfg.GatewayActorName,
			From:       clientName + "@" + cfg.GatewayActorName,
			Intent:     message.IntentType.GatewayId,
			ClientName: clientName,
			Passcode:   cfg.Passcode,
			MessageId:  uuid.New().String(),
		},
		Event: &message.EventFields{
			Owner:             "$sys",
			Timestamp:         message.GetTimestamp(),
			LocationSeparator: "|",
		},
	}

	// Encode and send ID message
	socketMsg, err := message.EncodeMessage(idMsg, conversationUUID)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to encode ID message: %w", err)
	}

	// Check connection before sending
	if !conn.IsConnected() {
		conn.Close()
		return nil, fmt.Errorf("connection closed before sending ID message")
	}

	sent, sendErr := conn.Send(socketMsg.MessageBytes)
	if sendErr != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send ID message: %w", convertGatewayError(sendErr))
	}

	// Verify we actually sent data
	if sent == 0 {
		conn.Close()
		return nil, fmt.Errorf("failed to send ID message: no bytes sent")
	}

	logger.Info("ID message sent", "bytes", sent, "actor", cfg.GatewayActorName, "client_name", clientName)

	// Receive and check ID response for any errors
	idReceiveTimeout := cfg.ReceiveTimeout
	if idReceiveTimeout == 0 {
		idReceiveTimeout = 10 * time.Second // Default timeout for ID response
	}
	idReceiveCtx, idReceiveCancel := context.WithTimeout(ctx, idReceiveTimeout)
	defer idReceiveCancel()

	_, idResponseBytes, idReceiveErr := conn.Receive(idReceiveCtx)
	if idReceiveErr != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to receive ID response: %w", convertGatewayError(idReceiveErr))
	}

	if len(idResponseBytes) == 0 {
		conn.Close()
		return nil, fmt.Errorf("received empty ID response from %s", cfg.GatewayActorName)
	}

	idResponse, decodeErr := message.DecodeMessage(idResponseBytes)
	if decodeErr != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to decode ID response: %w", decodeErr)
	}

	// Check if the ID response indicates an error
	if idResponse.ProcessingStatus() == "ERROR" {
		errMsg := idResponse.ProcessingMessage()
		if errMsg == "" {
			errMsg = "unknown error from Gateway"
		}
		conn.Close()
		return nil, fmt.Errorf("ID message rejected by Gateway: %s", errMsg)
	}

	logger.Info("ID response received", "actor", cfg.GatewayActorName, "status", idResponse.ProcessingStatus())

	// Send STREAM ON message to enable streaming mode (default behavior)
	// Only skip if explicitly disabled via EnableStreaming = false
	// nil (default) or true = enable streaming, false = disable streaming
	enableStreaming := cfg.EnableStreaming == nil || *cfg.EnableStreaming
	if enableStreaming {
		streamOnMsg := &message.Message{
			Envelope: message.Envelope{
				To:         "$system@" + cfg.GatewayActorName,
				From:       clientName + "@" + cfg.GatewayActorName,
				Intent:     message.IntentType.GatewayStreamOn,
				ClientName: clientName,
				Passcode:   cfg.Passcode,
				MessageId:  uuid.New().String(),
			},
		}
		streamOnSocketMsg, err := message.EncodeMessage(streamOnMsg, uuid.New().String())
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to encode STREAM ON message: %w", err)
		}

		// Check connection before sending
		if !conn.IsConnected() {
			conn.Close()
			return nil, fmt.Errorf("connection closed before sending STREAM ON message")
		}

		streamOnSent, sendErr := conn.Send(streamOnSocketMsg.MessageBytes)
		if sendErr != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to send STREAM ON message: %w", convertGatewayError(sendErr))
		}

		// Verify we actually sent data
		if streamOnSent == 0 {
			conn.Close()
			return nil, fmt.Errorf("failed to send STREAM ON message: no bytes sent")
		}

		logger.Info("STREAM ON message sent", "bytes", streamOnSent, "actor", cfg.GatewayActorName)
	} else {
		logger.Info("streaming disabled, skipping STREAM ON", "actor", cfg.GatewayActorName)
	}

	// Create connection pool if configured
	var pool *connection.ChannelPool
	if cfg.PoolConfig.MaxCapacity > 0 {
		factory := func() (net.Conn, string, error) {
			// Create a new connection for the pool
			netConn, err := net.DialTimeout(cfg.Network, fmt.Sprintf("%s:%s", cfg.Host, cfg.Port), cfg.DialTimeout)
			if err != nil {
				return nil, "", err
			}
			return netConn, uuid.New().String(), nil
		}
		pool = connection.NewChannelPool(cfg.PoolConfig.MaxCapacity, factory)
		if err := connection.InitializeChannelPool(pool, cfg.PoolConfig.InitialCapacity); err != nil {
			return nil, fmt.Errorf("failed to initialize connection pool: %w", err)
		}
	}

	// Create receiver context
	receiverCtx, receiverCancel := context.WithCancel(context.Background())

	client := &Client{
		conn:             conn,
		pool:             pool,
		cfg:              cfg,
		gatewayActorName: cfg.GatewayActorName,
		clientName:       clientName,
		key:              key,
		receiverCtx:      receiverCtx,
		receiverCancel:   receiverCancel,
		pending:          make(map[string]chan *message.Message),
		logger:           logger,
	}

	// Register the new client (double-check to prevent race conditions)
	registryMu.Lock()
	// Check again in case another goroutine created the client while we were creating ours
	if existingClient, exists := clientRegistry[key]; exists {
		registryMu.Unlock()
		// Another goroutine created the client, close our connection and return the existing one
		if client.pool != nil {
			client.pool.Close()
		}
		if client.conn != nil {
			client.conn.Close()
		}
		client.receiverCancel() // Cancel the receiver context we created
		if existingClient.IsConnected() {
			logger.Info("another goroutine created client, returning existing one", "key", key)
			return existingClient, nil
		}
		// Existing client is not connected, remove it and register ours
		registryMu.Lock()
		delete(clientRegistry, key)
		delete(actorRegistry, cfg.GatewayActorName)
		clientRegistry[key] = client
		actorRegistry[cfg.GatewayActorName] = client
		registryMu.Unlock()
		logger.Info("registered new client", "key", key, "msg", "replaced disconnected one")
		// Start background receiver if concurrent mode is enabled
		if cfg.EnableConcurrentMode {
			client.StartReceiver()
		}
		return client, nil
	}
	clientRegistry[key] = client
	actorRegistry[cfg.GatewayActorName] = client
	registryMu.Unlock()
	logger.Info("registered new client", "key", key)

	// Start background receiver if concurrent mode is enabled
	if cfg.EnableConcurrentMode {
		client.StartReceiver()
	}

	return client, nil
}

// SendMessage sends a message to a Pod-OS actor and returns the response.
// This method automatically sets the message's ClientName and From address
// to match this client's identity, ensuring consistency even if the message
// was created with a different (stale) ClientName.
//
// When concurrent mode is enabled (background receiver is active), this method
// uses MessageId correlation to route responses to the correct caller, allowing
// multiple goroutines to send messages simultaneously.
func (c *Client) SendMessage(ctx context.Context, msg *message.Message) (*message.Message, error) {
	// Auto-update ClientName to match this client's identity
	if msg.ClientName != c.clientName {
		c.logger.Info("updating message ClientName", "from", msg.ClientName, "to", c.clientName)
		msg.ClientName = c.clientName
	}

	// Auto-update From address to use this client's ClientName
	if msg.From != "" && strings.Contains(msg.From, "@") {
		parts := strings.Split(msg.From, "@")
		expectedFrom := c.clientName + "@" + parts[1]
		if msg.From != expectedFrom {
			c.logger.Info("updating message From", "from", msg.From, "to", expectedFrom)
			msg.From = expectedFrom
		}
	}

	// Ensure MessageId exists for potential correlation
	if msg.MessageId == "" {
		msg.MessageId = uuid.New().String()
	}

	// Use concurrent pattern if receiver is active, otherwise use synchronous pattern
	if c.receiverActive {
		return c.sendMessageWithCorrelation(ctx, msg)
	}
	return c.sendMessageSync(ctx, msg)
}

// SendControlMessage sends a control message (no response expected)
func (c *Client) SendControlMessage(ctx context.Context, msg *message.SocketMessage) error {
	// Check connection before sending
	if !c.conn.IsConnected() {
		return fmt.Errorf("connection closed before sending control message")
	}

	sent, sendErr := c.conn.Send(msg.MessageBytes)
	if sendErr != nil {
		return fmt.Errorf("failed to send control message: %w", convertGatewayError(sendErr))
	}

	// Verify we actually sent data
	if sent == 0 {
		return fmt.Errorf("failed to send control message: no bytes sent")
	}

	c.logger.Info("sent control message", "bytes", sent, "actor", c.gatewayActorName)
	return nil
}

// StartReceiver starts the background receiver goroutine for concurrent message handling.
// When active, responses are routed to waiting callers using MessageId correlation.
// This is automatically called when EnableConcurrentMode is true in config.
func (c *Client) StartReceiver() {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.receiverActive {
		c.logger.Info("background receiver already active", "actor", c.gatewayActorName)
		return
	}

	c.receiverActive = true
	c.receiverWg.Add(1)
	go func() {
		defer c.receiverWg.Done()
		c.receiveLoop()
	}()

	c.logger.Info("started background receiver", "actor", c.gatewayActorName)
}

// StopReceiver stops the background receiver goroutine.
// Any pending requests will receive an error.
func (c *Client) StopReceiver() {
	c.sendMu.Lock()
	if !c.receiverActive {
		c.sendMu.Unlock()
		return
	}
	c.sendMu.Unlock()

	// Signal receiver to stop
	c.receiverCancel()

	// Wait for receiver to stop (with timeout)
	done := make(chan struct{})
	go func() {
		c.receiverWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Receiver stopped cleanly
	case <-time.After(5 * time.Second):
		c.logger.Warn("timeout waiting for receiver to stop", "actor", c.gatewayActorName)
	}

	c.sendMu.Lock()
	c.receiverActive = false
	c.sendMu.Unlock()

	// Cancel any pending requests
	c.pendingMu.Lock()
	for msgId, ch := range c.pending {
		close(ch)
		delete(c.pending, msgId)
	}
	c.pendingMu.Unlock()

	c.logger.Info("stopped background receiver", "actor", c.gatewayActorName)
}

// IsReceiverActive returns whether the background receiver is running
func (c *Client) IsReceiverActive() bool {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return c.receiverActive
}

// receiveLoop continuously receives messages and routes them to waiting callers.
// If a connection error occurs (e.g., gateway restart), it will attempt to reconnect
// using exponential backoff if reconnection is enabled in the configuration.
func (c *Client) receiveLoop() {
	for {
		select {
		case <-c.receiverCtx.Done():
			c.logger.Info("receiver context cancelled, exiting loop", "actor", c.gatewayActorName)
			return
		default:
			// Continue receiving
		}

		// Check if still connected
		if !c.IsConnected() {
			// Attempt reconnection if enabled
			if c.cfg.ReconnectConfig.IsEnabled() {
				if c.attemptReconnection() {
					// Reconnection successful, continue receiving
					continue
				}
			}
			c.logger.Info("connection lost, receiver exiting", "actor", c.gatewayActorName)
			return
		}

		// Receive with a timeout to allow checking for shutdown
		// Use 30 second timeout for responsiveness to shutdown signals
		receiveCtx, cancel := context.WithTimeout(c.receiverCtx, 30*time.Second)
		_, responseBytes, receiveErr := c.conn.Receive(receiveCtx)
		cancel()

		if receiveErr != nil {
			// Check if receiver is shutting down
			if c.receiverCtx.Err() != nil {
				return
			}
			// Check if it's a timeout error (expected when idle)
			errStr := receiveErr.Error()
			if isTimeoutError(errStr) {
				// Timeout is normal when idle, continue loop
				continue
			}

			// Check if it's a connection error that requires reconnection
			if isConnectionError(errStr) {
				c.logger.Error("connection error", "actor", c.gatewayActorName, "error", receiveErr)

				// Notify pending callers that connection was lost
				c.notifyPendingCallersConnectionLost()

				// Attempt reconnection if enabled
				if c.cfg.ReconnectConfig.IsEnabled() {
					if c.attemptReconnection() {
						// Reconnection successful, continue receiving
						continue
					}
				}
				// Reconnection failed or disabled, exit loop
				c.logger.Error("connection recovery failed, receiver exiting", "actor", c.gatewayActorName)
				return
			}

			// Log other errors but continue
			c.logger.Error("receive error", "actor", c.gatewayActorName, "error", receiveErr)
			continue
		}

		if len(responseBytes) == 0 {
			continue
		}

		// Decode the response
		response, decodeErr := message.DecodeMessage(responseBytes)
		if decodeErr != nil {
			c.logger.Error("failed to decode response", "actor", c.gatewayActorName, "error", decodeErr)
			continue
		}

		// Route response to the waiting caller using MessageId
		messageId := response.MessageId
		if messageId == "" {
			c.logger.Warn("received response without MessageId", "actor", c.gatewayActorName)
			continue
		}

		c.pendingMu.RLock()
		responseChan, exists := c.pending[messageId]
		c.pendingMu.RUnlock()

		if exists {
			// Non-blocking send to avoid deadlock if caller already timed out
			select {
			case responseChan <- response:
				if c.logger.Enabled(log.LevelDebug) {
					c.logger.Debug("routed response to caller", "message_id", messageId)
				}
			default:
				if c.logger.Enabled(log.LevelDebug) {
					c.logger.Debug("caller already gone", "message_id", messageId, "msg", "timed out")
				}
			}
		} else {
			if c.logger.Enabled(log.LevelDebug) {
				c.logger.Debug("no pending request for MessageId", "message_id", messageId)
			}
		}
	}
}

// notifyPendingCallersConnectionLost notifies all pending callers that the connection was lost.
// Callers will receive a nil message, indicating they should retry their request.
func (c *Client) notifyPendingCallersConnectionLost() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for msgId, ch := range c.pending {
		// Close the channel to signal connection loss
		close(ch)
		delete(c.pending, msgId)
		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("notified pending caller of connection loss", "message_id", msgId)
		}
	}
}

// attemptReconnection attempts to reconnect to the gateway with exponential backoff.
// Returns true if reconnection was successful, false otherwise.
func (c *Client) attemptReconnection() bool {
	c.reconnectingMu.Lock()
	if c.reconnecting {
		c.reconnectingMu.Unlock()
		// Another goroutine is already reconnecting, wait and check result
		time.Sleep(100 * time.Millisecond)
		return c.IsConnected()
	}
	c.reconnecting = true
	c.reconnectAttempt = 0
	c.reconnectingMu.Unlock()

	defer func() {
		c.reconnectingMu.Lock()
		c.reconnecting = false
		c.reconnectingMu.Unlock()
	}()

	reconnectCfg := &c.cfg.ReconnectConfig
	maxRetries := reconnectCfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 10 // Default
	}

	backoff := reconnectCfg.GetInitialBackoff()
	multiplier := reconnectCfg.GetBackoffMultiplier()
	maxBackoff := reconnectCfg.GetMaxBackoff()

	for attempt := 1; attempt <= maxRetries || maxRetries == 0; attempt++ {
		// Check if receiver context is cancelled
		select {
		case <-c.receiverCtx.Done():
			c.logger.Info("reconnection cancelled", "actor", c.gatewayActorName, "msg", "context done")
			return false
		default:
		}

		c.reconnectingMu.Lock()
		c.reconnectAttempt = attempt
		c.reconnectingMu.Unlock()

		c.logger.Info("reconnection attempt", "attempt", attempt, "max_retries", maxRetries, "actor", c.gatewayActorName, "backoff", backoff)

		// Attempt to reconnect the underlying connection
		if err := c.conn.Reconnect(); err != nil {
			c.logger.Warn("reconnection attempt failed", "attempt", attempt, "actor", c.gatewayActorName, "error", err)

			// Wait with exponential backoff before next attempt
			select {
			case <-c.receiverCtx.Done():
				c.logger.Info("reconnection cancelled during backoff", "actor", c.gatewayActorName)
				return false
			case <-time.After(backoff):
			}

			// Increase backoff for next attempt
			backoff = time.Duration(float64(backoff) * multiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Connection re-established, now re-authenticate
		c.logger.Info("connection re-established, re-authenticating", "actor", c.gatewayActorName)

		if err := c.reAuthenticate(); err != nil {
			c.logger.Error("re-authentication failed", "actor", c.gatewayActorName, "error", err)
			// Close the connection and try again
			c.conn.Close()

			// Wait before next attempt
			select {
			case <-c.receiverCtx.Done():
				return false
			case <-time.After(backoff):
			}

			backoff = time.Duration(float64(backoff) * multiplier)
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		c.logger.Info("successfully reconnected and re-authenticated", "actor", c.gatewayActorName)
		return true
	}

	c.logger.Warn("max reconnection attempts reached", "actor", c.gatewayActorName)
	return false
}

// reAuthenticate re-sends the ID message and STREAM ON message after reconnection.
// This is required to re-establish the client's identity with the gateway.
func (c *Client) reAuthenticate() error {
	// Create and send ID message
	conversationUUID := uuid.New().String()
	idMsg := &message.Message{
		Envelope: message.Envelope{
			To:         "$system@" + c.gatewayActorName,
			From:       c.clientName + "@" + c.gatewayActorName,
			Intent:     message.IntentType.GatewayId,
			ClientName: c.clientName,
			Passcode:   c.cfg.Passcode,
			MessageId:  uuid.New().String(),
		},
		Event: &message.EventFields{
			Owner:             "$sys",
			Timestamp:         message.GetTimestamp(),
			LocationSeparator: "|",
		},
	}

	// Encode and send ID message
	socketMsg, err := message.EncodeMessage(idMsg, conversationUUID)
	if err != nil {
		return fmt.Errorf("failed to encode ID message: %w", err)
	}

	sent, sendErr := c.conn.Send(socketMsg.MessageBytes)
	if sendErr != nil {
		return fmt.Errorf("failed to send ID message: %w", convertGatewayError(sendErr))
	}

	if sent == 0 {
		return fmt.Errorf("failed to send ID message: no bytes sent")
	}

	c.logger.Info("re-authentication: ID message sent", "bytes", sent, "actor", c.gatewayActorName)

	// Receive ID response
	idReceiveTimeout := c.cfg.ReceiveTimeout
	if idReceiveTimeout == 0 {
		idReceiveTimeout = 10 * time.Second
	}
	idReceiveCtx, idReceiveCancel := context.WithTimeout(c.receiverCtx, idReceiveTimeout)
	defer idReceiveCancel()

	_, idResponseBytes, idReceiveErr := c.conn.Receive(idReceiveCtx)
	if idReceiveErr != nil {
		return fmt.Errorf("failed to receive ID response: %w", convertGatewayError(idReceiveErr))
	}

	if len(idResponseBytes) == 0 {
		return fmt.Errorf("received empty ID response from %s", c.gatewayActorName)
	}

	idResponse, decodeErr := message.DecodeMessage(idResponseBytes)
	if decodeErr != nil {
		return fmt.Errorf("failed to decode ID response: %w", decodeErr)
	}

	// Check if the ID response indicates an error
	if idResponse.ProcessingStatus() == "ERROR" {
		errMsg := idResponse.ProcessingMessage()
		if errMsg == "" {
			errMsg = "unknown error from Gateway"
		}
		return fmt.Errorf("ID message rejected by Gateway: %s", errMsg)
	}

	c.logger.Info("re-authentication: ID response received", "actor", c.gatewayActorName, "status", idResponse.ProcessingStatus())

	// Send STREAM ON message if streaming was enabled
	enableStreaming := c.cfg.EnableStreaming == nil || *c.cfg.EnableStreaming
	if enableStreaming {
		streamOnMsg := &message.Message{
			Envelope: message.Envelope{
				To:         "$system@" + c.gatewayActorName,
				From:       c.clientName + "@" + c.gatewayActorName,
				Intent:     message.IntentType.GatewayStreamOn,
				ClientName: c.clientName,
				Passcode:   c.cfg.Passcode,
				MessageId:  uuid.New().String(),
			},
		}
		streamOnSocketMsg, err := message.EncodeMessage(streamOnMsg, uuid.New().String())
		if err != nil {
			return fmt.Errorf("failed to encode STREAM ON message: %w", err)
		}

		streamOnSent, sendErr := c.conn.Send(streamOnSocketMsg.MessageBytes)
		if sendErr != nil {
			return fmt.Errorf("failed to send STREAM ON message: %w", convertGatewayError(sendErr))
		}

		if streamOnSent == 0 {
			return fmt.Errorf("failed to send STREAM ON message: no bytes sent")
		}

		c.logger.Info("re-authentication: STREAM ON message sent", "bytes", streamOnSent, "actor", c.gatewayActorName)
	}

	return nil
}

// sendMessageWithCorrelation sends a message using MessageId correlation for concurrent operations.
// The caller's response is routed via the pending map.
func (c *Client) sendMessageWithCorrelation(ctx context.Context, msg *message.Message) (*message.Message, error) {
	// Ensure MessageId exists for correlation
	if msg.MessageId == "" {
		msg.MessageId = uuid.New().String()
	}
	messageId := msg.MessageId

	// Create response channel for this request
	responseChan := make(chan *message.Message, 1)

	// Register the pending request
	c.pendingMu.Lock()
	c.pending[messageId] = responseChan
	c.pendingMu.Unlock()

	// Ensure cleanup on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, messageId)
		c.pendingMu.Unlock()
	}()

	// Encode message
	conversationUUID := uuid.New().String()
	socketMsg, err := message.EncodeMessage(msg, conversationUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to encode message: %w", err)
	}

	// Check connection before sending
	if !c.conn.IsConnected() {
		return nil, fmt.Errorf("connection closed before sending message")
	}

	// Send with mutex to prevent interleaved writes
	c.sendMu.Lock()
	sent, sendErr := c.conn.Send(socketMsg.MessageBytes)
	if c.logger.Enabled(log.LevelDebug) {
		c.logger.Debug("sending raw message", "message", string(socketMsg.MessageBytes))
	}
	c.sendMu.Unlock()

	if sendErr != nil {
		return nil, fmt.Errorf("failed to send message: %w", convertGatewayError(sendErr))
	}

	if sent == 0 {
		return nil, fmt.Errorf("failed to send message: no bytes sent")
	}

	c.logger.Info("sent message", "bytes", sent, "actor", c.gatewayActorName, "message_id", messageId)

	// Wait for response with context timeout
	select {
	case response, ok := <-responseChan:
		if !ok {
			// Channel was closed, connection was lost during request
			// Return a specific error so caller can decide to retry
			return nil, ErrConnectionLost
		}
		if response == nil {
			return nil, fmt.Errorf("received nil response from %s", c.gatewayActorName)
		}
		// Check if the response indicates a protocol error
		if response.ProcessingStatus() == "ERROR" {
			if response.ProcessingMessage() == "" {
				return response, errors.New("unknown error from Pod-OS actor")
			}
			return response, errors.New(response.ProcessingMessage())
		}
		return response, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("request to %s timed out waiting for response [MessageId: %s]: %w", c.gatewayActorName, messageId, ctx.Err())
	}
}

// sendMessageSync sends a message using synchronous send-then-receive pattern.
// Used when background receiver is not active.
func (c *Client) sendMessageSync(ctx context.Context, msg *message.Message) (*message.Message, error) {
	// Encode message
	conversationUUID := uuid.New().String()
	socketMsg, err := message.EncodeMessage(msg, conversationUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to encode message: %w", err)
	}

	// Check connection before sending
	if !c.conn.IsConnected() {
		return nil, fmt.Errorf("connection closed before sending message")
	}

	// Send via connection (with mutex in case someone starts receiver mid-operation)
	c.sendMu.Lock()
	sent, sendErr := c.conn.Send(socketMsg.MessageBytes)
	c.sendMu.Unlock()

	if sendErr != nil {
		return nil, fmt.Errorf("failed to send message: %w", convertGatewayError(sendErr))
	}

	if sent == 0 {
		return nil, fmt.Errorf("failed to send message: no bytes sent")
	}

	c.logger.Info("sent message", "bytes", sent, "actor", c.gatewayActorName)

	// Check connection before receiving
	if !c.conn.IsConnected() {
		return nil, fmt.Errorf("connection closed before receiving response")
	}

	// Receive response, respecting the context timeout
	_, responseBytes, receiveErr := c.conn.Receive(ctx)
	if receiveErr != nil {
		return nil, fmt.Errorf("failed to receive response: %w", convertGatewayError(receiveErr))
	}

	// Decode response
	responseMsg, err := message.DecodeMessage(responseBytes)
	if err != nil {
		return responseMsg, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if the response indicates a protocol error from Pod-OS actor
	if responseMsg.ProcessingStatus() == "ERROR" {
		if responseMsg.ProcessingMessage() == "" {
			return responseMsg, errors.New("unknown error from Pod-OS actor")
		}
		return responseMsg, errors.New(responseMsg.ProcessingMessage())
	}

	return responseMsg, nil
}

// isTimeoutError checks if an error string indicates a timeout
func isTimeoutError(errStr string) bool {
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "deadline exceeded") ||
		strings.Contains(errStr, "i/o timeout")
}

// isConnectionError checks if an error string indicates a connection loss
// that may be recoverable through reconnection (e.g., gateway restart).
func isConnectionError(errStr string) bool {
	return strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "use of closed network connection")
}

// Close closes the client connection and removes it from the registry.
// If the background receiver is active, it will be stopped first.
func (c *Client) Close() error {
	// Stop receiver if active
	if c.receiverActive {
		c.StopReceiver()
	} else if c.receiverCancel != nil {
		c.receiverCancel() // Cancel context even if receiver wasn't started
	}

	// Remove from both registries
	registryMu.Lock()
	if c.key != "" {
		delete(clientRegistry, c.key)
	}
	if c.gatewayActorName != "" {
		delete(actorRegistry, c.gatewayActorName)
	}
	registryMu.Unlock()
	c.logger.Info("removed client from registry", "key", c.key, "actor", c.gatewayActorName)

	if c.pool != nil {
		c.pool.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
	return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	if c.conn == nil {
		return false
	}
	return c.conn.IsConnected()
}

// IsReconnecting returns whether a reconnection attempt is currently in progress.
// Callers can use this to decide whether to wait for reconnection or fail fast.
func (c *Client) IsReconnecting() bool {
	c.reconnectingMu.Lock()
	defer c.reconnectingMu.Unlock()
	return c.reconnecting
}

// ReconnectAttempt returns the current reconnection attempt number.
// Returns 0 if not currently reconnecting.
func (c *Client) ReconnectAttempt() int {
	c.reconnectingMu.Lock()
	defer c.reconnectingMu.Unlock()
	return c.reconnectAttempt
}

// ClientName returns the ClientName used for this connection.
// This is the unique identifier used in the ID message and must be used
// for all subsequent messages on this connection.
func (c *Client) ClientName() string {
	return c.clientName
}

// ActorName returns the ActorName (gateway.domain_name) for this connection.
func (c *Client) ActorName() string {
	return c.gatewayActorName
}

// Conn returns the underlying connection.Client for direct socket operations.
// Use this when you need low-level access to the connection for sending
// pre-encoded messages or receiving raw responses.
func (c *Client) Conn() *connection.Client {
	return c.conn
}
