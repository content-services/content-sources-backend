package errors

import (
	"fmt"
)

type DaoError struct {
	Message       string
	NotFound      bool
	BadValidation bool
}

func (e DaoError) Error() string {
	return e.Message
}

func (e *DaoError) Wrap(msg string) {
	e.Message = fmt.Sprintf("%s: %v", msg, e.Error())
}
