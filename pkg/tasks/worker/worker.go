package worker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	m "github.com/content-services/content-sources-backend/pkg/instrumentation"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/tasks"
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
	runningTask *runningTask  // holds information about the in-progress task
}

type workerConfig struct {
	queue     queue.Queue
	workerWg  *sync.WaitGroup
	handlers  map[string]TaskHandler
	taskTypes []string
}

type runningTask struct {
	id             uuid.UUID
	token          uuid.UUID
	typename       string
	requestID      string
	taskCancelFunc context.CancelCauseFunc
	cancelled      bool
}

func (t *runningTask) setTaskInfo(info *models.TaskInfo) {
	t.id = info.Id
	t.token = info.Token
	t.typename = info.Typename
	t.requestID = info.RequestID
}

func (t *runningTask) clear() {
	t.id = uuid.Nil
	t.token = uuid.Nil
	t.typename = ""
	t.requestID = ""
	t.cancelled = false
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
	defer w.recoverOnPanic(log.Logger)

	w.readyChan <- struct{}{}

	beat := time.NewTimer(config.Get().Tasking.Heartbeat / 3)
	defer beat.Stop()

	for {
		select {
		case <-w.stopChan:
			if w.runningTask.id != uuid.Nil {
				err := w.requeue(w.runningTask.id)
				if err != nil {
					log.Logger.Error().Err(err).Msgf("error requeuing task with task_id: %v", w.runningTask.id)
				}
			}
			return
		case <-w.readyChan:
			taskCtx, taskCancel := context.WithCancelCause(ctx)
			w.runningTask.taskCancelFunc = taskCancel

			taskInfo, err := w.dequeue(taskCtx)
			if err != nil {
				if err == queue.ErrContextCanceled {
					continue
				}
				continue
			}

			if taskInfo != nil {
				taskCtx = logForTask(w.runningTask).WithContext(taskCtx)
				go w.queue.ListenForCancel(taskCtx, w.runningTask.id, w.runningTask.taskCancelFunc)
				go w.process(taskCtx, taskInfo)
			}
		case <-beat.C:
			if w.runningTask.token != uuid.Nil {
				err := w.queue.RefreshHeartbeat(w.runningTask.token)
				if err != nil {
					if err == queue.ErrRowsNotAffected {
						log.Logger.Error().Err(nil).Msg("No rows affected when refreshing heartbeat")
						continue
					}
					log.Logger.Error().Err(err).Msg("Error refreshing heartbeat")
				}
			}
			beat.Reset(config.Get().Tasking.Heartbeat / 3)
		}
	}
}

func (w *worker) dequeue(ctx context.Context) (*models.TaskInfo, error) {
	logger := logForTask(w.runningTask)
	defer w.recoverOnPanic(*logger)

	info, err := w.queue.Dequeue(ctx, w.taskTypes)
	if err != nil {
		if err == queue.ErrContextCanceled {
			return nil, err
		}
		log.Logger.Error().Err(err).Msg("error dequeuing task")
		w.readyChan <- struct{}{}
		return nil, err
	}
	if w.metrics != nil {
		w.metrics.RecordMessageLatency(*info.Queued)
	}

	w.runningTask.setTaskInfo(info)
	logForTask(w.runningTask).Info().Msg("[Dequeued Task]")

	return info, nil
}

func (w *worker) requeue(id uuid.UUID) error {
	logger := logForTask(w.runningTask)
	defer w.recoverOnPanic(*logger)

	err := w.queue.Requeue(id)
	if err != nil && errors.Is(err, queue.ErrMaxRetriesExceeded) {
		logger.Warn().Msgf("[Task Finished] task failed: %v", queue.ErrMaxRetriesExceeded)
		return nil
	}
	if err != nil {
		return err
	}
	logger.Info().Msg("[Requeued Task]")
	return nil
}

// process calls the handler for the task specified by taskInfo, finishes the task, then marks worker as ready for new task
func (w *worker) process(ctx context.Context, taskInfo *models.TaskInfo) {
	ctx = context.WithValue(ctx, config.ContextRequestIDKey{}, taskInfo.RequestID)
	logger := zerolog.Ctx(ctx)
	defer w.recoverOnPanic(*logger)

	if handler, ok := w.handlers[taskInfo.Typename]; ok {
		var finishStr string

		handlerErr := handler(ctx, taskInfo, &w.queue)

		err := w.queue.Finish(taskInfo.Id, handlerErr)
		if err != nil {
			logger.Error().Msgf("error finishing task: %v", err)
		}

		if errors.Is(handlerErr, context.Canceled) {
			finishStr = "task canceled"
			w.recordMessageResult(true)
			logger.Info().Msgf("[Finished Task] %v", finishStr)
		} else if handlerErr != nil && taskInfo.Retries >= queue.MaxTaskRetries {
			finishStr = "task failed and retry limit reached"
			w.recordMessageResult(false)
			logger.Error().Err(handlerErr).Msgf("[Finished Task] %v", finishStr)
		} else if handlerErr != nil {
			finishStr = "task failed"
			w.recordMessageResult(false)
			logger.Warn().Err(handlerErr).Msgf("[Finished Task] %v", finishStr)
		} else {
			finishStr = "task completed"
			w.recordMessageResult(true)
			logger.Info().Msgf("[Finished Task] %v", finishStr)
		}

		w.runningTask.clear()
	} else {
		logger.Warn().Msg("handler not found for task type")
	}
	w.runningTask.taskCancelFunc(queue.ErrNotRunning)
	w.readyChan <- struct{}{}
}

func (w *worker) recordMessageResult(res bool) {
	if w.metrics != nil {
		w.metrics.RecordMessageResult(res)
	}
}
func (w *worker) stop() {
	w.stopChan <- struct{}{}
}

// Catches a panic so that only the surrounding function is exited
func (w *worker) recoverOnPanic(logger zerolog.Logger) {
	var err error
	if r := recover(); r != nil {
		err, _ = r.(error)
		logger.Error().Err(err).Stack().Msgf("recovered panic in worker with error: %v", err)
		logger.Info().Msgf("[Finished Task] task failed (panic)")

		if w.runningTask != nil {
			tErr := w.queue.Finish(w.runningTask.id, err)
			if tErr != nil {
				log.Error().Err(tErr).Msgf("Could not update task during panic recovery, original error: %v", err.Error())
			}

			if w.runningTask.taskCancelFunc != nil {
				w.runningTask.taskCancelFunc(queue.ErrNotRunning)
			}
			w.runningTask.clear()
		}
		w.readyChan <- struct{}{}
	}
}

func logForTask(task *runningTask) *zerolog.Logger {
	logger := tasks.LogForTask(task.id.String(), task.typename, task.requestID)
	return logger
}
