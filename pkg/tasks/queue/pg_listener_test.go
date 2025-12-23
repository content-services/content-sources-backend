package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PgListenerSuite struct {
	suite.Suite
	pool    Pool
	pgxPool *pgxpool.Pool
	wg      sync.WaitGroup
}

func (s *PgListenerSuite) SetupTest() {
	pgxPool, err := NewPgxPool(context.Background(), db.GetUrl())
	require.NoError(s.T(), err)

	// Use real pool wrapper for LISTEN/NOTIFY functionality
	s.pool = &PgxPoolWrapper{pool: pgxPool}
	s.pgxPool = pgxPool
}

func (s *PgListenerSuite) TearDownTest() {
	// Wait for all notification goroutines to complete
	s.wg.Wait()

	if s.pgxPool != nil {
		s.pgxPool.Close()
	}
}

func TestPgListenerSuite(t *testing.T) {
	suite.Run(t, new(PgListenerSuite))
}

// sendNotificationAfter sends a notification after the listener has started waiting
func (s *PgListenerSuite) sendNotificationAfter(channel string, delay time.Duration) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		time.Sleep(delay)

		// Acquire a dedicated connection for this notification to avoid races
		conn, err := s.pgxPool.Acquire(context.Background())
		if err != nil {
			s.T().Log("Error acquiring connection for notification:", err)
			return
		}
		defer conn.Release()

		_, err = conn.Conn().Exec(context.Background(), "NOTIFY "+channel)
		if err != nil {
			s.T().Log("Error sending notification:", err)
		}
	}()
}

func (s *PgListenerSuite) TestNewPgListener() {
	listener := NewPgListener(s.pool, "test_channel")

	assert.NotNil(s.T(), listener)
	assert.Equal(s.T(), "test_channel", listener.channelName)
	assert.Equal(s.T(), s.pool, listener.pool)
	assert.Nil(s.T(), listener.persistentConn)
}

func (s *PgListenerSuite) TestWaitForNotification_Success() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener := NewPgListener(s.pool, "test_channel")

	// Send a notification after a small delay
	s.sendNotificationAfter("test_channel", 50*time.Millisecond)

	// Wait for notification
	err := listener.WaitForNotification(ctx)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), listener.persistentConn, "connection should be established and persisted")

	// Clean up
	listener.Close(context.Background())
}

func (s *PgListenerSuite) TestWaitForNotification_PersistentConnection() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener := NewPgListener(s.pool, "test_channel")
	defer listener.Close(context.Background())

	// First call - establishes connection
	s.sendNotificationAfter("test_channel", 50*time.Millisecond)
	err := listener.WaitForNotification(ctx)
	assert.NoError(s.T(), err)
	firstConn := listener.persistentConn

	// Second call - should reuse connection
	s.sendNotificationAfter("test_channel", 50*time.Millisecond)
	err = listener.WaitForNotification(ctx)
	assert.NoError(s.T(), err)
	secondConn := listener.persistentConn

	assert.Equal(s.T(), firstConn, secondConn, "should reuse the same connection")
}

func (s *PgListenerSuite) TestWaitForNotification_ContextCanceled() {
	ctx, cancel := context.WithCancel(context.Background())
	listener := NewPgListener(s.pool, "test_channel")

	// Cancel context after a delay (before notification)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := listener.WaitForNotification(ctx)
	assert.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, context.Canceled),
		"error should be context canceled")

	// Connection should be cleaned up after context cancellation
	assert.Nil(s.T(), listener.persistentConn, "connection should be cleaned up after error")
}

func (s *PgListenerSuite) TestClose_Success() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener := NewPgListener(s.pool, "test_channel")

	// Establish a connection first
	s.sendNotificationAfter("test_channel", 50*time.Millisecond)
	err := listener.WaitForNotification(ctx)
	require.NoError(s.T(), err)
	assert.NotNil(s.T(), listener.persistentConn, "connection should exist before Close")

	// Close the listener
	err = listener.Close(context.Background())
	assert.NoError(s.T(), err)
	assert.Nil(s.T(), listener.persistentConn, "connection should be nil after Close")
}

func (s *PgListenerSuite) TestClose_NoConnection() {
	listener := NewPgListener(s.pool, "test_channel")

	// Close without establishing a connection first
	err := listener.Close(context.Background())
	assert.NoError(s.T(), err, "Close should be idempotent when no connection exists")
}

func (s *PgListenerSuite) TestWaitForNotification_Timeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	listener := NewPgListener(s.pool, "test_channel")

	// Don't send any notification - let it timeout
	err := listener.WaitForNotification(ctx)
	assert.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, context.DeadlineExceeded),
		"error should be deadline exceeded")

	// Connection should be cleaned up
	assert.Nil(s.T(), listener.persistentConn, "connection should be cleaned up after timeout")
}

func (s *PgListenerSuite) TestWaitForNotification_MultipleNotifications() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listener := NewPgListener(s.pool, "test_channel")
	defer listener.Close(context.Background())

	// Receive multiple notifications in sequence
	for i := 0; i < 3; i++ {
		s.sendNotificationAfter("test_channel", 50*time.Millisecond)
		err := listener.WaitForNotification(ctx)
		assert.NoError(s.T(), err, "should receive notification %d", i+1)
	}

	assert.NotNil(s.T(), listener.persistentConn, "connection should still be established")
}
