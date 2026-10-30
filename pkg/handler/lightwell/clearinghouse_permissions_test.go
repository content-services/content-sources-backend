package lightwell

import (
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type clearinghouseFlags struct {
	canDelete, canDownload, canAddReproducers, canSubmit, canProcess bool
}

func TestClearinghouseCallerPermissions(t *testing.T) {
	saved := config.Get().Clients.Lightwell.ClearinghouseMapping
	t.Cleanup(func() {
		config.Get().Clients.Lightwell.ClearinghouseMapping = saved
	})
	config.Get().Clients.Lightwell.ClearinghouseMapping = config.ParseClearinghouseMappings(
		`{"CidOrgId":{"CID-1":"org"},"CidTam":{"CID-1":["reader","author"]}}`,
	)

	denied := errors.New("denied")
	author := clearinghouseCaller{orgID: "org", username: "author", readErr: denied}
	orgMember := clearinghouseCaller{orgID: "org", username: "member", readErr: denied}
	otherOrg := clearinghouseCaller{orgID: "other-org", username: "author", readErr: denied}
	reader := clearinghouseCaller{orgID: "other-org", username: "reader", featureErr: denied}
	authorReader := clearinghouseCaller{orgID: "org", username: "author"}
	noFeatures := clearinghouseCaller{orgID: "org", username: "author", featureErr: denied, readErr: denied}
	noUsername := clearinghouseCaller{orgID: "org", usernameErr: denied}

	pending := api.ClearinghouseSubmissionStatusPending
	submitted := api.ClearinghouseSubmissionStatusSubmitted
	processed := api.ClearinghouseSubmissionStatusProcessed

	cases := []struct {
		name   string
		caller clearinghouseCaller
		status string
		want   clearinghouseFlags
	}{
		{"author pending", author, pending, clearinghouseFlags{canDelete: true, canAddReproducers: true, canSubmit: true}},
		{"author submitted", author, submitted, clearinghouseFlags{}},
		{"author processed", author, processed, clearinghouseFlags{}},
		{"org member pending", orgMember, pending, clearinghouseFlags{canAddReproducers: true}},
		{"org member submitted", orgMember, submitted, clearinghouseFlags{}},
		{"other org pending", otherOrg, pending, clearinghouseFlags{}},
		{"reader pending", reader, pending, clearinghouseFlags{}},
		{"reader submitted", reader, submitted, clearinghouseFlags{canDownload: true, canProcess: true}},
		{"reader processed", reader, processed, clearinghouseFlags{canDownload: true}},
		{"author with read pending", authorReader, pending, clearinghouseFlags{canDelete: true, canAddReproducers: true, canSubmit: true}},
		{"author with read processed", authorReader, processed, clearinghouseFlags{canDownload: true}},
		{"author with read submitted", authorReader, submitted, clearinghouseFlags{canDownload: true, canProcess: true}},
		{"no features pending", noFeatures, pending, clearinghouseFlags{}},
		{"no username pending", noUsername, pending, clearinghouseFlags{canAddReproducers: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.New()
			upload := dao.ClearinghouseUpload{UUID: id, OrgID: "org", Username: "author", Filename: "submission.json", Status: tc.status}
			files := []dao.ClearinghouseUploadFile{
				{UUID: uuid.New(), UploadUUID: id, Filename: "submission.json", Submission: true},
				{UUID: uuid.New(), UploadUUID: id, Filename: "reproducer.py"},
			}

			got := tc.caller.submission(upload, files)
			assert.Equal(t, tc.want, clearinghouseFlags{
				canDelete:         got.CanDelete,
				canDownload:       got.CanDownload,
				canAddReproducers: got.CanAddReproducers,
				canSubmit:         got.CanSubmit,
				canProcess:        got.CanProcess,
			})
			assert.Len(t, got.Files, len(files))
			for _, file := range got.Files {
				assert.Equal(t, tc.want.canDelete, file.CanDelete, file.Filename)
				assert.Equal(t, tc.want.canDownload, file.CanDownload, file.Filename)
			}

			// The flags must agree with the checks the endpoints enforce.
			assert.Equal(t, got.CanDelete, tc.caller.checkDeleteSubmission(upload) == nil)
			assert.Equal(t, got.CanDelete, tc.caller.checkDeleteFile(upload) == nil)
			assert.Equal(t, got.CanDownload, tc.caller.checkDownload(upload) == nil)
			assert.Equal(t, got.CanAddReproducers, tc.caller.checkAddFiles(upload) == nil)
			assert.Equal(t, got.CanSubmit, tc.caller.checkSubmit(upload) == nil)
			assert.Equal(t, got.CanProcess, tc.caller.checkProcess(upload) == nil)
		})
	}
}
