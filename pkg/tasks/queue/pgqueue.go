package queue

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/log/zerologadapter"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const taskInfoReturning = ` id, type, payload, queued_at, started_at, finished_at, status, error, org_id, repository_uuid, token ` // fields to return when returning taskInfo

const (
	sqlNotify   = `NOTIFY tasks`
	sqlListen   = `LISTEN tasks`
	sqlUnlisten = `UNLISTEN tasks`

	sqlEnqueue = `INSERT INTO tasks(id, type, payload, queued_at, org_id, repository_uuid, status) VALUES ($1, $2, $3, statement_timestamp(), $4, $5, 'pending')`
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

	//nolint:unused
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
	sqlQueryTask = `
		SELECT type, payload, repository_uuid, org_id, queued_at, started_at, finished_at, status, error
		FROM tasks
		WHERE id = $1`
	sqlQueryTaskStatus = `
		SELECT ` + taskInfoReturning +
		` FROM tasks
		WHERE id = $1`
	sqlQueryRunningId = `
                SELECT id
                FROM tasks
                WHERE token = $1 AND finished_at IS NULL AND status != 'canceled'`
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

type PgQueue struct {
	Conn         Connection
	Pool         *pgxpool.Pool
	logger       *zerolog.Logger
	dequeuers    *dequeuers
	stopListener func()
}

type Connection interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
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

func NewPgQueue(url string, logger *zerolog.Logger) (PgQueue, error) {
	pxConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		return PgQueue{}, fmt.Errorf("error establishing connection: %w", err)
	}
	if config.Get().Tasking.PGXLogging {
		pxConfig.ConnConfig.Logger = zerologadapter.NewLogger(log.Logger)
	}
	pool, err := pgxpool.ConnectConfig(context.Background(), pxConfig)
	if err != nil {
		return PgQueue{}, fmt.Errorf("error establishing connection: %w", err)
	}

	listenContext, cancel := context.WithCancel(context.Background())
	q := PgQueue{
		logger:       logger,
		Pool:         pool,
		Conn:         pool,
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
				q.logger.Info().Msg("Shutting down the listener")
				return
			}

			// otherwise, just log the error and continue, there might just
			// be a temporary networking issue
			q.logger.Error().Err(err).Msg("Error waiting for notification on tasks channel")

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
			q.logger.Error().Err(err).Msg("Error unlistening for tasks in dequeue")
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
	var conn Connection
	var err error
	conn = p.Conn

	tx, err := conn.Begin(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error().Err(err).Msg("Error rolling back enqueue transaction")
		}
	}()

	taskId := uuid.New()
	_, err = conn.Exec(context.Background(), sqlEnqueue, taskId.String(), task.Typename, task.Payload, task.OrgId, task.RepositoryUUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error enqueuing task: %v", err)
	}

	for _, d := range task.Dependencies {
		_, err = tx.Exec(context.Background(), sqlInsertDependency, taskId, d)
		if err != nil {
			return uuid.Nil, fmt.Errorf("error inserting dependency: %v", err)
		}
	}

	_, err = conn.Exec(context.Background(), sqlNotify)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error notifying tasks channel: %v", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("unable to commit database transaction: %v", err)
	}

	p.logger.Info().Msg(fmt.Sprintf("[Enqueued Task] Task Type: %v | Task ID: %v", task.Typename, taskId.String()))
	return taskId, nil
}

func (p *PgQueue) Dequeue(ctx context.Context, taskTypes []string) (*TaskInfo, error) {
	// add ourselves as a dequeuer
	c := make(chan struct{}, 1)
	el := p.dequeuers.pushBack(c)
	defer p.dequeuers.remove(el)

	var info *TaskInfo
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

func (p *PgQueue) UpdatePayload(ctx context.Context, task *TaskInfo, payload interface{}) (*TaskInfo, error) {
	var conn Connection
	var err error
	conn = p.Conn
	_, err = conn.Exec(context.Background(), sqlUpdatePayload, payload, task.Id.String())
	return task, err
}

// dequeueMaybe is just a smaller helper for acquiring a connection and
// running the sqlDequeue query
func (p *PgQueue) dequeueMaybe(ctx context.Context, token uuid.UUID, taskTypes []string) (*TaskInfo, error) {
	var err error
	var info TaskInfo

	tx, err := p.Conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting a new transaction when dequeueing: %w", err)
	}

	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error().Err(err).Msg("Error rolling back dequeuing transaction")
		}
	}()

	err = tx.QueryRow(ctx, sqlDequeue, token, taskTypes).Scan(
		&info.Id, &info.Typename, &info.Payload, &info.Queued, &info.Started, &info.Finished, &info.Status,
		&info.Error, &info.OrgId, &info.RepositoryUUID, &info.Token,
	)

	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	// insert heartbeat
	_, err = tx.Exec(ctx, sqlInsertHeartbeat, token, info.Id)
	if err != nil {
		return nil, fmt.Errorf("error inserting the task's heartbeat: %v", err)
	}

	dependencies, err := p.taskDependencies(ctx, tx, info.Id)
	if err != nil {
		return nil, fmt.Errorf("error querying the task's dependencies: %v", err)
	}
	info.Dependencies = dependencies

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("error committing the transaction for dequeueing task %s: %w", info.Id.String(), err)
	}

	p.logger.Info().Msg(fmt.Sprintf("[Dequeued Task] Task Type: %v | Task ID: %v", info.Typename, info.Id.String()))
	return &info, nil
}

