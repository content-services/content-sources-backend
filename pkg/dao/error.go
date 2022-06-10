package dao

import (
	"github.com/jackc/pgconn"
)

type Error struct {
	Message       string
	NotFound      bool
	BadValidation bool
}

func (e *Error) Error() string {
	return e.Message
}

func isUniqueViolation(err error) bool {
	pgError, ok := err.(*pgconn.PgError)
	if ok {
		if pgError.Code == "23505" {
			return true
		}
	}
	return false
}
