package commands

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/content-services/content-sources-backend/pkg/external_repos"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const testRepoConfigUUID = "test-repo-config-uuid"
const testRepoName = "lightwell/java/remediated"

const testManifest = "advisory-1.json,checksum-aaa,1500\nadvisory-2.json,checksum-bbb,2500\n"

const testOSVAdvisory1 = `{
	"id": "ADV-001",
	"details": "First advisory",
	"severity": [{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N"}],
	"references": [{"url": "https://example.com/1", "type": "WEB"}],
	"affected": [{"package": {"name": "com.example:lib-a", "ecosystem": "Maven"}, "ranges": [{"type": "ECOSYSTEM", "events": [{"introduced": "0"}, {"fixed": "1.0.1"}]}]}]
}`

const testOSVAdvisory2 = `{
	"id": "ADV-002",
	"details": "Second advisory",
	"affected": [{"package": {"name": "com.example:lib-b", "ecosystem": "Maven"}, "ranges": [{"type": "ECOSYSTEM", "events": [{"introduced": "0"}, {"fixed": "2.0.0"}]}]}]
}`

func setupTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	origOrigin := config.Get().Clients.Pulp.ContentOrigin
	origPrefix := config.Get().Clients.Pulp.ContentPathPrefix
	config.Get().Clients.Pulp.ContentOrigin = server.URL
	config.Get().Clients.Pulp.ContentPathPrefix = ""
	t.Cleanup(func() {
		config.Get().Clients.Pulp.ContentOrigin = origOrigin
		config.Get().Clients.Pulp.ContentPathPrefix = origPrefix
	})

	return server
}

func mockRepoConfigFetch(mockDao *dao.MockDaoRegistry) {
	mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigByName",
		mock.Anything,
		config.LightwellOrg,
		testRepoName,
	).Return(api.RepositoryResponse{UUID: testRepoConfigUUID}, nil)
}

func testEntry() external_repos.LightwellAllowlistEntry {
	return external_repos.LightwellAllowlistEntry{
		Name:    testRepoName,
		OsvPath: "lightwell/osv/java/remediated",
	}
}

func TestProcessOsvForEntry_Success(t *testing.T) {
	// Serve two OSV advisory files from a test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/lightwell/osv/java/remediated/PULP_MANIFEST", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testManifest)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-1.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testOSVAdvisory1)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-2.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testOSVAdvisory2)
	})

	server := setupTestServer(t, mux)
	mockDao := dao.GetMockDaoRegistry(t)
	mockRepoConfigFetch(mockDao)

	// Phase 1: Sync advisories from OSV files into the database
	mockDao.LightwellAdvisory.On("ListByRepository", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellAdvisoryInput{}, nil)
	mockDao.LightwellAdvisory.On("SyncForRepository", mock.Anything, testRepoConfigUUID, testRepoName, mock.MatchedBy(func(advisories []dao.LightwellAdvisoryInput) bool {
		return len(advisories) == 2
	})).Return(nil)

	// Phase 2: After sync, sendAdvisoryNotifications queries for unnotified advisories
	// and opted-in orgs, then marks them as notified
	unnotified := []dao.LightwellNotificationData{
		{PackageName: "com.example:lib-a", AdvisoryID: "ADV-001", Severity: "9.8", FixedVersions: []string{"1.0.1"}},
		{PackageName: "com.example:lib-b", AdvisoryID: "ADV-002", Severity: "7.5", FixedVersions: []string{"2.0.0"}},
	}
	mockDao.LightwellAdvisory.On("ListUnnotifiedAdvisories", mock.Anything, testRepoConfigUUID).Return(unnotified, nil)
	mockDao.UserPreference.On("ListDistinctOrgsByPreference", mock.Anything,
		models.UserPreferenceLightwellNotificationEnabled, "true",
	).Return([]string{"org-1", "org-2"}, nil)
	mockDao.LightwellAdvisory.On("MarkAsNotified", mock.Anything, testRepoConfigUUID, unnotified).Return(nil)

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), false)
	assert.NoError(t, err)

	// Verify advisories were synced with correct data
	mockDao.LightwellAdvisory.AssertCalled(t, "SyncForRepository", mock.Anything, testRepoConfigUUID, testRepoName, mock.MatchedBy(func(advisories []dao.LightwellAdvisoryInput) bool {
		if len(advisories) != 2 {
			return false
		}
		return advisories[0].AdvisoryID == "ADV-001" &&
			advisories[0].PackageName == "com.example:lib-a" &&
			len(advisories[0].FixedVersions) == 1 && advisories[0].FixedVersions[0] == "1.0.1" &&
			advisories[0].Checksum == "checksum-aaa" &&
			advisories[1].AdvisoryID == "ADV-002" &&
			advisories[1].Checksum == "checksum-bbb"
	}))

	// Verify the notification flow ran: queried unnotified, queried opted-in orgs, marked as notified
	mockDao.LightwellAdvisory.AssertCalled(t, "ListUnnotifiedAdvisories", mock.Anything, testRepoConfigUUID)
	mockDao.UserPreference.AssertCalled(t, "ListDistinctOrgsByPreference", mock.Anything,
		models.UserPreferenceLightwellNotificationEnabled, "true")
	mockDao.LightwellAdvisory.AssertCalled(t, "MarkAsNotified", mock.Anything, testRepoConfigUUID, unnotified)
}

