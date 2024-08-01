package config

const (
	RepositorySnapshotTask        = "snapshot"                    // Task to create a snapshot for a repository config
	DeleteRepositorySnapshotsTask = "delete-repository-snapshots" // Task to delete all snapshots for a repository config
	IntrospectTask                = "introspect"                  // Task to introspect repository
	DeleteTemplatesTask           = "delete-templates"            // Task to delete all content templates marked for deletion
	UpdateTemplateContentTask     = "update-template-content"     // Task to update the pulp distributions of a template's snapshots
	UpdateRepositoryTask          = "update-repository"           // Task to update repository information in candlepin when the repository is updated
	AddUploadsTask                = "add-uploads-repository"      // Task to add uploaded files/artifacts to a repository
	UpdateLatestSnapshotTask      = "update-latest-snapshot"      // TODO write description
)

const (
	TaskStatusRunning   = "running"   // Task is running
	TaskStatusFailed    = "failed"    // Task has failed
	TaskStatusCompleted = "completed" // Task has completed
	TaskStatusCanceled  = "canceled"  // Task has been canceled
	TaskStatusPending   = "pending"   // Task is waiting to be started
)

var RequeueableTasks = []string{DeleteTemplatesTask, DeleteRepositorySnapshotsTask, UpdateTemplateContentTask}

var CancellableTasks = []string{IntrospectTask, RepositorySnapshotTask}
