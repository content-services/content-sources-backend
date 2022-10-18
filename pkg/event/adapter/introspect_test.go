package adapter

import (
	"fmt"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewIntrospect(t *testing.T) {
	result := NewIntrospect()
	assert.IsType(t, IntrospectRequest{}, result)
}

func TestFromRepositoryResponse(t *testing.T) {
	type TestCaseExpected struct {
		msg *message.IntrospectRequestMessage
		err error
	}
	type TestCase struct {
		Given    *api.RepositoryResponse
		Expected TestCaseExpected
	}
	testCases := []TestCase{
		{
			Given: nil,
			Expected: TestCaseExpected{
				err: fmt.Errorf("repositoryResponse cannot be nil"),
				msg: nil,
			},
		},
		{
			Given: &api.RepositoryResponse{
				UUID: "6742a4c0-0fe5-4abc-9037-bfbe57d3bcb5",
				URL:  "https://my-awesome-repository.test",
			},
			Expected: TestCaseExpected{
				err: nil,
				msg: &message.IntrospectRequestMessage{
					Uuid: "6742a4c0-0fe5-4abc-9037-bfbe57d3bcb5",
					Url:  "https://my-awesome-repository.test",
				},
			},
		},
	}

	for _, testCase := range testCases {
		result, err := NewIntrospect().FromRepositoryResponse(testCase.Given)
		if testCase.Expected.err != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.err.Error(), err.Error())
			assert.Equal(t, testCase.Expected.msg, result)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, testCase.Expected.msg, result)
		}
	}
}

func TestFromRepositoryBulkCreateResponse(t *testing.T) {
	type TestCaseExpected struct {
		msg *message.IntrospectRequestMessage
		err error
	}
	type TestCase struct {
		Given    *api.RepositoryBulkCreateResponse
		Expected TestCaseExpected
	}

	testCases := []TestCase{
		{
			Given: nil,
			Expected: TestCaseExpected{
				err: fmt.Errorf("repositoryBulkCreateResponse cannot be nil"),
				msg: nil,
			},
		},
		{
			Given: &api.RepositoryBulkCreateResponse{
				Repository: &api.RepositoryResponse{
					UUID: "6742a4c0-0fe5-4abc-9037-bfbe57d3bcb5",
					URL:  "https://my-awesome-repository.test",
				},
			},
			Expected: TestCaseExpected{
				err: nil,
				msg: &message.IntrospectRequestMessage{
					Uuid: "6742a4c0-0fe5-4abc-9037-bfbe57d3bcb5",
					Url:  "https://my-awesome-repository.test",
				},
			},
		},
	}

	for _, testCase := range testCases {
		result, err := NewIntrospect().FromRepositoryBulkCreateResponse(testCase.Given)
		if testCase.Expected.err != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.err.Error(), err.Error())
			assert.Equal(t, testCase.Expected.msg, result)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, testCase.Expected.msg, result)
		}
	}
}
