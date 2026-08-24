package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CoverageUploadSuite struct {
	*ModelsSuite
}

func TestCoverageUploadSuite(t *testing.T) {
	m := ModelsSuite{}
	r := CoverageUploadSuite{&m}
	suite.Run(t, &r)
}

func (s *CoverageUploadSuite) TestCoverageUploadCreate() {
	tx := s.tx

	upload := CoverageUpload{
		Sha256:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SizeBytes: 1024,
	}
	err := tx.Create(&upload).Error
	assert.NoError(s.T(), err)

	readUpload := CoverageUpload{}
	err = tx.Where("uuid = ?", upload.UUID).First(&readUpload).Error
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), upload.StorageKey, readUpload.StorageKey)
	assert.Equal(s.T(), upload.Sha256, readUpload.Sha256)
	assert.Equal(s.T(), upload.SizeBytes, readUpload.SizeBytes)
}

func (s *CoverageUploadSuite) TestCoverageUploadValidations() {
	t := s.T()
	tx := s.tx

	spName := "testcoverageuploadvalidations"
	testSha256 := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	testSizeBytes := int64(1024)

	testCases := []struct {
		given    CoverageUpload
		expected string
	}{
		{
			given: CoverageUpload{
				StorageKey: "coverage-uploads/test",
				Sha256:     "",
				SizeBytes:  testSizeBytes,
			},
			expected: "Sha256 cannot be blank.",
		},
		{
			given: CoverageUpload{
				StorageKey: "coverage-uploads/test",
				Sha256:     testSha256,
				SizeBytes:  0,
			},
			expected: "Size bytes must be greater than zero.",
		},
		{
			given: CoverageUpload{
				StorageKey: "coverage-uploads/test",
				Sha256:     testSha256,
				SizeBytes:  -1,
			},
			expected: "Size bytes must be greater than zero.",
		},
	}

	tx.SavePoint(spName)
	for _, item := range testCases {
		err := tx.Create(&item.given).Error
		assert.Error(t, err)
		if err != nil {
			assert.Equal(t, item.expected, err.Error())
		}
		tx.RollbackTo(spName)
	}
}
