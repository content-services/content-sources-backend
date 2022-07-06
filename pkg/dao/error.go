package dao

import "fmt"

type Error struct {
	Message       string
	NotFound      bool
	BadValidation bool
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Wrap(msg string) {
	e.Message = fmt.Sprintf("%s: %v", msg, e.Error())
}
