package queue

import (
	"context"
	"fmt"
	"sync"
)

// PgListener helper that manages a persistent LISTEN connection in postgres
// A persistent connection avoids missing notifications between LISTEN and UNLISTEN calls
// One instance of PgListener should only be used and closed by one goroutine
type PgListener struct {
	pool           Pool
	channelName    string
	persistentConn Connection
	mu             sync.Mutex
}

func NewPgListener(pool Pool, channelName string) *PgListener {
	return &PgListener{
		pool:        pool,
		channelName: channelName,
	}
}

// ensureConnection establishes a persistent connection that listens on p.channelName
func (p *PgListener) ensureConnection(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.persistentConn == nil {
		conn, err := p.pool.Acquire(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire connection: %w", err)
		}
		_, err = conn.Exec(ctx, "LISTEN "+p.channelName)
		if err != nil {
			conn.Release()
			return fmt.Errorf("failed to listen on channel %s: %w", p.channelName, err)
		}
		p.persistentConn = conn
	}
	return nil
}

// WaitForNotification create a connection to listen on the channel
// if a connection has been created, skip straight to waiting for a notification
func (p *PgListener) WaitForNotification(ctx context.Context) error {
	err := p.ensureConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire persistent connection: %w", err)
	}

	_, err = p.persistentConn.Conn().WaitForNotification(ctx)
	if err != nil {
		p.mu.Lock()
		if p.persistentConn != nil {
			p.persistentConn.Release()
			p.persistentConn = nil
		}
		p.mu.Unlock()
		return fmt.Errorf("error waiting for notification on channel %s: %w", p.channelName, err)
	}
	return nil
}

// Close unlisten from the channel and release the persistent connection
func (p *PgListener) Close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.persistentConn != nil {
		_, err := p.persistentConn.Exec(ctx, "UNLISTEN "+p.channelName)
		if err != nil {
			return fmt.Errorf("error unlistening on channel %s: %w", p.channelName, err)
		}
		p.persistentConn.Release()
		p.persistentConn = nil
	}
	return nil
}
