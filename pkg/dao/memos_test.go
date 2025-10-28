package dao

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// Helper function to compare JSON content semantically
func assertJSONEqual(t *testing.T, expected, actual json.RawMessage) {
	var expectedObj, actualObj interface{}
	err := json.Unmarshal(expected, &expectedObj)
	assert.NoError(t, err)
	err = json.Unmarshal(actual, &actualObj)
	assert.NoError(t, err)
	assert.Equal(t, expectedObj, actualObj)
}

type MemoSuite struct {
	*DaoSuite
}

func TestMemoSuite(t *testing.T) {
	m := DaoSuite{}
	r := MemoSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (ms *MemoSuite) TestRead() {
	md := memoDaoImpl{db: ms.tx}
	key := "test-key"

	// Test reading non-existent memo
	memo, err := md.Read(context.Background(), key)
	assert.NoError(ms.T(), err)
	assert.Nil(ms.T(), memo)

	// Create a memo first
	testData := json.RawMessage(`{"message": "test data", "count": 42}`)
	createdMemo, err := md.Write(context.Background(), key, testData)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), createdMemo)
	assert.Equal(ms.T(), key, createdMemo.Key)
	assertJSONEqual(ms.T(), testData, createdMemo.Memo)

	// Now test reading the existing memo
	readMemo, err := md.Read(context.Background(), key)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), readMemo)
	assert.Equal(ms.T(), key, readMemo.Key)
	assertJSONEqual(ms.T(), testData, readMemo.Memo)
	assert.Equal(ms.T(), createdMemo.UUID, readMemo.UUID)
}

func (ms *MemoSuite) TestWrite() {
	md := memoDaoImpl{db: ms.tx}
	key := "write-test-key"
	testData := json.RawMessage(`{"action": "create", "timestamp": "2024-01-01T00:00:00Z"}`)

	// Test creating a new memo
	memo, err := md.Write(context.Background(), key, testData)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), memo)
	assert.Equal(ms.T(), key, memo.Key)
	assertJSONEqual(ms.T(), testData, memo.Memo)
	assert.NotEmpty(ms.T(), memo.UUID)

	// Test updating an existing memo
	updatedData := json.RawMessage(`{"action": "update", "timestamp": "2024-01-02T00:00:00Z", "version": 2}`)
	updatedMemo, err := md.Write(context.Background(), key, updatedData)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), updatedMemo)
	assert.Equal(ms.T(), key, updatedMemo.Key)
	assertJSONEqual(ms.T(), updatedData, updatedMemo.Memo)
	assert.Equal(ms.T(), memo.UUID, updatedMemo.UUID) // UUID should remain the same
}

func (ms *MemoSuite) TestWriteEmptyData() {
	md := memoDaoImpl{db: ms.tx}
	key := "empty-data-key"
	emptyData := json.RawMessage(`{}`)

	memo, err := md.Write(context.Background(), key, emptyData)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), memo)
	assert.Equal(ms.T(), key, memo.Key)
	assertJSONEqual(ms.T(), emptyData, memo.Memo)
}

func (ms *MemoSuite) TestWriteComplexData() {
	md := memoDaoImpl{db: ms.tx}
	key := "complex-data-key"
	complexData := json.RawMessage(`{
		"user": {
			"id": 123,
			"name": "John Doe",
			"email": "john@example.com"
		},
		"settings": {
			"theme": "dark",
			"notifications": true,
			"preferences": ["email", "sms"]
		},
		"metadata": {
			"created_by": "system",
			"tags": ["important", "user-config"]
		}
	}`)

	memo, err := md.Write(context.Background(), key, complexData)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), memo)
	assert.Equal(ms.T(), key, memo.Key)
	assertJSONEqual(ms.T(), complexData, memo.Memo)

	// Verify we can read it back
	readMemo, err := md.Read(context.Background(), key)
	assert.NoError(ms.T(), err)
	assert.NotNil(ms.T(), readMemo)
	assertJSONEqual(ms.T(), complexData, readMemo.Memo)
}