func (p *PgQueue) taskDependencies(ctx context.Context, conn Connection, id uuid.UUID) ([]uuid.UUID, error) {
	rows, err := conn.Query(ctx, sqlQueryDependencies, id)
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

func (p *PgQueue) Status(taskId uuid.UUID) (*TaskInfo, error) {
	var conn Connection
	var err error
	conn = p.Conn

	// Use double pointers for timestamps because they might be NULL, which would result in *time.Time == nil
	var info TaskInfo
	err = conn.QueryRow(context.Background(), sqlQueryTaskStatus, taskId).Scan(
		&info.Id, &info.Typename, &info.Payload, &info.Queued, &info.Started, &info.Finished, &info.Status,
		&info.Error, &info.OrgId, &info.RepositoryUUID, &info.Token,
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
	var conn Connection
	var err error
	conn = p.Conn

	var status string
	var errMsg *string
	if taskError != nil {
		status = StatusFailed
		s := taskError.Error()
		errMsg = &s
	} else {
		status = StatusCompleted
		errMsg = nil
	}

	tx, err := conn.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error().Err(err).Msg(fmt.Sprintf("Error rolling back finish task transaction for task %v", taskId.String()))
		}
	}()

	info, err := p.Status(taskId)
	if err != nil {
		return err
	}
	if info.Status == StatusCanceled {
		return ErrCanceled
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

	p.logger.Info().Msg(fmt.Sprintf("[Finished Task] Task Type: %v | Task ID: %v", info.Typename, taskId.String()))
	return nil
}

func (p *PgQueue) Cancel(taskId uuid.UUID) error {
	var conn Connection
	var err error
	conn = p.Conn

	var started *time.Time
	var taskType string
	err = conn.QueryRow(context.Background(), sqlCancelTask, taskId).Scan(&taskType, &started)
	if err == pgx.ErrNoRows {
		return ErrNotRunning
	}
	if err != nil {
		return fmt.Errorf("error canceling task %s: %v", taskId, err)
	}

	p.logger.Info().Msg(fmt.Sprintf("[Canceling Task] Task Type: %v | Task ID: %v", taskType, taskId.String()))

	return nil
}

func (p *PgQueue) Requeue(taskId uuid.UUID) error {
	var conn Connection
	var err error
	conn = p.Conn

	tx, err := conn.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			p.logger.Error().Err(err).Msg(fmt.Sprintf("Error rolling back retry task transaction. Task id %v", taskId.String()))
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

	p.logger.Info().Msg(fmt.Sprintf("[Requeuing Task] Task Type: %v | Task ID: %v", info.Typename, taskId.String()))
	return nil
}

func (p *PgQueue) Heartbeats(olderThan time.Duration) []uuid.UUID {
	var conn Connection
	var err error
	conn = p.Conn

	rows, err := conn.Query(context.Background(), sqlQueryHeartbeats, olderThan)
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
			p.logger.Error().Err(err).Msg("Unable to read token from heartbeats")
			continue
		}
		tokens = append(tokens, t)
	}
	if rows.Err() != nil {
		p.logger.Error().Err(rows.Err()).Msg("Error reading tokens from heartbeats")
	}

	return tokens
}

// Reset the last heartbeat time to time.Now()
func (p *PgQueue) RefreshHeartbeat(token uuid.UUID) {
	var conn Connection
	var err error
	conn = p.Conn

	if token == uuid.Nil {
		return
	}

	tag, err := conn.Exec(context.Background(), sqlRefreshHeartbeat, token)
	if err != nil {
		p.logger.Error().Err(err).Msg("Error refreshing heartbeat")
	}
	if tag.RowsAffected() != 1 {
		p.logger.Error().Err(nil).Msg("No rows affected when refreshing heartbeat")
	}
}

func (p *PgQueue) IdFromToken(token uuid.UUID) (id uuid.UUID, err error) {
	conn := p.Conn

	err = conn.QueryRow(context.Background(), sqlQueryRunningId, token).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, ErrNotExist
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("Error retrieving id: %v", err)
	}
	return
}

// RemoveAllTasks used for tests, along with testTx, to clear tables before running tests
func (p *PgQueue) RemoveAllTasks() error {
	_, err := p.Conn.Exec(context.Background(), sqlDeleteAllTasks)
	if err != nil {
		return fmt.Errorf("error removing all tasks")
	}
	return nil
}
