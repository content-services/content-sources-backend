# Tasking System Architecture
**Features**
* Queue and process asynchronous tasks
* Check the status of queued tasks
* Requeue a task if its worker times-out or exits early
* Schedule dependent tasks

**Layout**

| Package          | Description                                                 |
|------------------|-------------------------------------------------------------|
| pkg/tasks        | each file contains code for handling a particular task type |
| pkg/tasks/queue  | queue used by client and worker to schedule tasks           |
| pkg/tasks/client | an interface to enqueue a task                              |
| pkg/tasks/worker | an interface to dequeue and handle tasks                    |

## pkg/tasks

`introspect.go`: handler and structures to support introspect task type.

## pkg/tasks/queue

Queue is an interface used by the client and worker packages for scheduling tasks. It is meant to be used through client/worker, not imported indepedently.

[permalink to code of Queue interface]


It also contains a postgres-backed implementation of the Queue interface in `pgqueue.go`.

## pkg/tasks/client

TaskClient is an interface for enqueuing tasks.

[ permalink to code of Client interface]


## pkg/tasks/worker

`TaskWorkerPool` is an interface used by the main application to start/configure the workers and the heartbeat listener.

[permalink to interface code]


A worker pool will manage the individual workers. Workers are meant to be used through the `TaskWorkerPool` interface, not directly.

Each worker is a goroutine looping with 3 potential paths:
1. The worker's context is cancelled, so the worker requeues its in-progress task and exits
2. The worker is ready to pick up a new task, so it dequeues a task and processes it. Each worker can only process one task at a time.
3. The worker's heartbeat is refreshed.

