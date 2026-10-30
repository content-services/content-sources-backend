package lightwell

import (
	"net/http"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/dao"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/handler"
	"github.com/labstack/echo/v4"
)

// clearinghouseCaller holds everything about the requesting identity that the
// clearinghouse permission rules depend on. Every clearinghouse handler checks
// access through its check* methods, and the can_* flags returned on
// submissions are derived from the same methods, so each rule lives in one place.
type clearinghouseCaller struct {
	orgID       string
	username    string
	usernameErr error
	// featureErr is nil when the caller has the Lightwell clearinghouse (customer) feature.
	featureErr error
	// readErr is nil when the caller has the Lightwell clearinghouse read feature.
	readErr error
}

func newClearinghouseCaller(c echo.Context) clearinghouseCaller {
	ctx := c.Request().Context()
	_, orgID := handler.GetAccountIdOrgId(c)
	username, usernameErr := clearinghouseUsername(c)
	return clearinghouseCaller{
		orgID:       orgID,
		username:    username,
		usernameErr: usernameErr,
		featureErr:  handler.CheckLightwellClearinghouseAccessible(ctx),
		readErr:     handler.CheckLightwellClearinghouseReadAccessible(ctx),
	}
}

// listParams limits list queries to the caller's org, or, for a read caller, to
// the orgs whose CID lists their username.
func (p clearinghouseCaller) listParams(page clearinghousePageParams) (dao.ListClearinghouseUploadsParams, error) {
	if p.readErr == nil {
		if p.usernameErr != nil {
			return dao.ListClearinghouseUploadsParams{}, p.usernameErr
		}
		orgIDs := config.Get().Clients.Lightwell.ClearinghouseMapping.OrgIDsForUser(p.username)
		if orgIDs == nil {
			orgIDs = []string{}
		}
		return dao.ListClearinghouseUploadsParams{
			OrgIDs:         orgIDs,
			RestrictOrgIDs: true,
			Limit:          page.limit,
			Offset:         page.offset,
		}, nil
	}
	if p.featureErr != nil {
		return dao.ListClearinghouseUploadsParams{}, ce.NewErrorResponse(http.StatusBadRequest, "Cannot access clearinghouse submissions",
			"Neither the user nor account is allowed.")
	}
	orgID := p.orgID
	return dao.ListClearinghouseUploadsParams{
		OrgID:  &orgID,
		Limit:  page.limit,
		Offset: page.offset,
	}, nil
}

// allowsOrg reports whether this caller may see a submission in orgID.
func (p clearinghouseCaller) allowsOrg(orgID string) (bool, error) {
	if p.readErr == nil {
		if p.usernameErr != nil {
			return false, p.usernameErr
		}
		return config.Get().Clients.Lightwell.ClearinghouseMapping.Allows(p.username, orgID), nil
	}
	if p.featureErr != nil {
		return false, ce.NewErrorResponse(http.StatusBadRequest, "Cannot access clearinghouse submissions",
			"Neither the user nor account is allowed.")
	}
	return p.orgID == orgID, nil
}

// checkAddFiles allows any member of the submission's org with the clearinghouse
// feature to add files while the submission is pending.
func (p clearinghouseCaller) checkAddFiles(upload dao.ClearinghouseUpload) error {
	if p.featureErr != nil {
		return p.featureErr
	}
	if upload.OrgID != p.orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
	}
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot add files to clearinghouse submission", "Submission has been submitted and can no longer be changed")
	}
	return nil
}

// checkDeleteSubmission allows only the author to delete a pending submission.
func (p clearinghouseCaller) checkDeleteSubmission(upload dao.ClearinghouseUpload) error {
	return p.checkAuthorPending(upload, "Cannot delete clearinghouse submission", "Only the user who created the submission can delete it")
}

// checkDeleteFile allows only the author to delete files from a pending submission.
func (p clearinghouseCaller) checkDeleteFile(upload dao.ClearinghouseUpload) error {
	return p.checkAuthorPending(upload, "Cannot delete clearinghouse submission file", "Only the user who created the submission can delete files")
}

// checkSubmit allows only the author to submit a pending submission.
func (p clearinghouseCaller) checkSubmit(upload dao.ClearinghouseUpload) error {
	if p.featureErr != nil {
		return p.featureErr
	}
	if upload.OrgID != p.orgID {
		return ce.NewErrorResponse(http.StatusNotFound, "Error submitting clearinghouse submission", "Submission not found")
	}
	if p.usernameErr != nil {
		return p.usernameErr
	}
	if upload.Username != p.username {
		return ce.NewErrorResponse(http.StatusForbidden, "Cannot submit clearinghouse submission", "Only the user who created the submission can submit it")
	}
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot submit clearinghouse submission", "Submission has been submitted and can no longer be changed")
	}
	return nil
}

// checkProcess allows a read caller to process a submitted submission when their
// username is listed on that submission's CID.
func (p clearinghouseCaller) checkProcess(upload dao.ClearinghouseUpload) error {
	if p.readErr != nil {
		return p.readErr
	}
	if p.usernameErr != nil {
		return p.usernameErr
	}
	if !config.Get().Clients.Lightwell.ClearinghouseMapping.Allows(p.username, upload.OrgID) {
		return ce.NewErrorResponse(http.StatusNotFound, "Error processing clearinghouse submission", "Submission not found")
	}
	if upload.Status != api.ClearinghouseSubmissionStatusSubmitted {
		return ce.NewErrorResponse(http.StatusConflict, "Cannot process clearinghouse submission", clearinghouseProcessConflict(upload.Status))
	}
	return nil
}

