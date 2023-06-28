package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type worker struct {
	queue       queue.Queue
	workerWg    *sync.WaitGroup // wait for worker loop to exit
	handlers    map[string]TaskHandler
	taskTypes   []string
	metrics     *m.Metrics
	readyChan   chan struct{} // receives value when worker is ready for new task
	stopChan    chan struct{} // receives value when worker should exit gracefully
	runningTask *runningTask  // holds ID and token of in-progress task
}

type workerConfig struct {
	queue     queue.Queue
	workerWg  *sync.WaitGroup
	handlers  map[string]TaskHandler
	taskTypes []string
}

type runningTask struct {
	mu        sync.Mutex
	taskId    uuid.UUID // only set this value using the setter method
	taskToken uuid.UUID // only set this value using the setter method
}

func (t *runningTask) SetTaskID(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.taskId = id
}

func (t *runningTask) SetTaskToken(id uuid.UUID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.taskToken = id
}

func newWorker(config workerConfig, metrics *m.Metrics) worker {
	return worker{
		queue:       config.queue,
		workerWg:    config.workerWg,
		handlers:    config.handlers,
		taskTypes:   config.taskTypes,
		readyChan:   make(chan struct{}, 1),
		stopChan:    make(chan struct{}, 1),
		metrics:     metrics,
		runningTask: &runningTask{},
	}
}

func (w *worker) start(ctx context.Context) {
	log.Logger.Info().Msg("Starting worker")
	defer w.workerWg.Done()
	defer recoverOnPanic(log.Logger)

	w.readyChan <- struct{}{}

	beat := time.NewTimer(config.Get().Tasking.Heartbeat / 3)
	defer beat.Stop()

	for {
		select {
		case <-w.stopChan:
			if w.runningTask.taskId != uuid.Nil {
				err := w.requeue(w.runningTask.taskId)
				if err != nil {
					log.Logger.Warn().Msg(fmt.Sprintf("error requeueing task: %v", err))
				}
			}
			return
		case <-w.readyChan:
			taskInfo, err := w.dequeue(ctx)
			if err != nil {
				if err == queue.ErrContextCanceled {
					continue
				}
				log.Logger.Warn().Msg(fmt.Sprintf("error dequeuing task: %v", err))
				w.readyChan <- struct{}{}
				continue
			}
			if taskInfo != nil {
				w.metrics.RecordMessageLatency(*taskInfo.Queued)
				w.runningTask.SetTaskID(taskInfo.Id)
				w.runningTask.SetTaskToken(taskInfo.Token)
				go w.process(ctx, taskInfo)
			}
		case <-beat.C:
			if w.runningTask.taskToken != uuid.Nil {
				w.queue.RefreshHeartbeat(w.runningTask.taskToken)
			}
			beat.Reset(config.Get().Tasking.Heartbeat / 3)
		}
	}
}

func (w *worker) dequeue(ctx context.Context) (*models.TaskInfo, error) {
	defer recoverOnPanic(log.Logger)
	return w.queue.Dequeue(ctx, w.taskTypes)
}

func (w *worker) requeue(id uuid.UUID) error {
	defer recoverOnPanic(log.Logger)
	return w.queue.Requeue(id)
}

// process calls the handler for the task specified by taskInfo, finishes the task, then marks worker as ready for new task
func (w *worker) process(ctx context.Context, taskInfo *models.TaskInfo) {
	defer recoverOnPanic(log.Logger)
	if handler, ok := w.handlers[taskInfo.Typename]; ok {
		err := handler(ctx, taskInfo, &w.queue)
		if err != nil {
			w.metrics.RecordMessageResult(false)
		} else {
			w.metrics.RecordMessageResult(true)
		}

		err = w.queue.Finish(taskInfo.Id, err)
		if err != nil {
			log.Logger.Warn().Msg(fmt.Sprintf("error finishing task: %v", err))
		}
		w.runningTask.SetTaskID(uuid.Nil)
		w.runningTask.SetTaskToken(uuid.Nil)
	} else {
		log.Logger.Warn().Msg(fmt.Sprintf("handler not found for task type, %s", taskInfo.Typename))
	}
	w.readyChan <- struct{}{}
}

func (w *worker) stop() {
	w.stopChan <- struct{}{}
}

// Catches a panic so that only the surrounding function is exited
func recoverOnPanic(logger zerolog.Logger) {
	var err error
	if r := recover(); r != nil {
		err, _ = r.(error)
		logger.Err(err).Stack().Msg(fmt.Sprintf("recovered panic in worker with error: %v", err))
	}
}
