//go:build windows

package npipe

// ErrClosed is the error returned by PipeListener.Accept when Close is called
// on the PipeListener.
var ErrClosed = PipeError{"Pipe has been closed.", false}

// PipeError is an error related to a call to a pipe
type PipeError struct {
	msg     string
	timeout bool
}

// Error implements the error interface
func (e PipeError) Error() string {
	return e.msg
}

// Timeout implements net.AddrError.Timeout()
func (e PipeError) Timeout() bool {
	return e.timeout
}

// Temporary implements net.AddrError.Temporary()
func (e PipeError) Temporary() bool {
	return false
}
