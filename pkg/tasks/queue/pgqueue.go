package queue

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zerologadapter"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const taskInfoReturning = ` id, type, payload, queued_at, started_at, finished_at, status, error, org_id, repository_uuid, token, request_id ` // fields to return when returning taskInfo

const (
	sqlNotify   = `NOTIFY tasks`
	sqlListen   = `LISTEN tasks`
	sqlUnlisten = `UNLISTEN tasks`

	sqlEnqueue = `INSERT INTO tasks(id, type, payload, queued_at, org_id, repository_uuid, status, request_id) VALUES ($1, $2, $3, statement_timestamp(), $4, $5, $6, $7)`
	sqlDequeue = `
		UPDATE tasks
		SET token = $1, started_at = statement_timestamp(), status = 'running'
		WHERE id = (
		  SELECT id
		  FROM ready_tasks
			  -- use ANY here, because "type in ()" doesn't work with bound parameters
			  -- literal syntax for this is '{"a", "b"}': https://www.postgresql.org/docs/13/arrays.html
		  WHERE type = ANY($2)
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + taskInfoReturning

	//nolint:unused,deadcode,varcheck
	sqlDequeueByID = `
		UPDATE tasks
		SET token = $1, started_at = statement_timestamp()
		WHERE id = (
		  SELECT id
		  FROM ready_tasks
		  WHERE id = $2
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED
		)
		RETURNING token, type, payload, queued_at, started_at`

	sqlRequeue = `
		UPDATE tasks
		SET started_at = NULL, token = NULL, status = 'pending'
		WHERE id = $1 AND started_at IS NOT NULL AND finished_at IS NULL`

	sqlInsertDependency  = `INSERT INTO task_dependencies VALUES ($1, $2)`
	sqlQueryDependencies = `
		SELECT dependency_id
		FROM task_dependencies
		WHERE task_id = $1`
	sqlQueryDependents = `
		SELECT task_id
		FROM task_dependencies
		WHERE dependency_id = $1`
	//nolint:unused,deadcode,varcheck
	sqlQueryTask = `
		SELECT type, payload, repository_uuid, org_id, queued_at, started_at, finished_at, status, error
		FROM tasks
		WHERE id = $1`
	sqlQueryTaskStatus = `
		SELECT ` + taskInfoReturning +
		` FROM tasks
		WHERE id = $1`
	sqlQueryIdFromToken = `
                SELECT id, status FROM tasks WHERE token = $1`
	sqlFinishTask = `
		UPDATE tasks
		SET finished_at = statement_timestamp(), status = $1, error = $2
		WHERE id = $3 AND finished_at IS NULL
		RETURNING finished_at`
	sqlCancelTask = `
		UPDATE tasks
		SET status = 'canceled'
		WHERE id = $1 AND finished_at IS NULL
		RETURNING type, started_at`
	sqlUpdatePayload = `
		UPDATE tasks
		SET payload = $1
		WHERE id = $2`

	sqlInsertHeartbeat = `
                INSERT INTO task_heartbeats(token, id, heartbeat)
                VALUES ($1, $2, statement_timestamp())`
	sqlQueryHeartbeats = `
                SELECT token
                FROM task_heartbeats
				WHERE age(statement_timestamp(), heartbeat) > $1`
	sqlRefreshHeartbeat = `
                UPDATE task_heartbeats
                SET heartbeat = statement_timestamp()
                WHERE token = $1`
	sqlDeleteHeartbeat = `
                DELETE FROM task_heartbeats
                WHERE id = $1`
	sqlDeleteAllTasks = `
                TRUNCATE tasks, task_heartbeats, task_dependencies`
)

// These interfaces represent all the interactions with pgxpool that are needed for the pgqueue
//  They do not implement the exact interface as their pgx(pool) equivalents, as we need to
//  return instances of our own interfaces sometimes (For example, Pool.Acquire should return
//  our Transaction interface instead of pgxpool.Trans)

// Pool  matches the pgxpool.Pool struct
type Pool interface {
	Transaction
	Acquire(ctx context.Context) (Connection, error)
}

// Transaction mimics the pgx.Tx struct
type Transaction interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

type Connection interface {
	Transaction
	Conn() *pgx.Conn
	Release()
}

// PgQueue a task queue backed by postgres, using pgxpool.Pool using a wrapper (PgxPoolWrapper) that implements a Pool interface
type PgQueue struct {
	Pool         Pool
	dequeuers    *dequeuers
	stopListener func()
}

// thread-safe list of dequeuers
type dequeuers struct {
	list  *list.List
	mutex sync.Mutex
}

func newDequeuers() *dequeuers {
	return &dequeuers{
		list: list.New(),
	}
}

func (d *dequeuers) pushBack(c chan struct{}) *list.Element {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	return d.list.PushBack(c)
}

func (d *dequeuers) remove(e *list.Element) {
	d.mutex.Lock()
	defer d.mutex.Unlock()

	d.list.Remove(e)
}

func (d *dequeuers) notifyAll() {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	cur := d.list.Front()
	for cur != nil {
		listenerChan, _ := cur.Value.(chan struct{})

		// notify in a non-blocking way
		select {
		case listenerChan <- struct{}{}:
		default:
		}
		cur = cur.Next()
	}
}

func NewPgxPool(url string) (*pgxpool.Pool, error) {
	pxConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if config.Get().Tasking.PGXLogging {
		pxConfig.ConnConfig.Logger = zerologadapter.NewLogger(log.Logger)
		pxConfig.ConnConfig.LogLevel, err = pgx.LogLevelFromString(config.Get().Logging.Level)
		if err != nil {
			log.Error().Err(err).Msg("Error setting Pgx log level")
		}
	}
	pool, err := pgxpool.ConnectConfig(context.Background(), pxConfig)
	if err != nil {
		return nil, fmt.Errorf("error establishing connection: %w", err)
	} else {
		return pool, nil
	}
}

func NewPgQueue(url string) (PgQueue, error) {
	var poolWrapper Pool
	pool, err := NewPgxPool(url)
	if err != nil {
		return PgQueue{}, fmt.Errorf("error establishing connection: %w", err)
	}
	listenContext, cancel := context.WithCancel(context.Background())
	poolWrapper = &PgxPoolWrapper{pool: pool}
	q := PgQueue{
		Pool:         poolWrapper,
		dequeuers:    newDequeuers(),
		stopListener: cancel,
	}

	listenerReady := make(chan struct{})
	go q.listen(listenContext, listenerReady)

	// wait for the listener to become ready
	<-listenerReady

	return q, nil
}

func (q *PgQueue) listen(ctx context.Context, ready chan<- struct{}) {
	ready <- struct{}{}

	for {
		err := q.waitAndNotify(ctx)
		if err != nil {
			// shutdown the listener if the context is canceled
			if errors.Is(err, context.Canceled) {
				log.Logger.Info().Msg("Shutting down the listener")
				return
			}

			// otherwise, just log the error and continue, there might just
			// be a temporary networking issue
			log.Logger.Error().Err(err).Msg("Error waiting for notification on tasks channel")

			// backoff to avoid log spam
			time.Sleep(time.Millisecond * 500)
		}
	}
}

func (q *PgQueue) waitAndNotify(ctx context.Context) error {
	conn, err := q.Pool.Acquire(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		panic(fmt.Errorf("error connecting to database: %v", err))
	}
	defer func() {
		// use the empty context as the listening context is already cancelled at this point
		_, err := conn.Exec(context.Background(), sqlUnlisten)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			log.Logger.Error().Err(err).Msg("Error unlistening for tasks in dequeue")
		}
		conn.Release()
	}()

	_, err = conn.Exec(ctx, sqlListen)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		panic(fmt.Errorf("error listening on tasks channel: %v", err))
	}

	_, err = conn.Conn().WaitForNotification(ctx)
	if err != nil {
		return err
	}

	// something happened in the database, notify all dequeuers
	q.dequeuers.notifyAll()
	return nil
}

func (p *PgQueue) Enqueue(task *Task) (uuid.UUID, error) {
	taskID := uuid.New()
	conn, err := p.Pool.Acquire(context.Background())
	if err != nil {
		return uuid.Nil, err
	}
	defer conn.Release()

	tx, err := conn.Begin(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting database transaction: %w", err)
	}
	defer func() {
		errRollback := tx.Rollback(context.Background())
		if errRollback != nil && !errors.Is(errRollback, pgx.ErrTxClosed) {
			err = fmt.Errorf("error rolling back enqueue transaction: %w: %v", errRollback, err)
		}
	}()

	_, err = tx.Exec(context.Background(), sqlEnqueue,
		taskID.String(), task.Typename, task.Payload, task.OrgId, task.RepositoryUUID, config.TaskStatusPending, task.RequestID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error enqueuing task: %w", err)
	}

	for _, d := range task.Dependencies {
		_, err = tx.Exec(context.Background(), sqlInsertDependency, taskID, d)
		if err != nil {
			return uuid.Nil, fmt.Errorf("error inserting dependency: %w", err)
		}
	}

	_, err = conn.Exec(context.Background(), sqlNotify)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error notifying tasks channel: %w", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to commit database transaction: %w", err)
	}

	return taskID, nil
}

func (p *PgQueue) Dequeue(ctx context.Context, taskTypes []string) (*models.TaskInfo, error) {
	// add ourselves as a dequeuer
	c := make(chan struct{}, 1)
	el := p.dequeuers.pushBack(c)
	defer p.dequeuers.remove(el)

	var info *models.TaskInfo
	var err error
	token := uuid.New()
	for {
		info, err = p.dequeueMaybe(ctx, token, taskTypes)
		if err == nil {
			break
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, ErrContextCanceled
			}
			return nil, fmt.Errorf("error dequeuing task: %v", err)
		}
		// no suitable task was found, wait for the next queue update
		select {
		case <-c:
		case <-ctx.Done():
			return nil, ErrContextCanceled
		}
	}

	return info, nil
}

func (p *PgQueue) UpdatePayload(task *models.TaskInfo, payload interface{}) (*models.TaskInfo, error) {
	var err error
	_, err = p.Pool.Exec(context.Background(), sqlUpdatePayload, payload, task.Id.String())
	return task, err
}

// dequeueMaybe is just a smaller helper for acquiring a connection and
// running the sqlDequeue query
func (p *PgQueue) dequeueMaybe(ctx context.Context, token uuid.UUID, taskTypes []string) (info *models.TaskInfo, err error) {
	info = &models.TaskInfo{}

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting a new transaction when dequeueing: %w", err)
	}

	defer func() {
		errRollback := tx.Rollback(context.Background())
		if errRollback != nil && !errors.Is(errRollback, pgx.ErrTxClosed) {
			err = fmt.Errorf("error rolling back dequeue transaction: %w: %v", errRollback, err)
		}
	}()

	err = tx.QueryRow(ctx, sqlDequeue, token, taskTypes).Scan(
		&info.Id, &info.Typename, &info.Payload, &info.Queued, &info.Started, &info.Finished, &info.Status,
		&info.Error, &info.OrgId, &info.RepositoryUUID, &info.Token, &info.RequestID,
	)
	if err != nil {
		return nil, fmt.Errorf("error during dequeue query: %w", err)
	}

	// insert heartbeat
	_, err = tx.Exec(ctx, sqlInsertHeartbeat, token, info.Id)
	if err != nil {
		return nil, fmt.Errorf("error inserting the task's heartbeat: %w", err)
	}

	dependencies, err := p.taskDependencies(ctx, tx, info.Id)
	if err != nil {
		return nil, fmt.Errorf("error querying the task's dependencies: %w", err)
	}
	info.Dependencies = dependencies

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("error committing the transaction for dequeueing task %s: %w", info.Id.String(), err)
	}

	return info, nil
}

func (p *PgQueue) taskDependencies(ctx context.Context, tx Transaction, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, sqlQueryDependencies, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dependencies := []uuid.UUID{}
	for rows.Next() {
		var d uuid.UUID
		err = rows.Scan(&d)
		if err != nil {
			return nil, err
		}

		dependencies = append(dependencies, d)
	}
	if rows.Err() != nil {
		return nil, err
	}

	return dependencies, nil
}

//nolint:unused
func (p *PgQueue) taskDependents(ctx context.Context, conn Connection, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := conn.Query(ctx, sqlQueryDependents, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dependents := []uuid.UUID{}
	for rows.Next() {
		var d uuid.UUID
		err = rows.Scan(&d)
		if err != nil {
			return nil, err
		}

		dependents = append(dependents, d)
	}
	if rows.Err() != nil {
		return nil, err
	}

	return dependents, nil
}

func (p *PgQueue) Status(taskId uuid.UUID) (*models.TaskInfo, error) {
	var err error

	// Use double pointers for timestamps because they might be NULL, which would result in *time.Time == nil
	var info models.TaskInfo
	conn, err := p.Pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	err = conn.QueryRow(context.Background(), sqlQueryTaskStatus, taskId).Scan(
		&info.Id, &info.Typename, &info.Payload, &info.Queued, &info.Started, &info.Finished, &info.Status,
		&info.Error, &info.OrgId, &info.RepositoryUUID, &info.Token, &info.RequestID,
	)
	if err != nil {
		return nil, err
	}
	deps, err := p.taskDependencies(context.Background(), conn, taskId)
	if err != nil {
		return nil, err
	}
	info.Dependencies = deps

	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (p *PgQueue) Finish(taskId uuid.UUID, taskError error) error {
	var err error

	var status string
	var errMsg *string
	if taskError != nil {
		if errors.Is(taskError, context.Canceled) {
			status = config.TaskStatusCanceled
		} else {
			status = config.TaskStatusFailed
		}
		s := taskError.Error()
		errMsg = &s
	} else {
		status = config.TaskStatusCompleted
		errMsg = nil
	}

	tx, err := p.Pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.Logger.Error().Err(err).Msg(fmt.Sprintf("Error rolling back finish task transaction for task %v", taskId.String()))
		}
	}()

	info, err := p.Status(taskId)
	if err != nil {
		return err
	}
	if info.Started == nil || info.Finished != nil {
		return ErrNotRunning
	}

	// Remove from heartbeats
	tag, err := tx.Exec(context.Background(), sqlDeleteHeartbeat, taskId)
	if err != nil {
		return fmt.Errorf("error removing task %s from heartbeats: %v", taskId, err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotExist
	}

	err = tx.QueryRow(context.Background(), sqlFinishTask, status, errMsg, taskId).Scan(&info.Finished)
	if err == pgx.ErrNoRows {
		return ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("error finishing task %s: %v", taskId, err)
	}

	_, err = tx.Exec(context.Background(), sqlNotify)
	if err != nil {
		return fmt.Errorf("error notifying tasks channel: %v", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("unable to commit database transaction: %v", err)
	}

	return nil
}

func (p *PgQueue) SendCancelNotification(ctx context.Context, taskId uuid.UUID) error {
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	channelName := getCancelChannelName(taskId)
	_, err = conn.Exec(ctx, "select pg_notify($1, 'cancel')", channelName)
	if err != nil {
		return err
	}

	return nil
}

func (p *PgQueue) Requeue(taskId uuid.UUID) error {
	var err error

	tx, err := p.Pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.Logger.Error().Err(err).Msg(fmt.Sprintf("Error rolling back retry task transaction. Task id %v", taskId.String()))
		}
	}()

	// Use double pointers for timestamps because they might be NULL, which would result in *time.Time == nil
	info, err := p.Status(taskId)
	if err == pgx.ErrNoRows {
		return ErrNotExist
	}
	if info.Started == nil || info.Finished != nil {
		return ErrNotRunning
	}

	// Remove from heartbeats
	tag, err := tx.Exec(context.Background(), sqlDeleteHeartbeat, taskId)
	if err != nil {
		return fmt.Errorf("error removing task %s from heartbeats: %v", taskId, err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotExist
	}

	tag, err = tx.Exec(context.Background(), sqlRequeue, taskId)
	if err != nil {
		return fmt.Errorf("error requeueing task %s: %v", taskId, err)
	}

	if tag.RowsAffected() != 1 {
		return ErrNotExist
	}

	_, err = tx.Exec(context.Background(), sqlNotify)
	if err != nil {
		return fmt.Errorf("error notifying tasks channel: %v", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("unable to commit database transaction: %v", err)
	}

	return nil
}

func (p *PgQueue) Heartbeats(olderThan time.Duration) []uuid.UUID {
	var err error

	rows, err := p.Pool.Query(context.Background(), sqlQueryHeartbeats, olderThan)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tokens []uuid.UUID
	for rows.Next() {
		var t uuid.UUID
		err = rows.Scan(&t)
		if err != nil {
			// Log the error and try to continue with the next row
			log.Logger.Error().Err(err).Msg("Unable to read token from heartbeats")
			continue
		}
		tokens = append(tokens, t)
	}
	if rows.Err() != nil {
		log.Logger.Error().Err(rows.Err()).Msg("Error reading tokens from heartbeats")
	}

	return tokens
}

// Reset the last heartbeat time to time.Now()
func (p *PgQueue) RefreshHeartbeat(token uuid.UUID) error {
	var err error

	if token == uuid.Nil {
		return nil
	}

	tag, err := p.Pool.Exec(context.Background(), sqlRefreshHeartbeat, token)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		_, isRunning, err := p.IdFromToken(token)
		if err != nil {
			return err
		}
		if isRunning {
			tag, err := p.Pool.Exec(context.Background(), sqlRefreshHeartbeat, token)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return ErrRowsNotAffected
			}
		}
	}
	return nil
}

func (p *PgQueue) IdFromToken(token uuid.UUID) (id uuid.UUID, isRunning bool, err error) {
	var status string
	conn, err := p.Pool.Acquire(context.Background())
	if err != nil {
		return uuid.Nil, false, err
	}
	defer conn.Release()
	row := conn.QueryRow(context.Background(), sqlQueryIdFromToken, token)
	err = row.Scan(&id, &status)
	isRunning = status == config.TaskStatusRunning
	if err == pgx.ErrNoRows {
		return uuid.Nil, isRunning, ErrNotExist
	} else if err != nil {
		return uuid.Nil, isRunning, fmt.Errorf("Error retrieving id: %v", err)
	}

	return
}

// RemoveAllTasks used for tests, along with testTx, to clear tables before running tests
func (p *PgQueue) RemoveAllTasks() error {
	_, err := p.Pool.Exec(context.Background(), sqlDeleteAllTasks)
	if err != nil {
		return fmt.Errorf("error removing all tasks")
	}
	return nil
}

func (p *PgQueue) ListenForCancel(ctx context.Context, taskID uuid.UUID, cancelFunc context.CancelCauseFunc) {
	logger := zerolog.Ctx(ctx)
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		// If the task is finished before listen is initiated, a context canceled error is expected
		if !errors.Is(ErrNotRunning, context.Cause(ctx)) {
			logger.Error().Err(err).Msg("ListenForCancel: error acquiring connection")
		}
		return
	}
	defer conn.Release()

	// Register a channel for the task where a notification can be sent to cancel the task
	channelName := getCancelChannelName(taskID)
	// TODO remove debug logs. checking if channel register is hanging
	logger.Debug().Msg("ListenForCancel: preparing register channel")
	_, err = conn.Conn().Exec(ctx, "listen "+channelName)
	if err != nil {
		if !errors.Is(ErrNotRunning, context.Cause(ctx)) {
			logger.Error().Err(err).Msg("ListenForCancel: error registering channel")
		}
		return
	}
	logger.Debug().Msg("ListenForCancel: finished register channel")

	// When the function returns, unregister the channel
	defer func(conn *pgx.Conn) {
		// TODO Go 1.21 can replace context.Background() with context.WithoutCancel()
		_, err = conn.Exec(context.Background(), "unlisten "+channelName)
		if err != nil {
			logger.Error().Err(err).Msg("ListenForCancel: error unregistering listener")
		}
	}(conn.Conn())

	// Wait for a notification on the channel. This blocks until the channel receives a notification.
	_, err = conn.Conn().WaitForNotification(ctx)
	if err != nil {
		if !errors.Is(ErrNotRunning, context.Cause(ctx)) {
			logger.Error().Err(err).Msg("ListenForCancel: error waiting for notification")
		}
		return
	}

	// Cancel context only if context has not already been canceled. If the context has already been canceled, the task has finished.
	if !errors.Is(ctx.Err(), context.Canceled) {
		logger.Debug().Msg("[Canceled Task]")
		cancelFunc(ErrTaskCanceled)
	}
}

func getCancelChannelName(taskID uuid.UUID) string {
	return strings.Replace("task_"+taskID.String(), "-", "", -1)
}
