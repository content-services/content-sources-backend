package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type TaskHandler func(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error

type TaskWorkerPool interface {
	// StartWorkerPool starts background processes and workers
	// Should be run as a go routine.
	StartWorkerPool(ctx context.Context)
	// StartWorkers Starts workers up to number numWorkers defined in config.
	StartWorkers(ctx context.Context)
	// Stop Gracefully stops all workers
	Stop()
	// HeartbeatListener requeues tasks of workers whose heartbeats do not refresh within heartbeat duration
	HeartbeatListener(ctx context.Context)
	// ListenForCancelledTasks listens for task cancellation notifications
	ListenForCancelledTasks(ctx context.Context)
	// RequeueFailedTasks requeues failed tasks at regular intervals
	RequeueFailedTasks(ctx context.Context)
	// RegisterHandler assigns a function of type TaskHandler to a typename.
	// This function is the action performed to tasks of typename taskType.
	RegisterHandler(taskType string, handler TaskHandler)
}

type WorkerPool struct {
	queue     queue.Queue
	workerWg  *sync.WaitGroup        // wait for all workers to exit
	handlers  map[string]TaskHandler // associates a handler function to a typename
	taskTypes []string               // list of typenames
	workers   []*worker              // list of workers
	metrics   *m.Metrics
	workerMap *utils.ConcurrentMap[uuid.UUID, *worker]
}

func NewTaskWorkerPool(queue queue.Queue, metrics *m.Metrics) TaskWorkerPool {
	workerWg := sync.WaitGroup{}
	return &WorkerPool{
		queue:     queue,
		workerWg:  &workerWg,
		handlers:  make(map[string]TaskHandler),
		metrics:   metrics,
		workerMap: utils.NewConcurrentMap[uuid.UUID, *worker](),
	}
}

func (w *WorkerPool) HeartbeatListener(ctx context.Context) {
	heartbeat := config.Get().Tasking.Heartbeat
	ticker := time.NewTicker(heartbeat / 3)
	defer ticker.Stop()

	log.Logger.Info().Msg("Starting task heartbeat listener")
	for {
		select {
		case <-ctx.Done():
			log.Logger.Info().Msg("heartbeat listener shutting down")
			return
		case <-ticker.C:
			for _, token := range w.queue.Heartbeats(heartbeat) {
				id, isRunning, err := w.queue.IdFromToken(token)
				if err != nil {
					log.Logger.Warn().Err(err).Msg("error getting task id")
				}

				if isRunning {
					err = w.queue.Requeue(id)
					if err != nil {
						log.Logger.Warn().Err(err).Msg("error requeuing task")
					}
				}
			}
		}
	}
}

func (w *WorkerPool) RequeueFailedTasks(ctx context.Context) {
	heartbeat := config.Get().Tasking.Heartbeat
	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()

	log.Logger.Info().Msg("Starting failed task requeue listener")
	for {
		select {
		case <-ctx.Done():
			log.Logger.Info().Msg("failed task requeue listener shutting down")
			return
		case <-ticker.C:
			err := w.queue.RequeueFailedTasks(config.RequeueableTasks)
			if err != nil {
				log.Logger.Warn().Err(err).Msg("error requeuing failed tasks")
			}
		}
	}
}

func (w *WorkerPool) ListenForCancelledTasks(ctx context.Context) {
	log.Logger.Info().Msg("Starting task cancellation listener")
	for {
		taskToCancel, err := w.queue.ListenForCanceledTask(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Logger.Info().Msg("cancellation listener shutting down")
				return
			}
			log.Logger.Warn().Err(err).Msg("error listening for canceled tasks")
			continue
		}

		wrk, _ := w.workerMap.Get(taskToCancel)
		if wrk != nil && wrk.runningTask != nil && wrk.runningTask.taskCancelFunc != nil {
			wrk.runningTask.taskCancelFunc(queue.ErrTaskCanceled)
		}
	}
}

func (w *WorkerPool) StartWorkerPool(ctx context.Context) {
	go w.HeartbeatListener(ctx)
	go w.ListenForCancelledTasks(ctx)
	go w.RequeueFailedTasks(ctx)
	w.StartWorkers(ctx)
}

func (w *WorkerPool) StartWorkers(ctx context.Context) {
	workerCount := config.Get().Tasking.WorkerCount

	log.Info().Msgf("Starting %v workers", workerCount)
	for i := 0; i < workerCount; i++ {
		wrk := newWorker(workerConfig{
			queue:     w.queue,
			workerWg:  w.workerWg,
			handlers:  w.handlers,
			taskTypes: w.taskTypes,
			workerMap: w.workerMap,
		}, w.metrics)

		w.workers = append(w.workers, &wrk)
		wrk.workerWg.Add(1)
		go wrk.start(ctx)
	}
}

func (w *WorkerPool) RegisterHandler(taskType string, handler TaskHandler) {
	w.handlers[taskType] = handler
	if !contains(w.taskTypes, taskType) {
		w.taskTypes = append(w.taskTypes, taskType)
	}
}

func (w *WorkerPool) Stop() {
	log.Logger.Info().Msg("Stopping workers")
	for _, wrk := range w.workers {
		wrk.stop()
	}
	w.workerWg.Wait()
}
