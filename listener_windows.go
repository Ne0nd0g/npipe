//go:build windows

package npipe

import (
	// Standard
	"fmt"
	"net"
	"sync"

	// X Package
	"golang.org/x/sys/windows"
)

// PipeListener is a named pipe listener. Clients should typically use variables of type net.Listener instead of assuming named pipe.
type PipeListener struct {
	mu     sync.Mutex
	addr   PipeAddr
	handle windows.Handle
	closed bool

	// acceptHandle contains the current handle waiting for
	// an incoming connection or nil.
	acceptHandle windows.Handle
	// acceptOverlapped is set before waiting on a connection.
	// If not waiting, it is nil.
	acceptOverlapped *windows.Overlapped
}

// NewPipeListener is a factory that creates and returns a pointer to a PipeListener
// https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-createnamedpipew
// HANDLE CreateNamedPipeW(
//
//	[in]           LPCWSTR               lpName,
//	[in]           DWORD                 dwOpenMode,
//	[in]           DWORD                 dwPipeMode,
//	[in]           DWORD                 nMaxInstances,
//	[in]           DWORD                 nOutBufferSize,
//	[in]           DWORD                 nInBufferSize,
//	[in]           DWORD                 nDefaultTimeOut,
//	[in, optional] LPSECURITY_ATTRIBUTES lpSecurityAttributes
//
// );
func NewPipeListener(name string, openMode, pipeMode, maxInstances, outBuffer, inBuffer, timeout uint32, sa *windows.SecurityAttributes) (*PipeListener, error) {
	// Validate the provided named pipe path
	err := ValidatePipeAddress(name)
	if err != nil {
		return nil, fmt.Errorf("npipe.NewPipeListener(): %s", err)
	}

	// Convert the pipe name to a UTF-16 string pointer
	lpName, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("npipe.NewPipeListener(): there was an error converting \"%s\" to a UTF16 pointer: %s", name, err)
	}

	// Create the named pipe
	handle, err := windows.CreateNamedPipe(lpName, openMode, pipeMode, maxInstances, outBuffer, inBuffer, timeout, sa)
	if err != nil {
		return nil, fmt.Errorf("npipe.NewPipeListener(): there was an error calling the WINAPI CreateNamedPipe function: %s", err)
	}

	pl := PipeListener{
		mu:               sync.Mutex{},
		addr:             PipeAddr(name),
		handle:           handle,
		closed:           false,
		acceptHandle:     0,
		acceptOverlapped: nil,
	}
	return &pl, nil
}

// NewPipeListenerQuick creates a Windows named pipe in a default configuration where
// The pipe mode will be type BYTE
// An unlimited number of instances can be created for this pipe
// The In and Out buffer size will be 512 bytes
// The default timeout is set to zero which ends up being 50 milliseconds
// The default Security Descriptor is full control to the LocalSystem account, administrators, and the creator owner
//
//	Read access is granted to members of the "Everyone" group and the "anonymous" account.
//
// This function replaced the "createPipe" function from the npipe package before it was forked
func NewPipeListenerQuick(name string, first bool) (*PipeListener, error) {
	mode := windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED
	if first {
		mode |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}

	listener, err := NewPipeListener(name, uint32(mode), windows.PIPE_TYPE_BYTE, windows.PIPE_UNLIMITED_INSTANCES, 512, 512, 0, nil)
	if err != nil {
		err = fmt.Errorf("\"npipe.NewPipeListenerQuick(): %s\"", err)
	}
	return listener, err
}

// Accept implements the Accept method in the net.Listener interface; it
// waits for the next call and returns a generic net.Conn.
func (l *PipeListener) Accept() (net.Conn, error) {
	c, err := l.AcceptPipe()
	for err == windows.ERROR_NO_DATA {
		// Ignore clients that connect and immediately disconnect.
		c, err = l.AcceptPipe()
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// AcceptPipe accepts the next incoming call and returns the new connection.
// It might return an error if a client connected and immediately cancelled
// the connection.
func (l *PipeListener) AcceptPipe() (*PipeConn, error) {
	if l == nil {
		return nil, fmt.Errorf("npipe.PipeListener.AcceptPipe(): the PipeListener is nil")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.addr == "" || l.closed {
		return nil, fmt.Errorf("npipe.PipeListener.AcceptPipe(): the address is empty or the listener is closed")
	}

	// the first time we call accept, the handle will have been created by the Listen
	// call. This is to prevent race conditions where the client thinks the server
	// isn't listening because it hasn't actually called create yet. After the first time, we'll
	// have to create a new handle each time
	handle := l.handle
	if handle == 0 {
		var err error
		// Convert the pipe name to a UTF-16 string pointer
		lpName, err := windows.UTF16PtrFromString(l.addr.String())
		if err != nil {
			return nil, fmt.Errorf("npipe.PipeListener.AcceptPipe(): there was an error converting \"%s\" to a UTF16 pointer: %s", l.addr, err)
		}
		handle, err = windows.CreateNamedPipe(lpName, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED, windows.PIPE_TYPE_BYTE, windows.PIPE_UNLIMITED_INSTANCES, 512, 512, 0, nil)
		if err != nil {
			return nil, err
		}
	} else {
		l.handle = 0
	}

	overlapped, err := newOverlapped()
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(overlapped.HEvent)
	err = windows.ConnectNamedPipe(handle, overlapped)
	if err == nil || err == windows.ERROR_PIPE_CONNECTED {
		return &PipeConn{handle: handle, addr: l.addr}, nil
	}

	if err == windows.ERROR_IO_INCOMPLETE || err == windows.ERROR_IO_PENDING {
		l.acceptOverlapped = overlapped
		l.acceptHandle = handle
		// unlock here so close can function correctly while we wait (we'll
		// get relocked via the defer below, before the original defer
		// unlock happens.)
		l.mu.Unlock()
		defer func() {
			l.mu.Lock()
			l.acceptOverlapped = nil
			l.acceptHandle = 0
			// unlock is via defer above.
		}()
		_, err = waitForCompletion(handle, overlapped)
	}
	if err == windows.ERROR_OPERATION_ABORTED {
		// Return error compatible to net.Listener.Accept() in case the
		// listener was closed.
		return nil, ErrClosed
	}
	if err != nil {
		return nil, err
	}
	return &PipeConn{handle: handle, addr: l.addr}, nil
}

// Close stops listening on the address.
// Already Accepted connections are not closed.
func (l *PipeListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true
	if l.handle != 0 {
		err := disconnectNamedPipe(l.handle)
		if err != nil {
			return err
		}
		err = windows.CloseHandle(l.handle)
		if err != nil {
			return err
		}
		l.handle = 0
	}
	if l.acceptOverlapped != nil && l.acceptHandle != 0 {
		// Cancel the pending IO. This call does not block, so it is safe
		// to hold onto the mutex above.

		if err := windows.CancelIoEx(l.acceptHandle, l.acceptOverlapped); err != nil {
			return err
		}
		err := windows.CloseHandle(l.acceptOverlapped.HEvent)
		if err != nil {
			return err
		}
		l.acceptOverlapped.HEvent = 0
		err = windows.CloseHandle(l.acceptHandle)
		if err != nil {
			return err
		}
		l.acceptHandle = 0
	}
	return nil
}

// Addr returns the listener's network address, a PipeAddr.
func (l *PipeListener) Addr() net.Addr { return l.addr }

// Handle returns the Windows Handle to
func (l *PipeListener) Handle() windows.Handle {
	return l.handle
}
