package queue

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/rs/zerolog"
)

const (
	sqlNotify   = `NOTIFY tasks`
	sqlListen   = `LISTEN tasks`
	sqlUnlisten = `UNLISTEN tasks`

	sqlEnqueue = `INSERT INTO tasks(id, type, payload, queued_at, org_id, repository_uuid) VALUES ($1, $2, $3, statement_timestamp(), $4, $5)`
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
		RETURNING id, type, payload, queued_at`

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
		SET started_at = NULL, token = NULL
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
		SELECT type, queued_at, started_at, finished_at, status, error
		FROM tasks
		WHERE id = $1`
	sqlQueryRunningId = `
                SELECT id
                FROM tasks
                WHERE token = $1 AND finished_at IS NULL AND status != 'canceled'`
	sqlFinishTask = `
		UPDATE tasks
		SET finished_at = now(), status = $1, error = $2
		WHERE id = $3 AND finished_at IS NULL
		RETURNING finished_at`
	sqlCancelTask = `
		UPDATE tasks
		SET status = 'canceled'
		WHERE id = $1 AND finished_at IS NULL
		RETURNING type, started_at`

	sqlInsertHeartbeat = `
                INSERT INTO task_heartbeats(token, id, heartbeat)
                VALUES ($1, $2, now())`
	sqlQueryHeartbeats = `
                SELECT token
                FROM task_heartbeats
                WHERE age(now(), heartbeat) > $1`
	sqlRefreshHeartbeat = `
                UPDATE task_heartbeats
                SET heartbeat = now()
                WHERE token = $1`
	sqlDeleteHeartbeat = `
                DELETE FROM task_heartbeats
                WHERE id = $1`
)