func (ms *MemoSuite) TestGetLastSuccessfulPulpLogDate_NoMemo() {
	md := memoDaoImpl{db: ms.tx}
	ctx := context.Background()

	// Ensure no memo exists by deleting any existing one
	err := ms.tx.WithContext(ctx).Where("key = ?", config.MemoPulpLastSuccessfulPulpLogParse).Delete(&models.Memo{}).Error
	assert.NoError(ms.T(), err)

	// Test when no memo exists - should return yesterday's date
	date, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.Error(ms.T(), err)
	assert.NotNil(ms.T(), date)
}

func (ms *MemoSuite) TestGetLastSuccessfulPulpLogDate_WithValidMemo() {
	md := memoDaoImpl{db: ms.tx}
	ctx := context.Background()

	// Create a memo with a valid date using the new struct format
	testDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	dateStr := testDate.Format("2006-01-02")
	dateMemo := struct {
		Date string `json:"date"`
	}{Date: dateStr}
	dateBytes, err := json.Marshal(dateMemo)
	assert.NoError(ms.T(), err)

	_, err = md.Write(ctx, config.MemoPulpLastSuccessfulPulpLogParse, dateBytes)
	assert.NoError(ms.T(), err)

	// Test retrieving the date
	retrievedDate, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.NoError(ms.T(), err)
	assert.Equal(ms.T(), testDate.Year(), retrievedDate.Year())
	assert.Equal(ms.T(), testDate.Month(), retrievedDate.Month())
	assert.Equal(ms.T(), testDate.Day(), retrievedDate.Day())
}

func (ms *MemoSuite) TestGetLastSuccessfulPulpLogDate_WithInvalidDateFormat() {
	md := memoDaoImpl{db: ms.tx}
	ctx := context.Background()

	// Create a memo with invalid date format
	invalidData := json.RawMessage(`"2024-13-45"`) // Invalid month and day
	_, err := md.Write(ctx, config.MemoPulpLastSuccessfulPulpLogParse, invalidData)
	assert.NoError(ms.T(), err)

	// Test retrieving the date - should return error and yesterday's date
	retrievedDate, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.Error(ms.T(), err) // Should error due to invalid date format
	assert.NotNil(ms.T(), retrievedDate)
}

func (ms *MemoSuite) TestSaveLastSuccessfulPulpLogDate() {
	md := memoDaoImpl{db: ms.tx}
	ctx := context.Background()

	// Test saving a valid date
	testDate := time.Date(2024, 6, 20, 14, 30, 0, 0, time.UTC)
	err := md.SaveLastSuccessfulPulpLogDate(ctx, testDate)
	assert.NoError(ms.T(), err)

	// Verify it was saved correctly by reading it back
	retrievedDate, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.NoError(ms.T(), err)
	assert.Equal(ms.T(), testDate.Year(), retrievedDate.Year())
	assert.Equal(ms.T(), testDate.Month(), retrievedDate.Month())
	assert.Equal(ms.T(), testDate.Day(), retrievedDate.Day())
	// Time should be normalized to midnight (date format only stores date, not time)
	assert.Equal(ms.T(), 0, retrievedDate.Hour())
	assert.Equal(ms.T(), 0, retrievedDate.Minute())
}

func (ms *MemoSuite) TestSaveLastSuccessfulPulpLogDate_UpdateExisting() {
	md := memoDaoImpl{db: ms.tx}
	ctx := context.Background()

	// Save initial date
	initialDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	err := md.SaveLastSuccessfulPulpLogDate(ctx, initialDate)
	assert.NoError(ms.T(), err)

	// Verify initial date
	retrievedDate, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.NoError(ms.T(), err)
	assert.Equal(ms.T(), initialDate.Year(), retrievedDate.Year())
	assert.Equal(ms.T(), initialDate.Month(), retrievedDate.Month())
	assert.Equal(ms.T(), initialDate.Day(), retrievedDate.Day())

	// Update with a new date
	newDate := time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)
	err = md.SaveLastSuccessfulPulpLogDate(ctx, newDate)
	assert.NoError(ms.T(), err)

	// Verify the date was updated
	updatedDate, err := md.GetLastSuccessfulPulpLogDate(ctx)
	assert.NoError(ms.T(), err)
	assert.Equal(ms.T(), newDate.Year(), updatedDate.Year())
	assert.Equal(ms.T(), newDate.Month(), updatedDate.Month())
	assert.Equal(ms.T(), newDate.Day(), updatedDate.Day())
	assert.NotEqual(ms.T(), initialDate.Day(), updatedDate.Day())
}
