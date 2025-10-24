package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/clients/pulp_client"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/content-services/content-sources-backend/pkg/jobs"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/content-services/content-sources-backend/pkg/tasks/queue"
	"github.com/content-services/content-sources-backend/pkg/utils"
	uuid2 "github.com/google/uuid"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PulpLogParsingSuite struct {
	Suite
	dao *dao.DaoRegistry
	ctx context.Context
}

func TestPulpLogParsingSuite(t *testing.T) {
	suite.Run(t, new(PulpLogParsingSuite))
}

func (s *PulpLogParsingSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ctx = context.Background()
	s.dao = dao.GetDaoRegistry(db.DB)

	// Force local storage for integration tests
	config.Get().Clients.Pulp.StorageType = "local"
}

func (s *PulpLogParsingSuite) TestPulpLogParsingWithRealLogs() {
	config.Get().Clients.Pulp.RepoContentGuards = false
	// Create a repository
	accountId := uuid2.NewString()
	repo, err := s.dao.RepositoryConfig.Create(s.ctx, api.RepositoryRequest{
		Name:      utils.Ptr(uuid2.NewString()),
		URL:       utils.Ptr("https://fixtures.pulpproject.org/rpm-unsigned/"),
		AccountID: utils.Ptr(accountId),
		OrgID:     utils.Ptr(accountId),
	})
	require.NoError(s.T(), err)
	repoUuid, err := uuid2.Parse(repo.RepositoryUUID)
	require.NoError(s.T(), err)

	// Create domain for the repository
	domainName, err := s.dao.Domain.FetchOrCreateDomain(s.ctx, accountId)
	require.NoError(s.T(), err)

	// Set up pulp domain
	pulpClient := pulp_client.GetPulpClientWithDomain(domainName)
	_, err = pulpClient.LookupOrCreateDomain(s.ctx, domainName)
	require.NoError(s.T(), err)

	// Start the snapshot task
	taskClient := client.NewTaskClient(&s.queue)
	taskUuid, err := taskClient.Enqueue(queue.Task{
		Typename:   config.RepositorySnapshotTask,
		Payload:    payloads.SnapshotPayload{},
		OrgId:      repo.OrgID,
		ObjectUUID: utils.Ptr(repoUuid.String()),
		ObjectType: utils.Ptr(config.ObjectTypeRepository),
	})
	require.NoError(s.T(), err)

	// Wait for the task to complete
	s.WaitOnTask(taskUuid)

	// Verify the snapshot was created
	snaps, _, err := s.dao.Snapshot.List(s.ctx, repo.OrgID, repo.UUID, api.PaginationData{Limit: -1}, api.FilterData{})
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), snaps)

	// Wait a bit for the snapshot to be fully available
	time.Sleep(5 * time.Second)

	// Fetch the repomd.xml to generate Pulp logs
	distPath := fmt.Sprintf("%s/repodata/repomd.xml", snaps.Data[0].URL)
	err = s.getRequest(distPath, identity.Identity{OrgID: accountId, Internal: identity.Internal{OrgID: accountId}}, 200)
	require.NoError(s.T(), err)

	// Wait a bit for logs to be generated
	time.Sleep(2 * time.Second)

	logs, err := s.capturePulpLogs()
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), logs, "No Pulp logs captured")

	// Test parsePulpLogMessage with the captured logs
	s.testParsePulpLogMessage(logs, domainName, accountId)
}

func callDocker(containerName string) ([]byte, error) {
	cmd := exec.Command("docker", "logs", "--tail", "100", containerName)
	return cmd.Output()
}

func (s *PulpLogParsingSuite) capturePulpLogs() ([]string, error) {
	// Execute podman logs command to get recent logs from the pulp content container
	podman_name := "cs_pulp_content_1"
	output, err := callDocker(podman_name)
	if err != nil {
		docker_name := "pulp-pulp_content-1"
		var err2 error
		output, err2 = callDocker(docker_name)
		if err != nil {
			return nil, fmt.Errorf("failed to execute podman logs: (%w) and docker logs (%w):", err, err2)
		}
	}

	logLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return logLines, nil
}

