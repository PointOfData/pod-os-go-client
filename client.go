package podos

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/PointOfData/pod-os-go-client/config"
	"github.com/PointOfData/pod-os-go-client/connection"
	"github.com/PointOfData/pod-os-go-client/debuglog" // #region agent log
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

const (
	// receiveLoopTimeout bounds each background receive so the loop stays
	// responsive to shutdown and can enforce the liveness deadline.
	receiveLoopTimeout = 30 * time.Second
	// connectionLivenessTimeout is the liveness backstop: if requests are
	// pending but no frame has been received for this long, the connection is
	// declared dead even without a hard TCP error. Sized above receiveLoopTimeout
	// and the keepalive probe window so it only fires when TCP-level detection
	// has not already tripped.
	connectionLivenessTimeout = 90 * time.Second
)

// ConnectionState represents the current state of a Client's connection.
type ConnectionState int

const (
	StateConnected       ConnectionState = iota // Connection is active (emitted after successful reconnect)
	StateDisconnected                           // Connection was lost (err is the cause)
	StateReconnecting                           // Reconnect attempt starting (err is the trigger that caused disconnect)
	StateReconnectFailed                        // All reconnect attempts exhausted (err is the last failure)
)

func (s ConnectionState) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateDisconnected:
		return "disconnected"
	case StateReconnecting:
		return "reconnecting"
	case StateReconnectFailed:
		return "reconnect_failed"
	default:
		return "unknown"
	}
}

