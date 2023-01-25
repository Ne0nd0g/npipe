//go:build windows

package npipe

import (
	"fmt"
	// Standard
	"unsafe"

	// X Package
	"golang.org/x/sys/windows"
)

var (
	modkernel32 = windows.NewLazyDLL("kernel32.dll")
)

// disconnectNamedPipe disconnects the server end of a named pipe instance from a client process.
// https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-disconnectnamedpipe
// BOOL DisconnectNamedPipe(
//
//	[in] HANDLE hNamedPipe
//
// );
func disconnectNamedPipe(handle windows.Handle) error {
	procDisconnectNamedPipe := modkernel32.NewProc("DisconnectNamedPipe")
	ret, _, err := procDisconnectNamedPipe.Call(uintptr(handle))
	if err != windows.Errno(0) {
		return fmt.Errorf("npipe.disconnectNamedPipe(): there was an error calling the Windows API function DisconnectNamedPipe with return code %d: %s", ret, err)
	}
	return nil
}

// waitNamedPipe waits until either a time-out interval elapses or an instance of the specified named pipe is available
// for connection (that is, the pipe's server process has a pending ConnectNamedPipe operation on the pipe).
// https://learn.microsoft.com/en-us/windows/win32/api/namedpipeapi/nf-namedpipeapi-waitnamedpipew
// BOOL WaitNamedPipeW(
//
//	[in] LPCWSTR lpNamedPipeName,
//	[in] DWORD   nTimeOut
//
// );
func waitNamedPipe(name *uint16, timeout uint32) error {
	procWaitNamedPipeW := modkernel32.NewProc("WaitNamedPipeW")
	ret, _, err := procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(name)), uintptr(timeout), 0)
	if err != windows.Errno(0) {
		return fmt.Errorf("npipe.waitNamedPipe(): there was an error calling the Windows API function WaitNamedPipeW with return code %d: %s", ret, err)
	}
	return nil
}
