package worker

import (
	"context"
	"sync"
	"time"

	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type TaskHandler func(ctx context.Context, task *queue.TaskInfo) error

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
	queue             queue.Queue
	numWorkers        int
	logger            *zerolog.Logger
	heartbeatInterval time.Duration          // interval to check for missed heartbeats
	heartbeat         time.Duration          // length of heartbeat
	workerWg          *sync.WaitGroup        // wait for all workers to exit
	handlers          map[string]TaskHandler // associates a handler function to a typename
	taskTypes         []string               // list of typenames
	workers           []*worker              // list of workers
	metrics           *m.Metrics
}

type Config struct {
	NumWorkers        int
	HeartbeatInterval time.Duration // interval to poll heartbeats
	Heartbeat         time.Duration
}

func NewTaskWorkerPool(config Config, queue queue.Queue, metrics *m.Metrics) TaskWorkerPool {
	workerWg := sync.WaitGroup{}
	return &WorkerPool{
		queue:             queue,
		numWorkers:        config.NumWorkers,
		logger:            &log.Logger,
		workerWg:          &workerWg,
		handlers:          make(map[string]TaskHandler),
		heartbeat:         config.Heartbeat,
		heartbeatInterval: config.HeartbeatInterval,
		metrics:           metrics,
	}
}

func (w *WorkerPool) HeartbeatListener() {
	go func() {
		w.logger.Info().Msg("starting task heartbeat listener")
		for {
			//nolint:staticcheck
			for range time.Tick(w.heartbeatInterval) {
				for _, token := range w.queue.Heartbeats(w.heartbeatInterval) {
					id, err := w.queue.IdFromToken(token)
					if err != nil {
						w.logger.Warn().Err(err).Msg("error getting task id")
					}

					err = w.queue.Requeue(id)
					if err != nil {
						w.logger.Warn().Err(err).Msg("error requeuing task")
					}
				}
			}
		}
	}()
}

func (w *WorkerPool) StartWorkers(ctx context.Context) {
	for i := 0; i < w.numWorkers; i++ {
		wrk := newWorker(workerConfig{
			queue:     w.queue,
			logger:    w.logger,
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
	w.logger.Info().Msg("Stopping workers")
	for _, wrk := range w.workers {
		wrk.stop()
	}
	w.workerWg.Wait()
}