// responseWithRaw holds a decoded message and the raw wire bytes for callers that need both.
type responseWithRaw struct {
	Msg *message.Message
	Raw []byte
}

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
	// Preserve the fatal connection-lost classification across the conversion so
	// callers can detect it with errors.Is(err, ErrConnectionLost) and retry.
	if gatewayErr.Code == gatewayerrors.ErrCodeConnectionLost {
		if gatewayErr.OriginalError != nil {
			return fmt.Errorf("%s: %v: %w", gatewayErr.Message, gatewayErr.OriginalError, ErrConnectionLost)
		}
		return fmt.Errorf("%s: %w", gatewayErr.Message, ErrConnectionLost)
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
	keepaliveWg    sync.WaitGroup                   // Wait group for keepalive goroutine
	pendingMu      sync.RWMutex                     // Protects pending maps
	pending        map[string]chan *message.Message // messageId -> response channel
	pendingWithRaw map[string]chan *responseWithRaw // messageId -> response+raw channel

	// Reconnection state
	reconnecting     bool       // Whether a reconnection is in progress
	reconnectingMu   sync.Mutex // Protects reconnecting flag
	reconnectAttempt int        // Current reconnection attempt number
	reconnectCond    *sync.Cond // Broadcast when reconnect finishes (success or failure)
	closed           bool       // Set by Close(); prevents reconnect after shutdown

	// Connection state observer — called on every state transition. Set via
	// OnConnectionStateChange; nil means no observer. The callback is invoked
	// synchronously inside the reconnect path, so implementations should be fast
	// and non-blocking (e.g. log.Printf).
	stateHandler func(ConnectionState, error)

	// unmatchedHandler is called (in a new goroutine) when the background
	// receiver gets a message whose MessageId does not match any pending request.
	// Set via SetUnmatchedMessageHandler after NewClient returns.
	unmatchedHandler func(*message.Message)

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
		WireHook:       cfg.WireHook,
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

	if logger.Enabled(log.LevelDebug) {
		logger.Debug("wire: sending GatewayId frame",
			"intent",       idMsg.Envelope.Intent.Name,
			"message_type", idMsg.Envelope.Intent.MessageType,
			"to",           idMsg.Envelope.To,
			"from",         idMsg.Envelope.From,
			"msg_id",       idMsg.Envelope.MessageId,
			"header",       socketMsg.Header,
			"total_bytes",  len(socketMsg.MessageBytes),
		)
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

		if logger.Enabled(log.LevelDebug) {
			logger.Debug("wire: sending GatewayStreamOn frame",
				"intent",       streamOnMsg.Envelope.Intent.Name,
				"message_type", streamOnMsg.Envelope.Intent.MessageType,
				"to",           streamOnMsg.Envelope.To,
				"from",         streamOnMsg.Envelope.From,
				"msg_id",       streamOnMsg.Envelope.MessageId,
				"header",       streamOnSocketMsg.Header,
				"total_bytes",  len(streamOnSocketMsg.MessageBytes),
			)
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
		pendingWithRaw:   make(map[string]chan *responseWithRaw),
		logger:           logger,
	}
	client.reconnectCond = sync.NewCond(&client.reconnectingMu)

	if cfg.UnmatchedMessageHandler != nil {
		client.unmatchedHandler = cfg.UnmatchedMessageHandler
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
		client.startKeepaliveLoop()
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

	client.startKeepaliveLoop()

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
	c.normalizeMessageFrom(msg)

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

// SendMessageWithRaw sends a message and returns both the decoded response and the raw wire bytes.
// Use this when the caller needs to display or log the undecoded server response (e.g. for a "Raw" tab).
func (c *Client) SendMessageWithRaw(ctx context.Context, msg *message.Message) (*message.Message, []byte, error) {
	c.normalizeMessageFrom(msg)
	if msg.MessageId == "" {
		msg.MessageId = uuid.New().String()
	}
	if c.receiverActive {
		return c.sendMessageWithCorrelationRaw(ctx, msg)
	}
	return c.sendMessageSyncRaw(ctx, msg)
}

// SendControlMessage sends a control message (no response expected)
func (c *Client) SendControlMessage(ctx context.Context, msg *message.SocketMessage) error {
	// Check connection before sending
	if !c.conn.IsConnected() {
		return fmt.Errorf("connection closed before sending control message")
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

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

// startKeepaliveLoop starts a background goroutine that sends app-level AIP
// Keepalive frames on the primary connection and idle pooled connections.
func (c *Client) startKeepaliveLoop() {
	interval := c.cfg.GetKeepaliveInterval()
	if interval <= 0 {
		return
	}

	c.keepaliveWg.Add(1)
	go func() {
		defer c.keepaliveWg.Done()
		c.keepaliveLoop(interval)
	}()
}

func (c *Client) keepaliveLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.receiverCtx.Done():
			return
		case <-ticker.C:
			if c.closed || !c.IsConnected() {
				continue
			}
			c.reconnectingMu.Lock()
			reconnecting := c.reconnecting
			c.reconnectingMu.Unlock()
			if reconnecting {
				continue
			}
			if err := c.sendKeepalive(); err != nil {
				if c.logger.Enabled(log.LevelDebug) {
					c.logger.Debug("keepalive send failed", "actor", c.gatewayActorName, "error", err)
				}
			}
			if c.pool != nil {
				c.sendPoolKeepalives()
			}
		}
	}
}

func (c *Client) buildKeepaliveMessage() *message.Message {
	return &message.Message{
		Envelope: message.Envelope{
			To:         "$system@" + c.gatewayActorName,
			From:       c.FromAddress(),
			Intent:     message.IntentType.Keepalive,
			ClientName: c.clientName,
			MessageId:  uuid.New().String(),
		},
	}
}

func (c *Client) encodeKeepalive() ([]byte, error) {
	msg := c.buildKeepaliveMessage()
	socketMsg, err := message.EncodeMessage(msg, uuid.New().String())
	if err != nil {
		return nil, err
	}
	return socketMsg.MessageBytes, nil
}

// sendKeepalive sends a fire-and-forget Keepalive on the primary connection.
func (c *Client) sendKeepalive() error {
	wire, err := c.encodeKeepalive()
	if err != nil {
		return fmt.Errorf("encode keepalive: %w", err)
	}
	if !c.conn.IsConnected() {
		return fmt.Errorf("connection closed before sending keepalive")
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if !c.conn.IsConnected() {
		return fmt.Errorf("connection closed before sending keepalive")
	}

	sent, sendErr := c.conn.Send(wire)
	if sendErr != nil {
		return fmt.Errorf("failed to send keepalive: %w", convertGatewayError(sendErr))
	}
	if sent == 0 {
		return fmt.Errorf("failed to send keepalive: no bytes sent")
	}

	if c.logger.Enabled(log.LevelDebug) {
		c.logger.Debug("sent keepalive", "bytes", sent, "actor", c.gatewayActorName)
	}
	return nil
}

func (c *Client) sendPoolKeepalives() {
	wire, err := c.encodeKeepalive()
	if err != nil {
		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("encode pool keepalive failed", "error", err)
		}
		return
	}

	sendTimeout := c.cfg.SendTimeout
	if sendTimeout <= 0 {
		sendTimeout = 5 * time.Second
	}

	c.pool.PingIdleConnections(func(conn net.Conn) error {
		if err := conn.SetWriteDeadline(time.Now().Add(sendTimeout)); err != nil {
			return err
		}
		n, err := conn.Write(wire)
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("no bytes sent on pooled connection")
		}
		return nil
	})
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
	for msgId, ch := range c.pendingWithRaw {
		close(ch)
		delete(c.pendingWithRaw, msgId)
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
	// lastActivity is the last time a full frame was received (or a reconnect
	// succeeded). It backstops the TCP-level detection: if requests are pending
	// but no frame arrives for connectionLivenessTimeout, the connection is dead.
	lastActivity := time.Now()
	// #region agent log
	debuglog.Log("H-A", "client.go:receiveLoop", "receiveLoop started BUILD=dead-conn-fix-v2", map[string]any{
		"actor": c.gatewayActorName, "reconnectEnabled": c.cfg.ReconnectConfig.IsEnabled(),
	})
	var dbgLastHB time.Time
	var dbgLastOutcome string = "none"
	defer func() {
		debuglog.Log("H-G", "client.go:receiveLoop.exit", "receiveLoop RETURNED", map[string]any{
			"pendingCount": c.pendingCount(), "lastOutcome": dbgLastOutcome,
			"receiverCtxErr": fmt.Sprint(c.receiverCtx.Err()),
		})
	}()
	// #endregion
	for {
		// #region agent log
		if time.Since(dbgLastHB) >= 30*time.Second {
			debuglog.Log("H-F", "client.go:receiveLoop.hb", "loop alive, about to Receive", map[string]any{
				"pendingCount": c.pendingCount(), "sinceLastActivityMs": time.Since(lastActivity).Milliseconds(),
				"connected": c.IsConnected(), "lastOutcome": dbgLastOutcome,
			})
			dbgLastHB = time.Now()
		}
		// #endregion
		select {
		case <-c.receiverCtx.Done():
			c.logger.Info("receiver context cancelled, exiting loop", "actor", c.gatewayActorName)
			return
		default:
			// Continue receiving
		}

		// Check if still connected. The transport may have been marked dead by a
		// concurrent sender's failed write; fail in-flight callers, then reconnect.
		if !c.IsConnected() {
			c.handleConnectionLost(ErrConnectionLost)
			if c.cfg.ReconnectConfig.IsEnabled() {
				if c.attemptReconnection(nil) {
					lastActivity = time.Now()
					continue
				}
			}
			c.logger.Info("connection lost, receiver exiting", "actor", c.gatewayActorName)
			return
		}

		// Receive with a timeout to allow checking for shutdown and to enforce
		// the liveness deadline.
		receiveCtx, cancel := context.WithTimeout(c.receiverCtx, receiveLoopTimeout)
		dbgRecvStart := time.Now() // #region agent log #endregion
		_, responseBytes, receiveErr := c.conn.Receive(receiveCtx)
		cancel()
		// #region agent log
		dbgElapsed := time.Since(dbgRecvStart)
		if receiveErr != nil {
			dbgLastOutcome = fmt.Sprintf("err(%dms): %s", dbgElapsed.Milliseconds(), receiveErr.Error())
		} else {
			dbgLastOutcome = fmt.Sprintf("ok(%dms,len=%d)", dbgElapsed.Milliseconds(), len(responseBytes))
		}
		// A single Receive that blocks far past the receiveLoopTimeout proves the
		// read deadline is not being enforced on this socket.
		if dbgElapsed > 2*receiveLoopTimeout {
			debuglog.Log("H-F", "client.go:receiveLoop.slow", "Receive exceeded 2x timeout", map[string]any{
				"elapsedMs": dbgElapsed.Milliseconds(), "outcome": dbgLastOutcome,
				"pendingCount": c.pendingCount(),
			})
		}
		// #endregion

		if receiveErr != nil {
			// Check if receiver is shutting down
			if c.receiverCtx.Err() != nil {
				return
			}

			// #region agent log
			isIdle := gatewayerrors.IsIdleTimeout(receiveErr)
			pc := c.pendingCount()
			idleMs := time.Since(lastActivity).Milliseconds()
			if !isIdle || pc > 0 {
				debuglog.Log("H-B", "client.go:receiveLoop.err", "receive error classified", map[string]any{
					"isIdleTimeout": isIdle, "isConnLost": gatewayerrors.IsConnectionLost(receiveErr),
					"pendingCount": pc, "sinceLastActivityMs": idleMs,
					"livenessThresholdMs": connectionLivenessTimeout.Milliseconds(),
					"err":                 receiveErr.Error(),
				})
			}
			// #endregion

			// Benign idle timeout: no frame was in progress, so the socket is
			// still considered healthy. Continue UNLESS we have pending requests
			// and have heard nothing for too long (liveness backstop).
			if gatewayerrors.IsIdleTimeout(receiveErr) {
				if c.pendingCount() == 0 || time.Since(lastActivity) <= connectionLivenessTimeout {
					continue
				}
				c.logger.Error("liveness timeout: pending requests with no frames received; treating connection as dead",
					"actor", c.gatewayActorName, "idle", time.Since(lastActivity))
				// #region agent log
				debuglog.Log("H-D", "client.go:receiveLoop.liveness", "liveness backstop fired -> fatal", map[string]any{
					"pendingCount": c.pendingCount(), "sinceLastActivityMs": time.Since(lastActivity).Milliseconds(),
				})
				// #endregion
			} else {
				c.logger.Error("connection error", "actor", c.gatewayActorName, "error", receiveErr)
			}

			// #region agent log
			debuglog.Log("H-C", "client.go:receiveLoop.fatal", "handling fatal -> notify pending + reconnect", map[string]any{
				"pendingCount": c.pendingCount(), "err": receiveErr.Error(),
			})
			// #endregion

			// Fatal: mark down, fail every in-flight caller fast with
			// ErrConnectionLost, then reconnect.
			c.handleConnectionLost(receiveErr)
			if c.cfg.ReconnectConfig.IsEnabled() {
				if c.attemptReconnection(receiveErr) {
					lastActivity = time.Now()
					continue
				}
			}
			c.logger.Error("connection recovery failed, receiver exiting", "actor", c.gatewayActorName)
			return
		}

		lastActivity = time.Now()

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
		rawChan, rawExists := c.pendingWithRaw[messageId]
		responseChan, exists := c.pending[messageId]
		c.pendingMu.RUnlock()

		if rawExists {
			select {
			case rawChan <- &responseWithRaw{Msg: response, Raw: responseBytes}:
				if c.logger.Enabled(log.LevelDebug) {
					c.logger.Debug("routed response+raw to caller", "message_id", messageId)
				}
			default:
				if c.logger.Enabled(log.LevelDebug) {
					c.logger.Debug("caller already gone", "message_id", messageId, "msg", "timed out")
				}
			}
		} else if exists {
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
				c.logger.Debug("no pending request for MessageId — dispatching to unmatched handler", "message_id", messageId, "intent", response.Envelope.Intent.Name)
			}
			if c.unmatchedHandler != nil {
				go c.unmatchedHandler(response)
			}
		}
	}
}

// OnConnectionStateChange registers a callback that fires on every connection state
// transition. The error parameter is:
//   - StateDisconnected: the error that caused the disconnect.
//   - StateReconnecting: the trigger error (may be nil if the trigger is unknown).
//   - StateConnected: nil (reconnect succeeded).
//   - StateReconnectFailed: the last reconnect attempt error.
//
// Note: StateConnected is not emitted for the initial connection in NewClient because
// no handler can be registered before the constructor returns. It is only emitted
// after a successful reconnect.
//
// The callback is invoked synchronously so it should be fast and non-blocking.
// Safe to call before or after StartReceiver.
func (c *Client) OnConnectionStateChange(fn func(ConnectionState, error)) {
	c.reconnectingMu.Lock()
	c.stateHandler = fn
	c.reconnectingMu.Unlock()
}

// emitState calls the registered state handler, if any.
func (c *Client) emitState(state ConnectionState, err error) {
	c.reconnectingMu.Lock()
	fn := c.stateHandler
	c.reconnectingMu.Unlock()
	if fn != nil {
		fn(state, err)
	}
}

// waitForReconnect blocks until the client is connected or the context expires.
// Returns true if the connection was restored, false otherwise.
func (c *Client) waitForReconnect(ctx context.Context) bool {
	if c.IsConnected() {
		return true
	}

	// Poll via the condition variable that attemptReconnection broadcasts on.
	// We use a goroutine to bridge ctx cancellation into the cond.Wait loop
	// because sync.Cond has no native context support.
	done := make(chan bool, 1)
	go func() {
		c.reconnectingMu.Lock()
		defer c.reconnectingMu.Unlock()
		for !c.conn.IsConnected() && !c.closed {
			// If no reconnect is happening and we're not connected, give up.
			if !c.reconnecting {
				done <- false
				return
			}
			c.reconnectCond.Wait()
		}
		done <- c.conn.IsConnected() && !c.closed
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		// Unblock the waiting goroutine by broadcasting; it will re-check and exit.
		c.reconnectCond.Broadcast()
		return false
	}
}

// SetUnmatchedMessageHandler registers a handler that is called (in a new goroutine)
// for every message received by the background receiver that does not match any
// pending outbound request. This enables an Actor to receive inbound ActorRequest
// messages from other Actors while still using concurrent mode for its own queries.
// Must be called before StartReceiver (or before NewClient when EnableConcurrentMode
// is true) to avoid a race; calling it after is safe but may miss early messages.
func (c *Client) SetUnmatchedMessageHandler(fn func(*message.Message)) {
	c.unmatchedHandler = fn
}

// pendingCount returns the number of in-flight requests awaiting a response.
func (c *Client) pendingCount() int {
	c.pendingMu.RLock()
	defer c.pendingMu.RUnlock()
	return len(c.pending) + len(c.pendingWithRaw)
}

// handleConnectionLost emits the disconnected state and fails every in-flight
// caller immediately with ErrConnectionLost so they can retry, rather than
// blocking until each request's own deadline.
func (c *Client) handleConnectionLost(err error) {
	c.emitState(StateDisconnected, err)
	c.notifyPendingCallersConnectionLost()
}

// notifyPendingCallersConnectionLost notifies all pending callers that the connection was lost.
// Callers will receive a nil message, indicating they should retry their request.
func (c *Client) notifyPendingCallersConnectionLost() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for msgId, ch := range c.pending {
		close(ch)
		delete(c.pending, msgId)
		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("notified pending caller of connection loss", "message_id", msgId)
		}
	}
	for msgId, ch := range c.pendingWithRaw {
		close(ch)
		delete(c.pendingWithRaw, msgId)
		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("notified pending raw caller of connection loss", "message_id", msgId)
		}
	}
}

