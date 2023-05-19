package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type worker struct {
	queue     queue.Queue
	logger    *zerolog.Logger
	workerWg  *sync.WaitGroup // wait for worker loop to exit
	handlers  map[string]TaskHandler
	taskTypes []string
	metrics   *m.Metrics
	readyChan chan struct{} // receives value when worker is ready for new task
	stopChan  chan struct{} // receives value when worker should exit gracefully
}

type workerConfig struct {
	queue     queue.Queue
	logger    *zerolog.Logger
	workerWg  *sync.WaitGroup
	handlers  map[string]TaskHandler
	taskTypes []string
}

func newWorker(config workerConfig, metrics *m.Metrics) worker {
	return worker{
		queue:     config.queue,
		logger:    config.logger,
		workerWg:  config.workerWg,
		handlers:  config.handlers,
		taskTypes: config.taskTypes,
		readyChan: make(chan struct{}, 1),
		stopChan:  make(chan struct{}, 1),
		metrics:   metrics,
	}
}

func (w *worker) start(ctx context.Context) {
	w.logger.Info().Msg("Starting worker")
	defer w.workerWg.Done()
	defer recoverOnPanic(w.logger)

	var taskId uuid.UUID
	var taskToken uuid.UUID
	w.readyChan <- struct{}{}

	beat := time.NewTimer(config.Get().Tasking.Heartbeat / 3)
	defer beat.Stop()

	for {
		select {
		case <-w.stopChan:
			if taskId != uuid.Nil {
				err := w.requeue(taskId)
				if err != nil {
					w.logger.Warn().Msg(fmt.Sprintf("error requeueing task: %v", err))
				}
			}
			return
		case <-w.readyChan:
			taskInfo, err := w.dequeue(ctx)
			if err != nil {
				if err == queue.ErrContextCanceled {
					continue
				}
				w.logger.Warn().Msg(fmt.Sprintf("error dequeuing task: %v", err))
				w.readyChan <- struct{}{}
				continue
			}
			if taskInfo != nil {
				w.metrics.RecordMessageLatency(*taskInfo.Queued)
				taskId = taskInfo.Id
				taskToken = taskInfo.Token
				go w.process(ctx, taskInfo)
			}
		case <-beat.C:
			w.queue.RefreshHeartbeat(taskToken)
			beat.Reset(config.Get().Tasking.Heartbeat / 3)
		}
	}
}

func (w *worker) dequeue(ctx context.Context) (*queue.TaskInfo, error) {
	defer recoverOnPanic(w.logger)
	return w.queue.Dequeue(ctx, w.taskTypes)
}

func (w *worker) requeue(id uuid.UUID) error {
	defer recoverOnPanic(w.logger)
	return w.queue.Requeue(id)
}

// process calls the handler for the task specified by taskInfo, finishes the task, then marks worker as ready for new task
func (w *worker) process(ctx context.Context, taskInfo *queue.TaskInfo) {
	defer recoverOnPanic(w.logger)
	if handler, ok := w.handlers[taskInfo.Typename]; ok {
		err := handler(ctx, taskInfo, &w.queue)
		if err != nil {
			w.metrics.RecordMessageResult(false)
		} else {
			w.metrics.RecordMessageResult(true)
		}

		err = w.queue.Finish(taskInfo.Id, err)
		if err != nil {
			w.logger.Warn().Msg(fmt.Sprintf("error finishing task: %v", err))
		}
	} else {
		w.logger.Warn().Msg(fmt.Sprintf("handler not found for task type, %s", taskInfo.Typename))
	}
	w.readyChan <- struct{}{}
}

func (w *worker) stop() {
	w.stopChan <- struct{}{}
}

// Catches a panic so that only the surrounding function is exited
func recoverOnPanic(logger *zerolog.Logger) {
	var err error
	if r := recover(); r != nil {
		err, _ = r.(error)
		logger.Err(err).Stack().Msg(fmt.Sprintf("recovered panic in worker with error: %v", err))
	}
}
