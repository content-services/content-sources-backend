package errors

import (
	"errors"
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
		return fmt.Sprintf("%v", e.Err.Error())
	}
}

func (e *DaoError) Unwrap() error {
	return errors.Unwrap(e.Err)
}

func (e *DaoError) Wrap(err error) {
	e.Err = fmt.Errorf("%s: %w", e.Message, err)
}