func (s *PulpLogParsingSuite) testParsePulpLogMessage(logs []string, domainName, orgId string) {
	job := jobs.TransformPulpLogsJob{
		DomainMap: map[string]string{domainName: orgId},
	}

	events := []jobs.PulpLogEvent{}
	for i, logLine := range logs {
		s.T().Logf("Testing log line %d: %s", i+1, logLine)

		// Parse the log message
		if strings.Contains(logLine, "repomd.xml") {
			event := job.ParsePulpLogMessage(logLine)
			if event != nil {
				events = append(events, *event)
			} else {
				s.T().Logf("Log line %d did not match expected format: %s", i+1, logLine)
			}
		}
	}
	assert.NotEmpty(s.T(), events)
	for _, event := range events {
		// Verify the parsed event has the expected structure
		assert.NotEmpty(s.T(), event.Timestamp, "Timestamp should not be empty")
		assert.NotEmpty(s.T(), event.Path, "Path should not be empty")
		assert.NotEmpty(s.T(), event.DomainName, "DomainName should not be empty")
		assert.NotEmpty(s.T(), event.RequestOrgId, "RequestOrgId should not be empty")
		assert.NotEmpty(s.T(), event.UserAgent, "UserAgent should not be empty")
		assert.NotEmpty(s.T(), event.FileSize, "FileSize should not be empty")

		// Verify domain name matches
		assert.Equal(s.T(), domainName, event.DomainName, "Domain name should match")

		// Verify org ID is set correctly
		assert.Equal(s.T(), orgId, event.OrgId, "Org ID should match")

		// Verify the path contains expected elements
		assert.Contains(s.T(), event.Path, "/api/pulp-content/", "Path should contain pulp-content API")
		assert.Contains(s.T(), event.Path, domainName, "Path should contain domain name")

		s.T().Logf("Successfully parsed log event: Domain=%s, OrgId=%s, Path=%s, RequestOrgId=%s",
			event.DomainName, event.OrgId, event.Path, event.RequestOrgId)
	}
}

func (s *PulpLogParsingSuite) getRequest(url string, id identity.Identity, expectedCode int) error {
	client := http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	js, err := json.Marshal(identity.XRHID{Identity: id})
	if err != nil {
		return err
	}
	req.Header = http.Header{}
	req.Header.Add(api.IdentityHeader, base64.StdEncoding.EncodeToString(js))
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	assert.Equal(s.T(), expectedCode, res.StatusCode)

	return nil
}

// Test with a known log format to ensure the parser works correctly
func (s *PulpLogParsingSuite) TestParsePulpLogMessageWithKnownFormat() {
	// Create a test log message in the expected format
	testLog := `10.130.6.126 [01/Jun/2025:23:59:51 +0000] "GET /api/pulp-content/testdomain/templates/8c18e4a4-0/repodata/repomd.xml HTTP/1.1" 302 732 "-" "libdnf (Red Hat Enterprise Linux 9.5; generic; Linux.x86_64)" cache:"HIT" artifact_size:"3141" rh_org_id:"5483888"`

	// Create a TransformPulpLogsJob instance
	job := jobs.TransformPulpLogsJob{
		DomainMap: map[string]string{"testdomain": "12345"},
	}

	// Parse the log message
	event := job.ParsePulpLogMessage(testLog)

	// Verify the parsed event
	require.NotNil(s.T(), event, "Event should not be nil")
	assert.Equal(s.T(), "testdomain", event.DomainName)
	assert.Equal(s.T(), "12345", event.OrgId)
	assert.Equal(s.T(), "5483888", event.RequestOrgId)
	assert.Equal(s.T(), "/api/pulp-content/testdomain/templates/8c18e4a4-0/repodata/repomd.xml", event.Path)
	assert.Equal(s.T(), "libdnf (Red Hat Enterprise Linux 9.5; generic; Linux.x86_64)", event.UserAgent)
	assert.Equal(s.T(), "3141", event.FileSize)
	assert.NotZero(s.T(), event.Timestamp, "Timestamp should be set")
}
