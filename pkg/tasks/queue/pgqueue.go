package queue

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/google/uuid"
	pgxzero "github.com/jackc/pgx-zerolog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const taskInfoReturning = ` id, type, payload, queued_at, started_at, finished_at, status, error, org_id, object_uuid, object_type, token, request_id, retries, next_retry_time, priority, cancel_attempted ` // fields to return when returning taskInfo

const (
	sqlNotify   = `NOTIFY tasks`
	sqlListen   = `LISTEN tasks`
	sqlUnlisten = `UNLISTEN tasks`

	sqlEnqueue = `INSERT INTO tasks(id, type, payload, queued_at, org_id, object_uuid, object_type, status, request_id, account_id, priority) VALUES ($1, $2, $3, clock_timestamp(), $4, $5, $6, $7, $8, $9, $10)`
	sqlDequeue = `
		UPDATE tasks
		SET token = $1, started_at = clock_timestamp(), status = 'running'
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

	sqlRequeue = `
		UPDATE tasks
		SET started_at = NULL, token = NULL, status = 'pending', retries = retries + 1, queued_at = clock_timestamp()
		WHERE id = $1 AND started_at IS NOT NULL AND finished_at IS NULL`

	sqlRequeueFailedTasks = `
		WITH v1 AS (
    		SELECT * FROM tasks t LEFT JOIN task_dependencies td ON (t.id = td.dependency_id)
    		WHERE (started_at IS NOT NULL AND finished_at IS NOT NULL AND status = 'failed' AND retries < 3 AND next_retry_time <= clock_timestamp() AND type = ANY($1::text[]) AND cancel_attempted = false)
		)
		UPDATE tasks SET started_at = NULL, finished_at = NULL, token = NULL, status = 'pending', retries = retries + 1, queued_at = clock_timestamp()
		FROM ( 
			SELECT tasks.id
      		FROM tasks, v1
      			WHERE v1.task_id = tasks.id
         		OR (tasks.started_at IS NOT NULL AND tasks.finished_at IS NOT NULL AND tasks.status = 'failed' AND tasks.retries < 3 AND tasks.next_retry_time <= clock_timestamp() AND tasks.type = ANY($1::text[]) AND tasks.cancel_attempted = false)
     	) t1
		WHERE tasks.id = t1.id`

	sqlInsertDependency  = `INSERT INTO task_dependencies VALUES ($1, $2)`
	sqlQueryDependencies = `
		SELECT ARRAY (
			SELECT dependency_id
			FROM task_dependencies
			WHERE task_id = $1 
		)`
	sqlQueryDependents = `
		SELECT ARRAY (
			SELECT task_id
			FROM task_dependencies
			WHERE dependency_id = $1 
		)`
	//nolint:unused,deadcode,varcheck
	sqlQueryTask = `
		SELECT type, payload, object_uuid, object_type, org_id, queued_at, started_at, finished_at, status, error
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
		SET finished_at = clock_timestamp(), status = $1, error = (left($2, 4000)), next_retry_time = $3
		WHERE id = $4 AND finished_at is NULL
		RETURNING finished_at`
	sqlCancelTask = `
		UPDATE tasks
		SET status = 'canceled', error = (left($2, 4000)), cancel_attempted = true
		WHERE id = $1 AND finished_at IS NULL`
	// sqlUpdatePayload
	sqlUpdatePayload = `
		UPDATE tasks
		SET payload = $1
		WHERE id = $2`

	sqlInsertHeartbeat = `
                INSERT INTO task_heartbeats(token, id, heartbeat)
                VALUES ($1, $2, clock_timestamp())`
	sqlQueryHeartbeats = `
                SELECT token
                FROM task_heartbeats
				WHERE age(clock_timestamp(), heartbeat) > $1`
	sqlRefreshHeartbeat = `
                UPDATE task_heartbeats
                SET heartbeat = clock_timestamp()
                WHERE token = $1`
	sqlDeleteHeartbeat = `
                DELETE FROM task_heartbeats
                WHERE id = $1`
	sqlDeleteAllTasks = `
                TRUNCATE task_heartbeats, task_dependencies; DELETE FROM TASKS;`
)

// These interfaces represent all the interactions with pgxpool that are needed for the pgqueue
//  They do not implement the exact interface as their pgx(pool) equivalents, as we need to
//  return instances of our own interfaces sometimes (For example, Pool.Acquire should return
//  our Transaction interface instead of pgxpool.Trans)

// Pool  matches the pgxpool.Pool struct
type Pool interface {
	Transaction
	Acquire(ctx context.Context) (Connection, error)
	Close()
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
	Pool      Pool
	dequeuers *dequeuers
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

func NewPgxPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pxConfig, err := pgxpool.ParseConfig(url)
	poolLimit := config.Get().Database.PoolLimit
	if poolLimit < math.MinInt32 || poolLimit > math.MaxInt32 {
		return nil, errors.New("invalid pool limit size")
	}
	pxConfig.MaxConns = int32(poolLimit)
	if err != nil {
		return nil, err
	}
	if config.Get().Tasking.PGXLogging {
		level, err := tracelog.LogLevelFromString(config.Get().Logging.Level)
		if err != nil {
			log.Logger.Error().Err(err).Msg("could not set log level for pgx logging, defaulting to DEBUG")
			level = tracelog.LogLevelDebug
		}
		pxConfig.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   pgxzero.NewLogger(log.Logger),
			LogLevel: level,
		}

		if err != nil {
			log.Error().Err(err).Msg("Error setting Pgx log level")
		}
	}
	pool, err := pgxpool.NewWithConfig(ctx, pxConfig)
	if err != nil {
		return nil, fmt.Errorf("error establishing connection: %w", err)
	}

	return pool, nil
}

func NewPgQueue(ctx context.Context, url string) (PgQueue, error) {
	var poolWrapper Pool
	pool, err := NewPgxPool(ctx, url)
	if err != nil {
		return PgQueue{}, fmt.Errorf("error establishing connection: %w", err)
	}
	// listenContext, cancel := context.WithCancel(context.Background())
	poolWrapper = &PgxPoolWrapper{pool: pool}
	q := PgQueue{
		Pool:      poolWrapper,
		dequeuers: newDequeuers(),
	}

	listenerReady := make(chan struct{})
	go q.listen(ctx, listenerReady)

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
		taskID.String(), task.Typename, task.Payload, task.OrgId, task.ObjectUUID, task.ObjectType,
		config.TaskStatusPending, task.RequestID, task.AccountId, task.Priority)
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
		&info.Error, &info.OrgId, &info.ObjectUUID, &info.ObjectType, &info.Token, &info.RequestID,
		&info.Retries, &info.NextRetryTime, &info.Priority, &info.CancelAttempted,
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

func (p *PgQueue) UpdatePayload(task *models.TaskInfo, payload interface{}) (*models.TaskInfo, error) {
	var err error
	_, err = p.Pool.Exec(context.Background(), sqlUpdatePayload, payload, task.Id.String())
	return task, err
}

func (p *PgQueue) taskDependencies(ctx context.Context, tx Transaction, id uuid.UUID) ([]string, error) {
	var dependencies []string
	err := tx.QueryRow(ctx, sqlQueryDependencies, id).Scan(&dependencies)
	if err != nil {
		return nil, err
	}
	return dependencies, nil
}

//nolint:unused
func (p *PgQueue) taskDependents(ctx context.Context, tx Transaction, id uuid.UUID) ([]string, error) {
	var dependents []string
	err := tx.QueryRow(ctx, sqlQueryDependents, id).Scan(&dependents)
	if err != nil {
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
		&info.Error, &info.OrgId, &info.ObjectUUID, &info.ObjectType, &info.Token, &info.RequestID,
		&info.Retries, &info.NextRetryTime, &info.Priority, &info.CancelAttempted,
	)
	if err != nil {
		return nil, err
	}
	deps, err := p.taskDependencies(context.Background(), conn, taskId)
	if err != nil {
		return nil, err
	}
	info.Dependencies = deps

	return &info, nil
}

func (p *PgQueue) Finish(taskId uuid.UUID, taskError error) error {
	var err error

	var status string
	var errMsg *string
	if taskError != nil {
		status = config.TaskStatusFailed
		s := strings.ToValidUTF8(taskError.Error(), "")
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

	var nextRetryTime *time.Time
	if status == config.TaskStatusFailed && info.Retries < MaxTaskRetries {
		upperBound := config.Get().Tasking.RetryWaitUpperBound
		retriesRemaining := float64(MaxTaskRetries - info.Retries)
		timeToWait := time.Second * time.Duration(upperBound.Seconds()/(retriesRemaining+1))
		add := time.Now().Add(timeToWait)
		nextRetryTime = &add
	}

	// Remove from heartbeats
	tag, err := tx.Exec(context.Background(), sqlDeleteHeartbeat, taskId)
	if err != nil {
		return fmt.Errorf("error removing task %s from heartbeats: %v", taskId, err)
	}
	if tag.RowsAffected() != 1 && info.Status != config.TaskStatusCanceled {
		logger := log.Logger.With().Str("task_id", taskId.String()).Logger()
		logger.Warn().Msgf("error finishing task: error deleting heartbeat: heartbeat not found. was this task requeued recently?")
	}

	err = tx.QueryRow(context.Background(), sqlFinishTask, status, errMsg, nextRetryTime, taskId).Scan(&info.Finished)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("error finishing task: %w", ErrNotExist)
	}
	if err != nil {
		return fmt.Errorf("error finishing task %s: %w", taskId, err)
	}

	if status == config.TaskStatusFailed {
		dependents, err := p.taskDependents(context.Background(), tx.Conn(), taskId)
		if err != nil {
			return fmt.Errorf("error fetching task dependents: %w", err)
		}
		for _, id := range dependents {
			_, err := tx.Exec(context.Background(), sqlCancelTask, id, "parent task failed")
			if err != nil {
				return fmt.Errorf("error cancelling dependent task: %w", err)
			}
		}
	}

	_, err = tx.Exec(context.Background(), sqlNotify)
	if err != nil {
		return fmt.Errorf("error notifying tasks channel: %w", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("unable to commit database transaction: %w", err)
	}

	return nil
}

func (p *PgQueue) Cancel(ctx context.Context, taskId uuid.UUID) error {
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting database transaction: %w", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.Logger.Error().Err(err).Msg(fmt.Sprintf("Error rolling back cancel task transaction for task %v", taskId.String()))
		}
	}()

	err = p.sendCancelNotification(ctx, taskId)
	if err != nil {
		return err
	}

	// Remove from heartbeats
	tag, err := tx.Exec(ctx, sqlDeleteHeartbeat, taskId)
	if err != nil {
		return fmt.Errorf("error removing task %s from heartbeats: %v", taskId, err)
	}
	if tag.RowsAffected() != 1 {
		logger := log.Logger.With().Str("task_id", taskId.String()).Logger()
		logger.Warn().Msgf("error canceling task: error deleting heartbeat: heartbeat not found. was this task requeued recently?")
	}

	_, err = tx.Exec(ctx, sqlCancelTask, taskId, "task canceled")
	if err != nil {
		return fmt.Errorf("error canceling task: %w", err)
	}

	dependents, err := p.taskDependents(context.Background(), tx.Conn(), taskId)
	if err != nil {
		return fmt.Errorf("error fetching task dependents: %w", err)
	}
	for _, id := range dependents {
		_, err := tx.Exec(context.Background(), sqlCancelTask, id, "parent task canceled")
		if err != nil {
			return fmt.Errorf("error cancelling dependent task: %w", err)
		}
	}

	_, err = tx.Exec(ctx, sqlNotify)
	if err != nil {
		return fmt.Errorf("error notifying tasks channel: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("unable to commit database transaction: %w", err)
	}

	return nil
}

func (p *PgQueue) sendCancelNotification(ctx context.Context, taskId uuid.UUID) error {
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	channelName := getCancelChannelName(taskId)
	_, err = conn.Exec(ctx, "select pg_notify($1, 'cancel')", channelName)
	if err != nil {
		return fmt.Errorf("error notifying cancel channel: %w", err)
	}
	return nil
}

func (p *PgQueue) Requeue(taskId uuid.UUID) error {
	var err error

	tx, err := p.Pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %w", err)
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
	if info.CancelAttempted && info.Status != config.TaskStatusRunning {
		return ErrTaskCanceled
	}
	if info.Started == nil || info.Finished != nil {
		return ErrNotRunning
	}
	if info.Retries == MaxTaskRetries {
		err = p.Finish(info.Id, ErrMaxRetriesExceeded)
		if err != nil {
			return fmt.Errorf("error finishing task")
		}
		err = tx.Commit(context.Background())
		if err != nil {
			return fmt.Errorf("unable to commit database transaction: %w", err)
		}
		return ErrMaxRetriesExceeded
	}

	// Remove from heartbeats
	tag, err := tx.Exec(context.Background(), sqlDeleteHeartbeat, taskId)
	if err != nil {
		return fmt.Errorf("error removing task %s from heartbeats: %w", taskId, err)
	}
	if tag.RowsAffected() != 1 {
		logger := log.Logger.With().Str("task_id", taskId.String()).Logger()
		logger.Warn().Msgf("error requeuing task: error deleting heartbeat: heartbeat not found. was this task finished recently?")
	}

	tag, err = tx.Exec(context.Background(), sqlRequeue, taskId)
	if err != nil {
		return fmt.Errorf("error requeueing task %s: %v", taskId, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("error requeuing task: %w", ErrNotExist)
	}

	_, err = tx.Exec(context.Background(), sqlNotify)
	if err != nil {
		return fmt.Errorf("error notifying tasks channel: %w", err)
	}

	err = tx.Commit(context.Background())
	if err != nil {
		return fmt.Errorf("unable to commit database transaction: %w", err)
	}

	return nil
}

func (p *PgQueue) RequeueFailedTasks(taskTypes []string) error {
	var err error

	tx, err := p.Pool.Begin(context.Background())
	if err != nil {
		return fmt.Errorf("error starting database transaction: %v", err)
	}
	defer func() {
		err = tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			log.Logger.Error().Err(err).Msg("Error rolling back retry failed tasks transaction")
		}
	}()

	tag, err := tx.Exec(context.Background(), sqlRequeueFailedTasks, pq.Array(taskTypes))
	if err != nil {
		return fmt.Errorf("error requeueing failed tasks: %w", err)
	}
	if tag.RowsAffected() != 0 {
		log.Logger.Info().Msg("Failed tasks being requeued")
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
		return fmt.Errorf("error removing all tasks: %w", err)
	}
	return nil
}

func (p *PgQueue) ListenForCancel(ctx context.Context, taskID uuid.UUID, cancelFunc context.CancelCauseFunc) {
	logger := zerolog.Ctx(ctx)
	conn, err := p.Pool.Acquire(ctx)

	if err != nil {
		// If the task is finished before listen is initiated, or server is exited, a context canceled error is expected
		if !isContextCancelled(ctx) {
			logger.Error().Err(err).Msg("ListenForCancel: error acquiring connection")
		}
		return
	}
	defer conn.Release()

	// Register a channel for the task where a notification can be sent to cancel the task
	channelName := getCancelChannelName(taskID)
	_, err = conn.Conn().Exec(ctx, "listen "+channelName)
	if err != nil {
		if !isContextCancelled(ctx) {
			logger.Error().Err(err).Msg("ListenForCancel: error registering channel")
		}
		return
	}

	// When the function returns, unregister the channel
	defer func(conn *pgx.Conn) {
		_, err = conn.Exec(context.WithoutCancel(ctx), "unlisten "+channelName)
		if err != nil {
			logger.Error().Err(err).Msg("ListenForCancel: error unregistering listener")
		}
	}(conn.Conn())

	// Wait for a notification on the channel. This blocks until the channel receives a notification.
	_, err = conn.Conn().WaitForNotification(ctx)
	if err != nil {
		if !isContextCancelled(ctx) {
			logger.Error().Err(err).Msg("ListenForCancel: error waiting for notification")
		}
		return
	}

	// Cancel context only if context has not already been canceled. If the context has already been canceled, the task has finished.
	if !errors.Is(ErrNotRunning, context.Cause(ctx)) {
		logger.Debug().Msg("[Canceled Task]")
		cancelFunc(ErrTaskCanceled)
	}
}

func (p *PgQueue) Close() {
	p.Pool.Close()
}

func isContextCancelled(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), ErrNotRunning) || errors.Is(context.Cause(ctx), ce.ErrServerExited)
}

func getCancelChannelName(taskID uuid.UUID) string {
	return strings.Replace("task_"+taskID.String(), "-", "", -1)
}
