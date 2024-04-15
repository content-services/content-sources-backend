package errors

import (
	"fmt"
)

type DaoError struct {
	Err           error
	Message       string
	NotFound      bool
	BadValidation bool
}

func (e *DaoError) Error() string {
	if e.Err == nil {
		return e.Message
	} else {
		return fmt.Sprintf("%v: %v", e.Message, e.Err.Error())
	}
}

func (e *DaoError) Unwrap() error {
	return e.Err
}

func (e *DaoError) Wrap(err error) {
	e.Err = err
}
