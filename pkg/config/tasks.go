package config

const (
	RepositorySnapshotTask        = "snapshot"                    // Task to create a snapshot for a repository config
	DeleteRepositorySnapshotsTask = "delete-repository-snapshots" // Task to delete all snapshots for a repository config
	DeleteSnapshotsTask           = "delete-snapshots"            // Task to delete all snapshots marked for deletion
	IntrospectTask                = "introspect"                  // Task to introspect repository
	DeleteTemplatesTask           = "delete-templates"            // Task to delete all content templates marked for deletion
	UpdateTemplateContentTask     = "update-template-content"     // Task to update the pulp distributions of a template's snapshots
	UpdateRepositoryTask          = "update-repository"           // Task to update repository information in candlepin when the repository is updated
	AddUploadsTask                = "add-uploads-repository"      // Task to add uploaded files/artifacts to a repository
	UpdateLatestSnapshotTask      = "update-latest-snapshot"      // Task to update templates to use the latest snapshot of a repository
)

const (
	TaskStatusRunning   = "running"   // Task is running
	TaskStatusFailed    = "failed"    // Task has failed
	TaskStatusCompleted = "completed" // Task has completed
	TaskStatusCanceled  = "canceled"  // Task has been canceled
	TaskStatusPending   = "pending"   // Task is waiting to be started
)

var RequeueableTasks = []string{DeleteTemplatesTask, DeleteRepositorySnapshotsTask, UpdateTemplateContentTask, DeleteSnapshotsTask}

var CancellableTasks = []string{IntrospectTask, RepositorySnapshotTask, UpdateTemplateContentTask}

const ObjectTypeRepository = "repository"
const ObjectTypeTemplate = "template"

// TasksToCleanup tasks that will get deleted, completed or failed, if older than 20 days
var TasksToCleanup = []string{
	IntrospectTask,
	RepositorySnapshotTask,
	UpdateTemplateContentTask,
	UpdateRepositoryTask,
	UpdateLatestSnapshotTask,
	AddUploadsTask,
}

// TasksToCleanupIfCompleted tasks that will get deleted if older than 10 days, only if status is completed
var TasksToCleanupIfCompleted = []string{DeleteRepositorySnapshotsTask, DeleteTemplatesTask, DeleteSnapshotsTask}
