package adapter

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/content-services/content-sources-backend/pkg/event/message"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewKafka(t *testing.T) {
	result := NewKafkaHeaders()
	assert.IsType(t, KafkaAdapter{}, result)
}

func TestGetEchoHeader(t *testing.T) {
	type TestCaseGiven struct {
		Ctx     echo.Context
		Key     string
		Default []string
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected []string
	}
	testCases := []TestCase{
		{
			Name: "Return default value",
			Given: TestCaseGiven{
				Ctx: echo.New().NewContext(
					&http.Request{
						Header: map[string][]string{},
					},
					&echo.Response{},
				),
				Key:     "my-key",
				Default: []string{"my-default-value"},
			},
			Expected: []string{"my-default-value"},
		},
		{
			Name: "Return the value of the key",
			Given: TestCaseGiven{
				Ctx: echo.New().NewContext(
					&http.Request{
						Header: map[string][]string{
							"my-key": {
								"no-my-default-key",
							},
						},
					},
					&echo.Response{},
				),
				Key:     "my-key",
				Default: []string{"my-default-value"},
			},
			Expected: []string{"no-my-default-key"},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result := getEchoHeader(testCase.Given.Ctx, testCase.Given.Key, testCase.Given.Default)
		assert.Equal(t, testCase.Expected, result)
	}
}

func TestFromEchoContext(t *testing.T) {
	type TestCaseGiven struct {
		Ctx   echo.Context
		Event string
	}
	type TestCaseExpected struct {
		Headers []kafka.Header
		err     error
	}
	type TestCase struct {
		Name     string
		Given    TestCaseGiven
		Expected TestCaseExpected
	}
	testCases := []TestCase{
		{
			Name: "Error when ctx is nil",
			Given: TestCaseGiven{
				Ctx:   nil,
				Event: "",
			},
			Expected: TestCaseExpected{
				err: fmt.Errorf("ctx cannot be nil"),
			},
		},
		{
			Name: "Error when Event is empty string",
			Given: TestCaseGiven{
				Ctx: echo.New().NewContext(
					&http.Request{
						Header: map[string][]string{},
					},
					&echo.Response{},
				),
				Event: "",
			},
			Expected: TestCaseExpected{
				err: fmt.Errorf("event cannot be an empty string"),
			},
		},
		{
			Name: "Error when x-rh-identity is not present",
			Given: TestCaseGiven{
				Ctx: echo.New().NewContext(
					&http.Request{
						Header: map[string][]string{},
					},
					&echo.Response{},
				),
				Event: message.HdrTypeIntrospect,
			},
			Expected: TestCaseExpected{
				err: fmt.Errorf("expected a value for '%s' http header", string(message.HdrXRhIdentity)),
			},
		},
		// {
		// 	Name: "Success but a random string is generated for x-rh-insihgts-request-id header",
		// 	Given: TestCaseGiven{
		// 		Ctx: echo.New().NewContext(
		// 			&http.Request{
		// 				Header: map[string][]string{
		// 					// Generated with: ./scripts/header.sh 999999
		// 					string(message.HdrXRhIdentity): {"eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiI5OTk5OTkifX19Cg=="},
		// 				},
		// 			},
		// 			&echo.Response{},
		// 		),
		// 		Event: message.HdrTypeIntrospect,
		// 	},
		// 	Expected: TestCaseExpected{
		// 		err: nil,
		// 	},
		// },
		{
			Name: "Success transformation",
			Given: TestCaseGiven{
				Ctx: echo.New().NewContext(
					&http.Request{
						Header: map[string][]string{
							// Generated with: ./scripts/header.sh 999999
							string(message.HdrXRhIdentity):          {"eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiI5OTk5OTkifX19Cg=="},
							string(message.HdrXRhInsightsRequestId): {"XBlIjoiQXNzb2NpYXRlIiwiYWNjb3V"},
						},
					},
					&echo.Response{},
				),
				Event: message.HdrTypeIntrospect,
			},
			Expected: TestCaseExpected{
				err: nil,
				Headers: []kafka.Header{
					{
						Key:   "Type",
						Value: []byte(message.HdrTypeIntrospect),
					},
					{
						Key:   string(message.HdrXRhIdentity),
						Value: []byte("eyJpZGVudGl0eSI6eyJ0eXBlIjoiQXNzb2NpYXRlIiwiYWNjb3VudF9udW1iZXIiOiIiLCJpbnRlcm5hbCI6eyJvcmdfaWQiOiI5OTk5OTkifX19Cg=="),
					},
					{
						Key:   string(message.HdrXRhInsightsRequestId),
						Value: []byte("XBlIjoiQXNzb2NpYXRlIiwiYWNjb3V"),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		result, err := NewKafkaHeaders().FromEchoContext(testCase.Given.Ctx, testCase.Given.Event)
		if testCase.Expected.err != nil {
			require.Error(t, err)
			assert.Equal(t, testCase.Expected.err.Error(), err.Error())
		} else {
			assert.NoError(t, err)
			assert.Equal(t, testCase.Expected.Headers, result)
		}
	}
}