func TestProcessOsvForEntry_SkipsExistingByChecksum(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lightwell/osv/java/remediated/PULP_MANIFEST", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testManifest)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-2.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testOSVAdvisory2)
	})

	server := setupTestServer(t, mux)
	mockDao := dao.GetMockDaoRegistry(t)
	mockRepoConfigFetch(mockDao)

	existingAdvisory := dao.LightwellAdvisoryInput{
		AdvisoryID: "ADV-001",
		Checksum:   "checksum-aaa",
	}
	mockDao.LightwellAdvisory.On("ListByRepository", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellAdvisoryInput{existingAdvisory}, nil)
	mockDao.LightwellAdvisory.On("SyncForRepository", mock.Anything, testRepoConfigUUID, testRepoName, mock.MatchedBy(func(advisories []dao.LightwellAdvisoryInput) bool {
		return len(advisories) == 2
	})).Return(nil)
	mockDao.LightwellAdvisory.On("ListUnnotifiedAdvisories", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellNotificationData{}, nil)

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), false)
	assert.NoError(t, err)
}

func TestProcessOsvForEntry_ForceRefetchesAll(t *testing.T) {
	advisory1Fetched := false
	mux := http.NewServeMux()
	mux.HandleFunc("/lightwell/osv/java/remediated/PULP_MANIFEST", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testManifest)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-1.json", func(w http.ResponseWriter, r *http.Request) {
		advisory1Fetched = true
		fmt.Fprint(w, testOSVAdvisory1)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-2.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testOSVAdvisory2)
	})

	server := setupTestServer(t, mux)
	mockDao := dao.GetMockDaoRegistry(t)
	mockRepoConfigFetch(mockDao)

	existingAdvisory := dao.LightwellAdvisoryInput{
		AdvisoryID: "ADV-001",
		Checksum:   "checksum-aaa",
	}
	mockDao.LightwellAdvisory.On("ListByRepository", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellAdvisoryInput{existingAdvisory}, nil)
	mockDao.LightwellAdvisory.On("SyncForRepository", mock.Anything, testRepoConfigUUID, testRepoName, mock.MatchedBy(func(advisories []dao.LightwellAdvisoryInput) bool {
		return len(advisories) == 2
	})).Return(nil)
	mockDao.LightwellAdvisory.On("ListUnnotifiedAdvisories", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellNotificationData{}, nil)

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), true)
	assert.NoError(t, err)
	assert.True(t, advisory1Fetched, "advisory-1.json should be re-fetched when force is true")
}

func TestProcessOsvForEntry_RepoConfigNotFound(t *testing.T) {
	server := setupTestServer(t, http.NewServeMux())
	mockDao := dao.GetMockDaoRegistry(t)

	mockDao.RepositoryConfig.On("InternalOnly_FetchRepoConfigByName", mock.Anything, mock.Anything, mock.Anything).
		Return(api.RepositoryResponse{}, fmt.Errorf("Could not find repository with name %s", testRepoName))

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error finding repository")
}

func TestProcessOsvForEntry_ManifestFetchError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lightwell/osv/java/remediated/PULP_MANIFEST", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := setupTestServer(t, mux)
	mockDao := dao.GetMockDaoRegistry(t)
	mockRepoConfigFetch(mockDao)

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error fetching manifest")
}

func TestProcessOsvForEntry_SkipsFailedOSVFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lightwell/osv/java/remediated/PULP_MANIFEST", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testManifest)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-1.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/lightwell/osv/java/remediated/advisory-2.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testOSVAdvisory2)
	})

	server := setupTestServer(t, mux)
	mockDao := dao.GetMockDaoRegistry(t)
	mockRepoConfigFetch(mockDao)
	mockDao.LightwellAdvisory.On("ListByRepository", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellAdvisoryInput{}, nil)
	mockDao.LightwellAdvisory.On("SyncForRepository", mock.Anything, testRepoConfigUUID, testRepoName, mock.MatchedBy(func(advisories []dao.LightwellAdvisoryInput) bool {
		return len(advisories) == 1 && advisories[0].AdvisoryID == "ADV-002"
	})).Return(nil)
	mockDao.LightwellAdvisory.On("ListUnnotifiedAdvisories", mock.Anything, testRepoConfigUUID).Return([]dao.LightwellNotificationData{}, nil)

	err := processOSVForEntry(context.Background(), mockDao.ToDaoRegistry(), server.Client(), testEntry(), false)
	assert.NoError(t, err)
}
