package pusherror

type PushError struct {
	Msg string
}
func (e *PushError) Error() string {
	return e.Msg
}

