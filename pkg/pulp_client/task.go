package pulp_client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	zest "github.com/content-services/zest/release/v3"
)

const (
	COMPLETED string = "completed"
	WAITING   string = "waiting"
	RUNNING   string = "running"
	SKIPPED   string = "skipped"
	CANCELED  string = "canceled"
	FAILED    string = "failed"
)

// GetTask Fetch a pulp task
func (r pulpDaoImpl) GetTask(taskHref string) (zest.TaskResponse, error) {
	task, _, err := r.client.TasksApi.TasksRead(r.ctx, taskHref).Execute()

	if err != nil {
		return zest.TaskResponse{}, err
	}

	return *task, nil
}

// PollTask Poll a task and return the final task object
func (r pulpDaoImpl) PollTask(taskHref string) (*zest.TaskResponse, error) {
	var task zest.TaskResponse
	var err error
	syncInProgress := true
	pollCount := 1
	for syncInProgress {
		task, err = r.GetTask(taskHref)
		if err != nil {
			return nil, err
		}

		taskState := *task.State
		prettyResponse, _ := json.MarshalIndent(task, "", "    ")
		fmt.Printf("%v", task)
		print("============ %v", taskState)

		switch {
		case taskState == COMPLETED:
			syncInProgress = false
		case taskState == WAITING:
			print("\n================== Waiting ==================\n")
		case taskState == RUNNING:
			print("\n================== Running ==================\n")
		case taskState == SKIPPED:
			print("\n================== Skipped ==================\n")
			fmt.Fprintf(os.Stdout, "Response from Repository Creation Skipped %v\nTask State is: %s \n\n", string(prettyResponse), taskState)
			syncInProgress = false
		case taskState == CANCELED:
			print("\n================== Canceled ==================\n")
			fmt.Fprintf(os.Stdout, "Response from Repository Creation Canceled %v\nTask State is: %s \n\n", string(prettyResponse), taskState)
			syncInProgress = false
		case taskState == FAILED:
			return &task, errors.New(TaskErrorString(task))
		default:
			print("\nState returned something else:", taskState, "\n")
			syncInProgress = false
		}

		if syncInProgress {
			SleepWithBackoff(pollCount)
			pollCount += 1
		}
	}
	return &task, nil
}

func SleepWithBackoff(iteration int) {
	var secs int
	if iteration <= 5 {
		secs = 1
	} else if iteration > 5 && iteration <= 10 {
		secs = 5
	} else if iteration > 10 && iteration <= 20 {
		secs = 10
	} else {
		secs = 30
	}
	fmt.Printf("Sleeping for %v\n", secs)
	time.Sleep(time.Duration(secs) * time.Second)
	fmt.Printf("Done")
}

func TaskErrorString(task zest.TaskResponse) string {
	str := ""
	for key, element := range task.Error {
		str = str + fmt.Sprintf("%v: %v.  ", key, element)
	}
	return str
}

func SelectVersionHref(task *zest.TaskResponse) *string {
	return SelectCreatedVersionHref(task, "/pulp/api/v3/repositories/rpm/rpm/.*/versions/[0-9]*/")
}

func SelectPublicationHref(task *zest.TaskResponse) *string {
	return SelectCreatedVersionHref(task, "/pulp/api/v3/publications/rpm/rpm/.*/")
}

func SelectRpmDistributionHref(task *zest.TaskResponse) *string {
	return SelectCreatedVersionHref(task, "/pulp/api/v3/distributions/rpm/rpm/.*/")
}

// SelectCreatedVersionHref scans a tasks CreatedResources and looks for a match to a regular expression
func SelectCreatedVersionHref(task *zest.TaskResponse, regex string) *string {
	if task != nil {
		for i := 0; i < len(task.CreatedResources); i++ {
			match, err := regexp.MatchString(regex, task.CreatedResources[i])
			if err == nil && match {
				return &task.CreatedResources[i]
			}
		}
	}
	return nil
}