// attemptReconnection attempts to reconnect to the gateway with exponential backoff.
// triggerErr is the error that caused the reconnection attempt (e.g. the receive error
// from receiveLoop); it is forwarded to the StateReconnecting callback so observers
// know why the reconnect started. Pass nil when the trigger is unknown.
// Returns true if reconnection was successful, false otherwise.
// On completion (success or failure) it broadcasts on reconnectCond so that
// waitForReconnect callers are unblocked.
func (c *Client) attemptReconnection(triggerErr error) bool {
	c.reconnectingMu.Lock()
	if c.closed {
		c.reconnectingMu.Unlock()
		return false
	}
	if c.reconnecting {
		c.reconnectingMu.Unlock()
		// Another goroutine is already reconnecting — wait for its result
		// instead of sleeping a fixed duration.
		return c.waitForReconnect(c.receiverCtx)
	}
	c.reconnecting = true
	c.reconnectAttempt = 0
	c.reconnectingMu.Unlock()

	c.emitState(StateReconnecting, triggerErr)

	finishReconnect := func(success bool, lastErr error) {
		c.reconnectingMu.Lock()
		c.reconnecting = false
		c.reconnectCond.Broadcast()
		c.reconnectingMu.Unlock()
		// #region agent log
		le := ""
		if lastErr != nil {
			le = lastErr.Error()
		}
		debuglog.Log("H-C", "client.go:attemptReconnection", "reconnect finished", map[string]any{
			"success": success, "lastErr": le,
		})
		// #endregion
		if success {
			c.emitState(StateConnected, nil)
		} else {
			c.emitState(StateReconnectFailed, lastErr)
		}
	}

	reconnectCfg := &c.cfg.ReconnectConfig
	maxRetries := reconnectCfg.MaxRetries
	unlimited := maxRetries == 0

	backoff := reconnectCfg.GetInitialBackoff()
	multiplier := reconnectCfg.GetBackoffMultiplier()
	maxBackoff := reconnectCfg.GetMaxBackoff()

	var lastErr error
	for attempt := 1; unlimited || attempt <= maxRetries; attempt++ {
		// Check if receiver context is cancelled or client was closed
		select {
		case <-c.receiverCtx.Done():
			c.logger.Info("reconnection cancelled", "actor", c.gatewayActorName, "msg", "context done")
			finishReconnect(false, c.receiverCtx.Err())
			return false
		default:
		}

		c.reconnectingMu.Lock()
		if c.closed {
			c.reconnectingMu.Unlock()
			finishReconnect(false, errors.New("client closed"))
			return false
		}
		c.reconnectAttempt = attempt
		c.reconnectingMu.Unlock()

		c.logger.Info("reconnection attempt", "attempt", attempt, "max_retries", maxRetries, "actor", c.gatewayActorName, "backoff", backoff)

		// Attempt to reconnect the underlying connection
		if err := c.conn.Reconnect(); err != nil {
			lastErr = err
			c.logger.Warn("reconnection attempt failed", "attempt", attempt, "actor", c.gatewayActorName, "error", err)

			// Wait with exponential backoff before next attempt
			select {
			case <-c.receiverCtx.Done():
				c.logger.Info("reconnection cancelled during backoff", "actor", c.gatewayActorName)
				finishReconnect(false, c.receiverCtx.Err())
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
			lastErr = err
			c.logger.Error("re-authentication failed", "actor", c.gatewayActorName, "error", err)
			// Close the connection and try again
			c.conn.Close()

			// Wait before next attempt
			select {
			case <-c.receiverCtx.Done():
				finishReconnect(false, c.receiverCtx.Err())
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
		finishReconnect(true, nil)
		return true
	}

	c.logger.Warn("max reconnection attempts reached", "actor", c.gatewayActorName)
	finishReconnect(false, lastErr)
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

	if c.logger.Enabled(log.LevelDebug) {
		c.logger.Debug("wire: sending GatewayId frame (re-auth)",
			"intent",       idMsg.Envelope.Intent.Name,
			"message_type", idMsg.Envelope.Intent.MessageType,
			"to",           idMsg.Envelope.To,
			"from",         idMsg.Envelope.From,
			"msg_id",       idMsg.Envelope.MessageId,
			"header",       socketMsg.Header,
			"total_bytes",  len(socketMsg.MessageBytes),
		)
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

		if c.logger.Enabled(log.LevelDebug) {
			c.logger.Debug("wire: sending GatewayStreamOn frame (re-auth)",
				"intent",       streamOnMsg.Envelope.Intent.Name,
				"message_type", streamOnMsg.Envelope.Intent.MessageType,
				"to",           streamOnMsg.Envelope.To,
				"from",         streamOnMsg.Envelope.From,
				"msg_id",       streamOnMsg.Envelope.MessageId,
				"header",       streamOnSocketMsg.Header,
				"total_bytes",  len(streamOnSocketMsg.MessageBytes),
			)
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

	// If connection is down, wait for the receiveLoop's reconnect rather than
	// failing immediately. This avoids forcing every caller to wrap sends in
	// their own reconnect logic.
	if !c.conn.IsConnected() {
		if c.cfg.ReconnectConfig.IsEnabled() {
			c.logger.Info("connection down, waiting for reconnect before send", "actor", c.gatewayActorName, "message_id", messageId)
			if !c.waitForReconnect(ctx) {
				return nil, ErrConnectionLost
			}
		} else {
			return nil, fmt.Errorf("connection closed before sending message")
		}
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
			// #region agent log
			debuglog.Log("H-C", "client.go:sendMessageWithCorrelation", "responseChan CLOSED -> ErrConnectionLost (recovery path)", map[string]any{
				"messageId": messageId,
			})
			// #endregion
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
		// #region agent log
		debuglog.Log("H-E", "client.go:sendMessageWithCorrelation", "ctx.Done fired -> context deadline (NO detection)", map[string]any{
			"messageId": messageId, "ctxErr": ctx.Err().Error(),
			"connected": c.conn.IsConnected(), "pendingCount": c.pendingCount(),
		})
		// #endregion
		return nil, fmt.Errorf("request to %s timed out waiting for response [MessageId: %s]: %w", c.gatewayActorName, messageId, ctx.Err())
	}
}

// sendMessageSync sends a message using synchronous send-then-receive pattern.
// Used when background receiver is not active.
// If a connection error occurs and reconnection is enabled, it will attempt to
// reconnect and retry the message once.
func (c *Client) sendMessageSync(ctx context.Context, msg *message.Message) (*message.Message, error) {
	resp, err := c.doSendMessageSync(ctx, msg)
	if err != nil && c.cfg.ReconnectConfig.IsEnabled() && isFatalConnError(err) {
		c.logger.Info("sync send failed with connection error, attempting reconnection", "actor", c.gatewayActorName, "error", err)
		if c.attemptReconnection(err) {
			return c.doSendMessageSync(ctx, msg)
		}
	}
	return resp, err
}

// doSendMessageSync performs the actual synchronous send-then-receive.
func (c *Client) doSendMessageSync(ctx context.Context, msg *message.Message) (*message.Message, error) {
	// Encode message
	conversationUUID := uuid.New().String()
	socketMsg, err := message.EncodeMessage(msg, conversationUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to encode message: %w", err)
	}

	// Wait for reconnect if connection is down and reconnect is enabled
	if !c.conn.IsConnected() {
		if c.cfg.ReconnectConfig.IsEnabled() {
			if !c.waitForReconnect(ctx) {
				return nil, ErrConnectionLost
			}
		} else {
			return nil, fmt.Errorf("connection closed before sending message")
		}
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

// sendMessageSyncRaw is like sendMessageSync but also returns the raw response bytes.
// If a connection error occurs and reconnection is enabled, it will attempt to
// reconnect and retry the message once.
func (c *Client) sendMessageSyncRaw(ctx context.Context, msg *message.Message) (*message.Message, []byte, error) {
	resp, raw, err := c.doSendMessageSyncRaw(ctx, msg)
	if err != nil && c.cfg.ReconnectConfig.IsEnabled() && isFatalConnError(err) {
		c.logger.Info("sync send (raw) failed with connection error, attempting reconnection", "actor", c.gatewayActorName, "error", err)
		if c.attemptReconnection(err) {
			return c.doSendMessageSyncRaw(ctx, msg)
		}
	}
	return resp, raw, err
}

// doSendMessageSyncRaw performs the actual synchronous send-then-receive with raw bytes.
func (c *Client) doSendMessageSyncRaw(ctx context.Context, msg *message.Message) (*message.Message, []byte, error) {
	conversationUUID := uuid.New().String()
	socketMsg, err := message.EncodeMessage(msg, conversationUUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode message: %w", err)
	}
	if !c.conn.IsConnected() {
		if c.cfg.ReconnectConfig.IsEnabled() {
			if !c.waitForReconnect(ctx) {
				return nil, nil, ErrConnectionLost
			}
		} else {
			return nil, nil, fmt.Errorf("connection closed before sending message")
		}
	}
	c.sendMu.Lock()
	sent, sendErr := c.conn.Send(socketMsg.MessageBytes)
	c.sendMu.Unlock()
	if sendErr != nil {
		return nil, nil, fmt.Errorf("failed to send message: %w", convertGatewayError(sendErr))
	}
	if sent == 0 {
		return nil, nil, fmt.Errorf("failed to send message: no bytes sent")
	}
	c.logger.Info("sent message", "bytes", sent, "actor", c.gatewayActorName)
	if !c.conn.IsConnected() {
		return nil, nil, fmt.Errorf("connection closed before receiving response")
	}
	_, responseBytes, receiveErr := c.conn.Receive(ctx)
	if receiveErr != nil {
		return nil, nil, fmt.Errorf("failed to receive response: %w", convertGatewayError(receiveErr))
	}
	responseMsg, err := message.DecodeMessage(responseBytes)
	if err != nil {
		return responseMsg, responseBytes, fmt.Errorf("failed to decode response: %w", err)
	}
	if responseMsg.ProcessingStatus() == "ERROR" {
		if responseMsg.ProcessingMessage() == "" {
			return responseMsg, responseBytes, errors.New("unknown error from Pod-OS actor")
		}
		return responseMsg, responseBytes, errors.New(responseMsg.ProcessingMessage())
	}
	return responseMsg, responseBytes, nil
}

// sendMessageWithCorrelationRaw is like sendMessageWithCorrelation but returns raw response bytes via pendingWithRaw.
func (c *Client) sendMessageWithCorrelationRaw(ctx context.Context, msg *message.Message) (*message.Message, []byte, error) {
	messageId := msg.MessageId
	if messageId == "" {
		messageId = uuid.New().String()
		msg.MessageId = messageId
	}
	rawChan := make(chan *responseWithRaw, 1)
	c.pendingMu.Lock()
	c.pendingWithRaw[messageId] = rawChan
	c.pendingMu.Unlock()
	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingWithRaw, messageId)
		c.pendingMu.Unlock()
	}()
	conversationUUID := uuid.New().String()
	socketMsg, err := message.EncodeMessage(msg, conversationUUID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to encode message: %w", err)
	}
	if !c.conn.IsConnected() {
		if c.cfg.ReconnectConfig.IsEnabled() {
			if !c.waitForReconnect(ctx) {
				return nil, nil, ErrConnectionLost
			}
		} else {
			return nil, nil, fmt.Errorf("connection closed before sending message")
		}
	}
	c.sendMu.Lock()
	sent, sendErr := c.conn.Send(socketMsg.MessageBytes)
	c.sendMu.Unlock()
	if sendErr != nil {
		return nil, nil, fmt.Errorf("failed to send message: %w", convertGatewayError(sendErr))
	}
	if sent == 0 {
		return nil, nil, fmt.Errorf("failed to send message: no bytes sent")
	}
	c.logger.Info("sent message", "bytes", sent, "actor", c.gatewayActorName, "message_id", messageId)
	select {
	case rawResp, ok := <-rawChan:
		if !ok {
			return nil, nil, ErrConnectionLost
		}
		if rawResp == nil || rawResp.Msg == nil {
			return nil, nil, fmt.Errorf("received nil response from %s", c.gatewayActorName)
		}
		if rawResp.Msg.ProcessingStatus() == "ERROR" {
			errMsg := rawResp.Msg.ProcessingMessage()
			if errMsg == "" {
				errMsg = "unknown error from Pod-OS actor"
			}
			return rawResp.Msg, rawResp.Raw, errors.New(errMsg)
		}
		return rawResp.Msg, rawResp.Raw, nil
	case <-ctx.Done():
		return nil, nil, fmt.Errorf("request to %s timed out waiting for response [MessageId: %s]: %w", c.gatewayActorName, messageId, ctx.Err())
	}
}

// isFatalConnError reports whether a transport-layer error signals a dead
// connection that requires reconnection. Classification is typed (the transport
// tags fatal conditions with ErrCodeConnectionLost) rather than string-matching.
func isFatalConnError(err error) bool {
	return gatewayerrors.IsConnectionLost(err) || errors.Is(err, ErrConnectionLost)
}

// Close closes the client connection and removes it from the registry.
// If the background receiver is active, it will be stopped first.
// After Close returns, the client will never reconnect.
func (c *Client) Close() error {
	// Mark as closed so attemptReconnection and waitForReconnect give up.
	c.reconnectingMu.Lock()
	c.closed = true
	c.reconnectCond.Broadcast()
	c.reconnectingMu.Unlock()

	// Stop receiver if active
	if c.receiverActive {
		c.StopReceiver()
	} else if c.receiverCancel != nil {
		c.receiverCancel() // Cancel context even if receiver wasn't started
	}

	// Stop keepalive loop (uses receiverCtx; wait for goroutine exit).
	c.keepaliveWg.Wait()

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

// ActorName returns the connection gateway FQN (gateway.domain) for this client.
func (c *Client) ActorName() string {
	return c.gatewayActorName
}

// FromAddress returns the sender routing identity for messages sent on this
// connection: "<clientName>@<connectionGatewayFQN>".
func (c *Client) FromAddress() string {
	return c.clientName + "@" + c.gatewayActorName
}

// normalizeMessageFrom ensures ClientName and From use this connection's identity.
// From always uses the connection gateway, not the routing target in To.
func (c *Client) normalizeMessageFrom(msg *message.Message) {
	if msg.ClientName != c.clientName {
		c.logger.Info("updating message ClientName", "from", msg.ClientName, "to", c.clientName)
		msg.ClientName = c.clientName
	}
	expectedFrom := c.FromAddress()
	if msg.From != expectedFrom {
		if msg.From != "" {
			c.logger.Info("updating message From", "from", msg.From, "to", expectedFrom)
		}
		msg.From = expectedFrom
	}
}

// Conn returns the underlying connection.Client for direct socket operations.
// Use this when you need low-level access to the connection for sending
// pre-encoded messages or receiving raw responses.
func (c *Client) Conn() *connection.Client {
	return c.conn
}
