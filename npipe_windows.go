//go:build windows

// Package npipe provides wrapper functions to more easily interact with Windows named pipes
package npipe

import (
	// Standard
	"fmt"
	"net"
	"strings"
	"time"

	// X Package
	"golang.org/x/sys/windows"
)

// Dial connects to a named pipe with the given address. If the specified pipe is not available,
// it will wait indefinitely for the pipe to become available.
//
// The address must be of the form \\.\\pipe\<name> for local pipes and \\<computer>\pipe\<name>
// for remote pipes.
//
// Dial will return a PipeError if you pass in a badly formatted pipe name.
//
// Examples:
//
//	// local pipe
//	conn, err := Dial(`\\.\pipe\mypipename`)
//
//	// remote pipe
//	conn, err := Dial(`\\othercomp\pipe\mypipename`)
func Dial(address string) (*PipeConn, error) {
	for {
		conn, err := dial(address, 0xFFFFFFFF)
		if err == nil {
			return conn, nil
		}
		if isPipeNotReady(err) {
			<-time.After(100 * time.Millisecond)
			continue
		}
		return nil, fmt.Errorf("npipe.Dial(): %s", err)
	}
}

// DialTimeout acts like Dial, but will time out after the duration of timeout
func DialTimeout(address string, timeout time.Duration) (*PipeConn, error) {
	deadline := time.Now().Add(timeout)

	now := time.Now()
	for now.Before(deadline) {
		millis := uint32(deadline.Sub(now) / time.Millisecond)
		conn, err := dial(address, millis)
		if err == nil {
			return conn, nil
		}
		if err == windows.ERROR_SEM_TIMEOUT {
			// This is WaitNamedPipe's timeout error, so we know we're done
			return nil, PipeError{fmt.Sprintf(
				"npipe.DialTimeout(): timed out waiting for pipe '%s' to come available", address), true}
		}
		if isPipeNotReady(err) {
			left := deadline.Sub(time.Now())
			retry := 100 * time.Millisecond
			if left > retry {
				<-time.After(retry)
			} else {
				<-time.After(left - time.Millisecond)
			}
			now = time.Now()
			continue
		}
		return nil, err
	}
	return nil, PipeError{fmt.Sprintf(
		"npipe.DialTimeout(): timed out waiting for pipe '%s' to come available", address), true}
}

// isPipeNotReady checks the error to see if it indicates the pipe is not ready
func isPipeNotReady(err error) bool {
	// Pipe Busy means another client just grabbed the open pipe end,
	// and the server hasn't made a new one yet.
	// File Not Found means the server hasn't created the pipe yet.
	// Neither is a fatal error.

	return err == windows.ERROR_FILE_NOT_FOUND || err == windows.ERROR_PIPE_BUSY
}

// newOverlapped creates a structure used to track asynchronous
// I/O requests that have been issued.
func newOverlapped() (*windows.Overlapped, error) {
	event, err := windows.CreateEvent(nil, 1, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("npipe.newOverlapped(): there was an error callling WINAPI CreateEvent: %s", err)
	}
	return &windows.Overlapped{HEvent: event}, nil
}

// waitForCompletion waits for an asynchronous I/O request referred to by overlapped to complete.
// This function returns the number of bytes transferred by the operation and an error code if
// applicable (nil otherwise).
func waitForCompletion(handle windows.Handle, overlapped *windows.Overlapped) (transferred uint32, err error) {
	_, err = windows.WaitForSingleObject(overlapped.HEvent, windows.INFINITE)
	if err != nil {
		return 0, fmt.Errorf("npipe.waitForCompletion(): there was an error calling WINAPI WaitForSingleObject: %s", err)
	}

	// GetOverlappedResult retrieves the results of an overlapped operation on the specified file, named pipe, or communications device.
	// https://learn.microsoft.com/en-us/windows/win32/api/ioapiset/nf-ioapiset-getoverlappedresult
	err = windows.GetOverlappedResult(handle, overlapped, &transferred, true)
	if err != nil {
		err = fmt.Errorf("npipe.waitForCompletion(): there was an error calling WINAPI GetOverlappedResult: %s", err)
	}
	return transferred, err
}

// dial is a helper to initiate a connection to a named pipe that has been started by a server.
// The timeout is only enforced if the pipe server has already created the pipe, otherwise
// this function will return immediately.
func dial(address string, timeout uint32) (*PipeConn, error) {
	name, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return nil, fmt.Errorf("npipe.dial(): there was an error converting \"%s\" to a UTF16 pointer: %s", address, err)
	}
	// If at least one instance of the pipe has been created, this function
	// will wait timeout milliseconds for it to become available.
	// It will return immediately regardless of timeout, if no instances
	// of the named pipe have been created yet.
	// If this returns with no error, there is a pipe available.
	if err = waitNamedPipe(name, timeout); err != nil {
		if err == windows.ERROR_BAD_PATHNAME {
			// badly formatted pipe name
			return nil, badAddr(address)
		}
		return nil, err
	}
	pathp, err := windows.UTF16PtrFromString(address)
	if err != nil {
		return nil, fmt.Errorf("npipe.dial(): there was an error converting \"%s\" to a UTF16 pointer: %s", address, err)
	}
	handle, err := windows.CreateFile(
		pathp, windows.GENERIC_READ|windows.GENERIC_WRITE,
		uint32(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE),
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("npipe.dial(): there was an error calling WINAPI CreateFile: %s", err)
	}
	return &PipeConn{handle: handle, addr: PipeAddr(address)}, nil
}

// Listen returns a new PipeListener that will listen on a pipe with the given address
// The address must be of the form \\.\pipe\<name>
// A PipeError for an incorrectly formatted pipe name
func Listen(address string) (*PipeListener, error) {
	pl, err := NewPipeListenerQuick(address, true)
	if err != nil {
		err = fmt.Errorf("npipe.Listen(): %s", err)
	}
	return pl, err
}

// ValidatePipeAddress validates that a proper Windows named pipe path was passed in (e.g., \\.\pipe\srvsvc)
func ValidatePipeAddress(address string) error {
	// Split to ensure there are enough parts
	// 0. blank
	// 1. blank
	// 2. is "." OR network IP
	// 3. must be PIPE
	// 4. must exist
	p := strings.Split(address, "\\")
	if len(p) < 5 {
		return fmt.Errorf("npipe.ValidatePipeAddress(): expected at least 5 parts from pipe address \"%s\" but received %d. Example: \\\\.\\pipe\\srvsvc", address, len(p))
	}

	// A dot "." represents the local host
	if p[2] != "." {
		// Ensure a valid IP address was provided
		ip := net.ParseIP(p[2])
		if ip == nil {
			return fmt.Errorf("npipe.ValidatePipeAddress(): invalid IP address \"%s\"", p[2])
		}
	}

	if strings.ToLower(p[3]) != "pipe" {
		return fmt.Errorf("npipe.ValidatePipeAddress(): expected \"pipe\" but received \"%s\"", p[3])
	}

	return nil
}

func badAddr(addr string) PipeError {
	return PipeError{fmt.Sprintf("Invalid pipe address '%s'.", addr), false}
}
func timeout(addr string) PipeError {
	return PipeError{fmt.Sprintf("Pipe IO timed out waiting for '%s'", addr), true}
}
