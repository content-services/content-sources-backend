package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearinghouseUploadDao(t *testing.T) {
	ctx := context.Background()
	uploads := GetDaoRegistry(db.DB).ClearinghouseUpload
	pool := db.LightwellPool()
	require.NotNil(t, pool)

	orgID := "clearinghouse-dao-" + uuid.NewString()
	uploadID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM clearinghouse_uploads WHERE uuid = $1", uploadID)
	})

	created, err := uploads.Create(ctx, CreateClearinghouseUploadParams{
		UUID:     uploadID,
		OrgID:    orgID,
		Username: "author",
		Name:     "My Submission",
		Filename: "submission.json",
		Status:   api.ClearinghouseSubmissionStatusPending,
	})
	require.NoError(t, err)
	assert.Equal(t, uploadID, created.UUID)
	assert.Equal(t, "My Submission", created.Name)
	assert.Equal(t, "submission.json", created.Filename)

	fileID := uuid.New()
	extraID := uuid.New()
	_, err = uploads.CreateFiles(ctx, []CreateClearinghouseUploadFileParams{
		{
			UUID:       fileID,
			UploadUUID: uploadID,
			Filename:   "vulnerabilities.json",
			Filepath:   "uploads/" + uploadID.String() + "/vulnerabilities.json",
			Username:   "author",
			Submission: true,
		},
		{
			UUID:       extraID,
			UploadUUID: uploadID,
			Filename:   "reproducer.py",
			Filepath:   "uploads/" + uploadID.String() + "/reproducer.py",
			Username:   "author",
		},
	}, nil)
	require.NoError(t, err)

	submissionFile, err := uploads.GetFileByName(ctx, uploadID, "vulnerabilities.json")
	require.NoError(t, err)
	assert.True(t, submissionFile.Submission)
	reproducerFile, err := uploads.GetFileByName(ctx, uploadID, "reproducer.py")
	require.NoError(t, err)
	assert.False(t, reproducerFile.Submission)

	_, err = uploads.CreateFiles(ctx, []CreateClearinghouseUploadFileParams{{
		UUID:       uuid.New(),
		UploadUUID: uploadID,
		Filename:   "reproducer.py",
		Filepath:   "uploads/" + uploadID.String() + "/reproducer.py",
		Username:   "author",
	}}, nil)
	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	assert.Equal(t, "23505", pgErr.Code)

	called := false
	err = uploads.DeleteFileIfUploading(ctx, uploadID, "reproducer.py", func(file ClearinghouseUploadFile) error {
		called = true
		assert.Equal(t, extraID, file.UUID)
		return errors.New("delete failed")
	})
	require.EqualError(t, err, "delete failed")
	assert.True(t, called)
	_, err = uploads.GetFileByName(ctx, uploadID, "reproducer.py")
	require.ErrorIs(t, err, pgx.ErrNoRows)

	removed := false
	err = uploads.DeleteIfUploading(ctx, uploadID, func(files []ClearinghouseUploadFile) error {
		removed = true
		require.Len(t, files, 1)
		assert.Equal(t, "vulnerabilities.json", files[0].Filename)
		return nil
	})
	require.NoError(t, err)
	assert.True(t, removed)
	_, err = uploads.Get(ctx, uploadID)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	uploadID = uuid.New()
	_, err = uploads.Create(ctx, CreateClearinghouseUploadParams{
		UUID:     uploadID,
		OrgID:    orgID,
		Username: "author",
		Filename: "vulnerabilities.json",
		Status:   api.ClearinghouseSubmissionStatusPending,
	})
	require.NoError(t, err)
	_, err = uploads.Submit(ctx, uploadID, "author")
	require.NoError(t, err)
	err = uploads.DeleteIfUploading(ctx, uploadID, func([]ClearinghouseUploadFile) error {
		t.Fatal("object delete must not run after the submission is closed")
		return nil
	})
	require.ErrorIs(t, err, ErrClearinghouseSubmissionClosed)
	_, err = uploads.Get(ctx, uploadID)
	require.NoError(t, err)

	closedID := uuid.New()
	_, err = uploads.Create(ctx, CreateClearinghouseUploadParams{
		UUID:     closedID,
		OrgID:    orgID,
		Username: "author",
		Filename: "closed.json",
		Status:   api.ClearinghouseSubmissionStatusSubmitted,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM clearinghouse_uploads WHERE uuid = $1", closedID)
	})
	_, err = uploads.CreateFiles(ctx, []CreateClearinghouseUploadFileParams{{
		UUID:       uuid.New(),
		UploadUUID: closedID,
		Filename:   "closed.json",
		Filepath:   "uploads/" + closedID.String() + "/closed.json",
		Username:   "author",
	}}, nil)
	require.ErrorIs(t, err, ErrClearinghouseSubmissionClosed)

	rollbackID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM clearinghouse_uploads WHERE uuid = $1", rollbackID)
	})
	stored := false
	_, _, err = uploads.CreateWithFiles(ctx, CreateClearinghouseUploadParams{
		UUID:     rollbackID,
		OrgID:    orgID,
		Username: "author",
		Filename: "rollback.json",
		Status:   api.ClearinghouseSubmissionStatusPending,
	}, []CreateClearinghouseUploadFileParams{{
		UUID:       uuid.New(),
		UploadUUID: rollbackID,
		Filename:   "rollback.json",
		Filepath:   "uploads/" + rollbackID.String() + "/rollback.json",
		Username:   "author",
	}}, func([]ClearinghouseUploadFile) error {
		stored = true
		_, getErr := uploads.Get(ctx, rollbackID)
		require.NoError(t, getErr)
		return errors.New("put failed")
	})
	require.EqualError(t, err, "put failed")
	assert.True(t, stored)
	_, err = uploads.Get(ctx, rollbackID)
	require.ErrorIs(t, err, pgx.ErrNoRows)

	keptID := uuid.New()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM clearinghouse_uploads WHERE uuid = $1", keptID)
	})
	_, err = uploads.Create(ctx, CreateClearinghouseUploadParams{
		UUID:     keptID,
		OrgID:    orgID,
		Username: "author",
		Filename: "kept.json",
		Status:   api.ClearinghouseSubmissionStatusPending,
	})
	require.NoError(t, err)
	extraFileID := uuid.New()
	_, err = uploads.CreateFiles(ctx, []CreateClearinghouseUploadFileParams{{
		UUID:       extraFileID,
		UploadUUID: keptID,
		Filename:   "extra.txt",
		Filepath:   "uploads/" + keptID.String() + "/extra.txt",
		Username:   "author",
	}}, func([]ClearinghouseUploadFile) error {
		_, getErr := uploads.GetFileByName(ctx, keptID, "extra.txt")
		require.NoError(t, getErr)
		return errors.New("put failed")
	})
	require.EqualError(t, err, "put failed")
	_, err = uploads.GetFileByName(ctx, keptID, "extra.txt")
	require.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = uploads.Get(ctx, keptID)
	require.NoError(t, err)
}
