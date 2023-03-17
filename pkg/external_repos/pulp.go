package external_repos

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/pulp_client"
)

// CreatePulpRepoFromURL
func CreatePulpRepoFromURL(url string) []error {
	var errs []error
	repoDao := dao.GetRepositoryDao(db.DB)
	repo, err := repoDao.FetchForUrl(url)

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n\nDidn't find URL reference in repoDao: %v\n\n", repo.URL)
		return []error{err}
	}

	pulpClient := pulp_client.GetPulpClient()

	err = PulpCreate(&repo, pulpClient)
	if err != nil {
		errs = append(errs, fmt.Errorf("Error creating pulp reference for %s: %s", repo.URL, err.Error()))
	}

	return errs
}

func PulpCreate(repo *dao.Repository, pulpClient pulp_client.PulpClient) error {
	fmt.Printf("\n\nRunning Pulp Create for: %s \n", repo.URL)
	var remoteHref string
	var repoHref string
	var taskHref string

	//Create remote
	createResp, firstErr := pulpClient.CreateRpmRemote(repo.UUID, repo.URL)

	if firstErr != nil {
		//Err, check if the remote exists, use it if so.
		resp, secondErr := pulpClient.GetRpmRemoteByName(repo.UUID)
		if secondErr != nil {
			return firstErr
		}
		remoteHref = *resp.PulpHref
	} else {
		remoteHref = *createResp.PulpHref
	}

	// Create Repository
	remoteResp, createErr := pulpClient.CreateRpmRepository(repo.UUID, repo.URL, &remoteHref)

	if createErr != nil {
		//Err, Presume repository exists, use it if so.
		resp, err := pulpClient.GetRpmRemoteByName(repo.UUID)
		if err != nil {
			return createErr
		}
		repoHref = *resp.PulpHref
	} else {
		repoHref = *remoteResp.PulpHref
	}

	taskHref, err := pulpClient.SyncRpmRepository(repoHref, nil)

	if err != nil {
		return err
	}

	syncInProgress := true
	syncCount := 1
	for syncInProgress {
		task, err := pulpClient.GetTask(taskHref)

		if err != nil {
			return err
		}

		taskState := *task.State
		prettyResponse, _ := json.MarshalIndent(task, "", "    ")

		switch {
		case taskState == "completed":
			print("\n================== DONE! ==================\n")
			fmt.Printf("\nRepository Creation Success\n\nTask State is: %s \n\n", taskState)
			syncInProgress = false
		case taskState == "waiting":
			print("\n================== Waiting ==================\n")
		case taskState == "running":
			print("\n================== Running ==================\n")
		case taskState == "skipped":
			print("\n================== Skipped ==================\n")
			fmt.Fprintf(os.Stdout, "Response from Repository Creation Skipped %v\nTask State is: %s \n\n", string(prettyResponse), taskState)
			syncInProgress = false
		case taskState == "canceled":
			print("\n================== Canceled ==================\n")
			fmt.Fprintf(os.Stdout, "Response from Repository Creation Canceled %v\nTask State is: %s \n\n", string(prettyResponse), taskState)
			syncInProgress = false
		case taskState == "failed":
			print("\n================== Failed ==================\n")
			fmt.Fprintf(os.Stdout, "Response from Repository Creation Failed %v\nTask State is: %s \n\n", string(prettyResponse), taskState)
			print(task.Error)
			syncInProgress = false
		default:
			print("\nState returned something else:", taskState, "\n")
			syncInProgress = false
		}

		if taskState != "completed" {
			fmt.Printf("\nPollingTime: %v seconds\n", syncCount)
			syncCount++
			time.Sleep(1000)
		}
	}

	return nil
}
