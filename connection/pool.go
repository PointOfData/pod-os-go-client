package connection

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	// ErrClosed is the error resulting if the pool is closed via pool.Close().
	ErrClosed = errors.New("pool is closed")
)

// Pool interface describes a pool implementation. A pool should have maximum
// capacity. An ideal pool is thread-safe and easy to use.
type Pool interface {
	// Get returns a new connection from the pool. Closing the connections puts
	// it back to the Pool. Closing it when the pool is destroyed or full will
	// be counted as an error.
	Get(context.Context) (ConnectionData, error)

	// Close closes the pool and all its connections. After Close() the pool is
	// no longer usable.
	Close()

	// Len returns the current number of idle connections of the pool.
	Len() int

	// NumberOfConns returns the total number of alive connections of the pool.
	NumberOfConns() int
}

// ConnectionData represents a connection with its UUID
type ConnectionData struct {
	Conns            net.Conn
	ConnectionIdUuid string
}

// ConnectionFactory is a function to create new connections.
type ConnectionFactory func() (net.Conn, string, error)

// ChannelPool implements the Pool interface based on buffered channels.
type ChannelPool struct {
	// storage for our net.Conn connections
	mu       sync.RWMutex
	aipConns chan ConnectionData

	maxCap    int
	semaphore chan struct{}

	// Connection generator
	factory ConnectionFactory
}

// PoolConn is a wrapper around net.Conn to modify the behavior of
// net.Conn's Close() method.
type PoolConn struct {
	net.Conn
	mu       sync.RWMutex
	c        *ChannelPool
	unusable bool
}

// Close() puts the given connects back to the pool instead of closing it.
func (p *PoolConn) Close() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.unusable {
		if p.Conn == nil {
			<-p.c.semaphore
			return p.Conn.Close()
		}
		return nil
	}

	return p.c.put(ConnectionData{Conns: p.Conn, ConnectionIdUuid: ""})
}

// MarkUnusable() marks the connection not usable any more, to let the pool close it instead of returning it to pool.
func (p *PoolConn) MarkUnusable() {
	p.mu.Lock()
	p.unusable = true
	p.mu.Unlock()
}

// wrapConn wraps a standard net.Conn to a poolConn net.Conn.
func (c *ChannelPool) wrapConn(conn net.Conn) net.Conn {
	p := &PoolConn{c: c}
	p.Conn = conn
	return p
}

// NewChannelPool creates a new channel pool with the given capacity
func NewChannelPool(maxCap int, factory ConnectionFactory) *ChannelPool {
	return &ChannelPool{
		aipConns:  make(chan ConnectionData, maxCap),
		semaphore: make(chan struct{}, maxCap),
		maxCap:    maxCap,
		factory:   factory,
	}
}

// InitializeChannelPool initializes a pool with an initial capacity.
// Factory is used when initial capacity is greater than zero to fill the pool.
// A zero initialCap doesn't fill the Pool until a new Get() is called.
func InitializeChannelPool(c *ChannelPool, initialCap int) error {
	if initialCap < 0 || c.maxCap <= 0 || initialCap > c.maxCap {
		return errors.New("invalid capacity settings")
	}

	var aipConn ConnectionData

	// create initial connections, if something goes wrong,
	// just close the pool error out.
	for i := 0; i < initialCap; i++ {
		c.semaphore <- struct{}{}
		conn, connectionIdUuid, err := c.factory()
		if err != nil {
			c.Close()
			return fmt.Errorf("connection factory encountered an error and is not able to fill the pool: %w", err)
		}

		aipConn.Conns = conn
		aipConn.ConnectionIdUuid = connectionIdUuid

		c.aipConns <- aipConn
	}

	return nil
}

func (c *ChannelPool) getConnsAndFactory() (chan ConnectionData, ConnectionFactory) {
	c.mu.RLock()
	aipConn := c.aipConns // c.aipConn.conn is protected by c.mu
	factory := c.factory
	c.mu.RUnlock()
	return aipConn, factory
}

// Get implements the Pool interfaces Get() method. If there is no new
// connection available in the pool, a new connection will be created via the
// Factory() method.
func (c *ChannelPool) Get(ctx context.Context) (ConnectionData, error) {
	aipConn, factory := c.getConnsAndFactory()
	if aipConn == nil {
		return ConnectionData{}, ErrClosed
	}

	// wrap our connections with out custom net.Conn implementation (wrapConn
	// method) that puts the connection back to the pool if it's closed.
	select {
	case aipConn := <-aipConn:
		if aipConn.Conns == nil {
			return ConnectionData{}, ErrClosed
		}
		aipConn.Conns = c.wrapConn(aipConn.Conns)
		return aipConn, nil
	default:
	}

	select {
	case c.semaphore <- struct{}{}:
		var aipConn ConnectionData
		conn, connectionIdUuid, err := factory()
		if err != nil {
			// restore claimed slot, otherwise max is permanently decreased
			<-c.semaphore
			return ConnectionData{}, err
		}
		aipConn.Conns = conn
		aipConn.ConnectionIdUuid = connectionIdUuid

		return aipConn, nil
	case aipConn := <-aipConn:
		if aipConn.Conns == nil {
			return ConnectionData{}, ErrClosed
		}
		aipConn.Conns = c.wrapConn(aipConn.Conns)
		return aipConn, nil
	case <-ctx.Done():
		return ConnectionData{}, ctx.Err()
	}
}

// put puts the connection back to the pool. If the pool is full or closed,
// conn is simply closed. A nil conn will be rejected.
func (c *ChannelPool) put(aipConn ConnectionData) error {
	if aipConn.Conns == nil {
		return errors.New("connection is nil. rejecting")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.aipConns == nil {
		// pool is closed, close passed connection
		// close net.conn
		return aipConn.Conns.Close()
	}

	// put the resource back into the pool. If the pool is full, this will
	// block and the default case will be executed.
	select {
	case c.aipConns <- aipConn:
		return nil
	default:
		<-c.semaphore
		// pool is full, remove the aipConn from the pool and close the connection
		// close net.conn
		aipConn.Conns.Close()
		return nil // aipConn.Close()
	}
}

// Close closes the pool and all its connections
func (c *ChannelPool) Close() {
	c.mu.Lock()
	aipConns := c.aipConns
	c.aipConns = nil
	c.factory = nil
	c.mu.Unlock()

	if aipConns == nil {
		return
	}

	close(aipConns)
	for conn := range aipConns {
		conn.Conns.Close()
	}
}

// Len returns the current number of idle connections
func (c *ChannelPool) Len() int {
	conns, _ := c.getConnsAndFactory()
	return len(conns)
}

// NumberOfConns returns the total number of alive connections
func (c *ChannelPool) NumberOfConns() int {
	return len(c.semaphore)
}