// checkDownload allows a read caller to download a submitted or processed
// submission when their username is listed on its CID. Customers, including the
// author, cannot download files from a submission.
func (p clearinghouseCaller) checkDownload(upload dao.ClearinghouseUpload) error {
	if p.readErr == nil {
		if p.usernameErr != nil {
			return p.usernameErr
		}
		if !config.Get().Clients.Lightwell.ClearinghouseMapping.Allows(p.username, upload.OrgID) {
			return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
		}
		switch upload.Status {
		case api.ClearinghouseSubmissionStatusSubmitted, api.ClearinghouseSubmissionStatusProcessed:
			return nil
		}
		return ce.NewErrorResponse(http.StatusConflict, "Cannot download clearinghouse submission", clearinghouseDownloadConflict(upload.Status))
	}
	if p.featureErr != nil {
		return p.readErr
	}
	if upload.OrgID == p.orgID {
		return ce.NewErrorResponse(http.StatusForbidden, "Cannot download clearinghouse submission", "Only clearinghouse read users can download submission files")
	}
	return ce.NewErrorResponse(http.StatusNotFound, "Error fetching clearinghouse submission", "Submission not found")
}

func (p clearinghouseCaller) checkAuthorPending(upload dao.ClearinghouseUpload, title, forbidden string) error {
	if p.featureErr != nil {
		return p.featureErr
	}
	if upload.OrgID != p.orgID {
		return ce.NewErrorResponse(http.StatusNotFound, title, "Submission not found")
	}
	if p.usernameErr != nil {
		return p.usernameErr
	}
	if upload.Username != p.username {
		return ce.NewErrorResponse(http.StatusForbidden, title, forbidden)
	}
	if upload.Status != api.ClearinghouseSubmissionStatusPending {
		return ce.NewErrorResponse(http.StatusConflict, title, "Submission has been submitted and can no longer be changed")
	}
	return nil
}

func clearinghouseProcessConflict(status string) string {
	if status == api.ClearinghouseSubmissionStatusPending {
		return "Submission must be submitted before it can be processed"
	}
	if status == api.ClearinghouseSubmissionStatusProcessed {
		return "Submission has already been processed"
	}
	return "Submission cannot be processed"
}

func clearinghouseDownloadConflict(status string) string {
	if status == api.ClearinghouseSubmissionStatusPending {
		return "Submission must be submitted before it can be downloaded"
	}
	return "Submission cannot be downloaded"
}

// submission converts a stored upload into the API response, including the
// permission flags for this caller.
func (p clearinghouseCaller) submission(upload dao.ClearinghouseUpload, files []dao.ClearinghouseUploadFile) api.ClearinghouseSubmission {
	canDeleteFile := p.checkDeleteFile(upload) == nil
	canDownload := p.checkDownload(upload) == nil
	outFiles := make([]api.ClearinghouseSubmissionFile, 0, len(files))
	for _, file := range files {
		outFiles = append(outFiles, api.ClearinghouseSubmissionFile{
			UUID:        file.UUID.String(),
			Filename:    file.Filename,
			Username:    file.Username,
			Submission:  file.Submission,
			CreatedAt:   file.CreatedAt,
			CanDelete:   canDeleteFile,
			CanDownload: canDownload,
		})
	}
	var count *int
	if upload.VulnerabilityCount != nil {
		n := int(*upload.VulnerabilityCount)
		count = &n
	}
	return api.ClearinghouseSubmission{
		UUID:               upload.UUID.String(),
		OrgID:              upload.OrgID,
		Username:           upload.Username,
		Name:               upload.Name,
		CreatedAt:          upload.CreatedAt,
		SubmittedAt:        upload.SubmittedAt,
		SubmittedBy:        upload.SubmittedBy,
		ProcessedAt:        upload.ProcessedAt,
		ProcessedBy:        upload.ProcessedBy,
		VulnerabilityCount: count,
		Status:             upload.Status,
		ErrorMessage:       upload.ErrorMessage,
		CanDelete:          p.checkDeleteSubmission(upload) == nil,
		CanDownload:        canDownload,
		CanAddReproducers:  p.checkAddFiles(upload) == nil,
		CanSubmit:          p.checkSubmit(upload) == nil,
		CanProcess:         p.checkProcess(upload) == nil,
		Files:              outFiles,
	}
}

// submissionWithCID converts a stored upload into the list/get API response.
// The customer identifier is only included for callers with the Lightwell
// clearinghouse read feature.
func (p clearinghouseCaller) submissionWithCID(upload dao.ClearinghouseUpload, files []dao.ClearinghouseUploadFile) api.ClearinghouseSubmissionWithCID {
	cid := ""
	if p.readErr == nil {
		cid = config.Get().Clients.Lightwell.ClearinghouseMapping.CIDForOrg(upload.OrgID)
	}
	return api.ClearinghouseSubmissionWithCID{
		ClearinghouseSubmission: p.submission(upload, files),
		CID:                     cid,
	}
}
