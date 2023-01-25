//go:build windows

package npipe

import (
	// Standard
	"fmt"
	"io"
	"net"
	"time"

	// X Package
	"golang.org/x/sys/windows"
)

// PipeConn is the implementation of the net.Conn interface for named pipe connections.
type PipeConn struct {
	handle        windows.Handle // handle is a Windows handle to the named pipe
	addr          PipeAddr       // addr is the named pipe network (pipe) and address
	readDeadline  *time.Time     // readDeadline is the timeout deadline to read
	writeDeadline *time.Time     // writeDeadline is the timeout deadline to write
}

// completeRequest looks at iodata to see if a request is pending. If so, it waits for it to either complete or to
// abort due to hitting the specified deadline. Deadline may be set to nil to wait forever. If no request is pending,
// the content of iodata is returned.
func (c *PipeConn) completeRequest(data iodata, deadline *time.Time, overlapped *windows.Overlapped) (int, error) {
	if data.err == windows.ERROR_IO_INCOMPLETE || data.err == windows.ERROR_IO_PENDING {
		var timer <-chan time.Time
		if deadline != nil {
			if timeDiff := deadline.Sub(time.Now()); timeDiff > 0 {
				timer = time.After(timeDiff)
			}
		}
		done := make(chan iodata)
		go func() {
			n, err := waitForCompletion(c.handle, overlapped)
			done <- iodata{n, err}
		}()
		select {
		case data = <-done:
		case <-timer:
			windows.CancelIoEx(c.handle, overlapped)
			data = iodata{0, timeout(c.addr.String())}
		}
	}
	// Windows will produce ERROR_BROKEN_PIPE upon closing
	// a handle on the other end of a connection. Go RPC
	// expects an io.EOF error in this case.
	if data.err == windows.ERROR_BROKEN_PIPE {
		data.err = io.EOF
	}
	return int(data.n), data.err
}

// Read implements the net.Conn Read method.
func (c *PipeConn) Read(b []byte) (int, error) {
	// Use ReadFile() rather than Read() because the latter
	// contains a workaround that eats ERROR_BROKEN_PIPE.
	overlapped, err := newOverlapped()
	if err != nil {
		return 0, fmt.Errorf("npipe.PipeConn.Read(): %s", err)
	}
	defer windows.CloseHandle(overlapped.HEvent)
	var n uint32
	err = windows.ReadFile(c.handle, b, &n, overlapped)
	return c.completeRequest(iodata{n, err}, c.readDeadline, overlapped)
}

// Write implements the net.Conn Write method.
func (c *PipeConn) Write(b []byte) (int, error) {
	overlapped, err := newOverlapped()
	if err != nil {
		return 0, fmt.Errorf("npipe.PipeConn.Write(): %s", err)
	}
	defer windows.CloseHandle(overlapped.HEvent)
	var n uint32
	err = windows.WriteFile(c.handle, b, &n, overlapped)
	return c.completeRequest(iodata{n, err}, c.writeDeadline, overlapped)
}

// Close closes the connection.
func (c *PipeConn) Close() error {
	return windows.CloseHandle(c.handle)
}

// LocalAddr returns the local network address.
func (c *PipeConn) LocalAddr() net.Addr {
	return c.addr
}

// RemoteAddr returns the remote network address.
func (c *PipeConn) RemoteAddr() net.Addr {
	// not sure what to do here, we don't have remote addr....
	return c.addr
}

// SetDeadline implements the net.Conn SetDeadline method.
// Note that timeouts are only supported on Windows Vista/Server 2008 and above
func (c *PipeConn) SetDeadline(t time.Time) error {
	err := c.SetReadDeadline(t)
	if err != nil {
		return fmt.Errorf("npipe.PipeConn.SetDeadline(): %s", err)
	}
	err = c.SetWriteDeadline(t)
	if err != nil {
		return fmt.Errorf("npipe.PipeConn.SetDeadline(): %s", err)
	}
	return nil
}

// SetReadDeadline implements the net.Conn SetReadDeadline method.
// Note that timeouts are only supported on Windows Vista/Server 2008 and above
func (c *PipeConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = &t
	return nil
}

// SetWriteDeadline implements the net.Conn SetWriteDeadline method.
// Note that timeouts are only supported on Windows Vista/Server 2008 and above
func (c *PipeConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = &t
	return nil
}
