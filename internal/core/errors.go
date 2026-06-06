package core

type ErrorKind string

const (
	ErrorKindConfig ErrorKind = "config_error"
	ErrorKindStore  ErrorKind = "store_error"
	ErrorKindIndex  ErrorKind = "index_error"
	ErrorKindQuery  ErrorKind = "query_error"
)

type Error struct {
	Kind    ErrorKind
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return string(e.Kind) + ": " + e.Message
	}
	return string(e.Kind) + ": " + e.Message + ": " + e.Cause.Error()
}