type PgQueue struct {
	logger       *zerolog.Logger
	pool         *pgxpool.Pool
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

func NewPgQueue(url string, logger *zerolog.Logger) (PgQueue, error) {
	pool, err := pgxpool.Connect(context.Background(), url)
	if err != nil {
		return PgQueue{}, fmt.Errorf("error establishing connection: %v", err)
	}

	listenContext, cancel := context.WithCancel(context.Background())
	q := PgQueue{
		logger:       logger,
		pool:         pool,
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
	conn, err := q.pool.Acquire(ctx)
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
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("error connecting to database: %v", err)
	}
	defer conn.Release()

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
	_, err = tx.Exec(context.Background(), sqlEnqueue, taskId, task.Typename, task.Payload, task.OrgId, task.RepositoryUUID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error enqueuing task: %v", err)
	}

	for _, d := range task.Dependencies {
		_, err = tx.Exec(context.Background(), sqlInsertDependency, taskId, d)
		if err != nil {
			return uuid.Nil, fmt.Errorf("error inserting dependency: %v", err)
		}
	}

	_, err = tx.Exec(context.Background(), sqlNotify)
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

	var taskId uuid.UUID
	var taskType string
	var args json.RawMessage
	var queued time.Time
	token := uuid.New()
	for {
		var err error
		taskId, taskType, args, queued, err = p.dequeueMaybe(ctx, token, taskTypes)
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

	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}
	defer conn.Release()

	// insert heartbeat
	_, err = conn.Exec(ctx, sqlInsertHeartbeat, token, taskId)
	if err != nil {
		return nil, fmt.Errorf("error inserting the task's heartbeat: %v", err)
	}

	dependencies, err := p.taskDependencies(ctx, conn, taskId)
	if err != nil {
		return nil, fmt.Errorf("error querying the task's dependencies: %v", err)
	}

	p.logger.Info().Msg(fmt.Sprintf("[Dequeued Task] Task Type: %v | Task ID: %v", taskType, taskId.String()))
	t := TaskInfo{
		Id:           taskId,
		Token:        token,
		Dependencies: dependencies,
		Typename:     taskType,
		Payload:      args,
		Queued:       queued,
	}
	return &t, nil
}

// dequeueMaybe is just a smaller helper for acquiring a connection and
// running the sqlDequeue query
func (p *PgQueue) dequeueMaybe(ctx context.Context, token uuid.UUID, taskTypes []string) (id uuid.UUID, taskType string, payload json.RawMessage, queued time.Time, err error) {
	var conn *pgxpool.Conn
	conn, err = p.pool.Acquire(ctx)
	if err != nil {
		return
	}
	defer conn.Release()

	err = conn.QueryRow(ctx, sqlDequeue, token, taskTypes).Scan(&id, &taskType, &payload, &queued)
	return
}

func (p *PgQueue) taskDependencies(ctx context.Context, conn *pgxpool.Conn, id uuid.UUID) ([]uuid.UUID, error) {
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
func (p *PgQueue) taskDependents(ctx context.Context, conn *pgxpool.Conn, id uuid.UUID) ([]uuid.UUID, error) {
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
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	// Use double pointers for timestamps because they might be NULL, which would result in *time.Time == nil
	var taskType, status, errMsg string
	var queued, started, finished *time.Time
	err = conn.QueryRow(context.Background(), sqlQueryTaskStatus, taskId).Scan(&taskType, &status, &queued, &started, &finished, &status, &errMsg)
	if err != nil {
		return nil, err
	}
	deps, err := p.taskDependencies(context.Background(), conn, taskId)
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	return &TaskInfo{
		Id:           taskId,
		Typename:     taskType,
		Dependencies: deps,
		Queued:       *queued,
		Started:      *started,
		Finished:     *finished,
		Error:        errMsg,
		Status:       status,
	}, nil
}

func (p *PgQueue) Finish(taskId uuid.UUID, taskError error) error {
	var status string
	if taskError != nil {
		status = StatusFailed
	} else {
		status = StatusCompleted
	}

	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}
	defer conn.Release()

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

	// Use double pointers for timestamps because they might be NULL, which would result in *time.Time == nil
	var taskType, statusQuery string
	var started, finished *time.Time
	err = tx.QueryRow(context.Background(), sqlQueryTaskStatus, taskId).Scan(&taskType, nil, &started, &finished, &statusQuery, nil)
	if err == pgx.ErrNoRows {
		return ErrNotExist
	}
	if status == StatusCanceled {
		return ErrCanceled
	}
	if started == nil || finished != nil {
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

	err = tx.QueryRow(context.Background(), sqlFinishTask, status, taskError, taskId).Scan(&finished)
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

	p.logger.Info().Msg(fmt.Sprintf("[Finished Task] Task Type: %v | Task ID: %v", taskType, taskId.String()))
	return nil
}

func (p *PgQueue) Cancel(taskId uuid.UUID) error {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}
	defer conn.Release()

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
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return fmt.Errorf("error connecting to database: %v", err)
	}
	defer conn.Release()

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
	var taskType, statusQuery string
	var started, finished *time.Time
	err = tx.QueryRow(context.Background(), sqlQueryTaskStatus, taskId).Scan(&taskType, nil, &started, &finished, &statusQuery, nil)
	if err == pgx.ErrNoRows {
		return ErrNotExist
	}
	if started == nil || finished != nil {
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

	p.logger.Info().Msg(fmt.Sprintf("[Requeuing Task] Task Type: %v | Task ID: %v", taskType, taskId.String()))
	return nil
}

func (p *PgQueue) Heartbeats(olderThan time.Duration) []uuid.UUID {
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return nil
	}
	defer conn.Release()

	rows, err := conn.Query(context.Background(), sqlQueryHeartbeats, olderThan.String())
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
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return
	}
	defer conn.Release()

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
	conn, err := p.pool.Acquire(context.Background())
	if err != nil {
		return uuid.Nil, fmt.Errorf("error establishing connection: %v", err)
	}
	defer conn.Release()

	err = conn.QueryRow(context.Background(), sqlQueryRunningId, token).Scan(&id)
	if err == pgx.ErrNoRows {
		return uuid.Nil, ErrNotExist
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("Error retrieving id: %v", err)
	}
	return
}
