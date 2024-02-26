package worker

import (
	"context"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog/log"
)

type TaskHandler func(ctx context.Context, task *models.TaskInfo, queue *queue.Queue) error

type TaskWorkerPool interface {
	// StartWorkers Starts workers up to number numWorkers defined in config.
	// Should be run as a go routine.
	StartWorkers(ctx context.Context)
	// Stop Gracefully stops all workers
	Stop()
	// HeartbeatListener requeues tasks of workers whose heartbeats do not refresh within heartbeat duration
	HeartbeatListener()
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
}

func NewTaskWorkerPool(queue queue.Queue, metrics *m.Metrics) TaskWorkerPool {
	workerWg := sync.WaitGroup{}
	return &WorkerPool{
		queue:    queue,
		workerWg: &workerWg,
		handlers: make(map[string]TaskHandler),
		metrics:  metrics,
	}
}

func (w *WorkerPool) HeartbeatListener() {
	heartbeat := config.Get().Tasking.Heartbeat
	go func() {
		log.Logger.Info().Msg("starting task heartbeat listener")
		for {
			//nolint:staticcheck
			for range time.Tick(heartbeat) {
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
				err := w.queue.RequeueFailedTasks(config.RequeueableTasks)
				if err != nil {
					log.Logger.Warn().Err(err).Msg("error requeuing failed tasks")
				}
			}
		}
	}()
}

func (w *WorkerPool) StartWorkers(ctx context.Context) {
	for i := 0; i < config.Get().Tasking.WorkerCount; i++ {
		wrk := newWorker(workerConfig{
			queue:     w.queue,
			workerWg:  w.workerWg,
			handlers:  w.handlers,
			taskTypes: w.taskTypes,
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
