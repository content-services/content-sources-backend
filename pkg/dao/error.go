package dao

type Error struct {
	Message       string
	NotFound      bool
	BadValidation bool
}

func (e *Error) Error() string {
	return e.Message
}
