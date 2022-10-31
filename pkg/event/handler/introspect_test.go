package handler

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func getDatabase() (*gorm.DB, sqlmock.Sqlmock) {
	var (
		db     *sql.DB
		gormDb *gorm.DB
		mock   sqlmock.Sqlmock
		err    error
	)

	db, mock, _ = sqlmock.New()
	dialector := postgres.New(postgres.Config{
		DSN:                  "sqlmock_db_0",
		DriverName:           "postgres",
		Conn:                 db,
		PreferSimpleProtocol: true,
	})
	gormDb, err = gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		panic(err)
	}
	return gormDb, mock
}

func TestNewIntrospectHandler(t *testing.T) {
	var (
		result *IntrospectHandler
		gormDb *gorm.DB
	)

	// When nil is passed it returns nil
	result = NewIntrospectHandler(nil)
	assert.Nil(t, result)

	// https://github.com/go-gorm/gorm/issues/3565
	// When a gormDb connector is passed
	gormDb, _ = getDatabase()

	result = NewIntrospectHandler(gormDb)
	assert.NotNil(t, result)
}

func TestIntrospectHandlerOnMessage(t *testing.T) {
	var err error

	type TestCase struct {
		Name     string
		Given    *kafka.Message
		Expected error
	}

	testCases := []TestCase{
		{
			Name:     "Error when unmarshall fails",
			Given:    &kafka.Message{},
			Expected: fmt.Errorf("Error deserializing payload:"),
		},
		{
			Name: "Error when url is invalid",
			Given: &kafka.Message{
				Value: []byte(`{"url":"","uuid":"6742a4c0-0fe5-4abc-9037-bfbe57d3bcb5"}`),
			},
			Expected: fmt.Errorf("Key: '' Error:Field validation for '' failed on the 'required' tag"),
		},
	}

	for _, testCase := range testCases {
		t.Log(testCase.Name)
		gormDb, _ := getDatabase()
		require.NotNil(t, gormDb)
		handler := NewIntrospectHandler(gormDb)
		require.NotNil(t, handler)
		err = handler.OnMessage(testCase.Given)
		if testCase.Expected != nil {
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.Expected.Error())
		} else {
			assert.NoError(t, err)
		}
	}
}
