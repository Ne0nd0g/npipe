# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

** This project is a fork of https://github.com/natefinch/npipe **

## 1.1.1 - 2026-10-04

### Changed

- Bumped the Go directive to 1.27.0
- Upgraded `golang.org/x/sys` to v0.48.0 (now a direct requirement)
- Added a `go.sum` (previously absent)
- Added GitHub Actions CI (build/test/vet/govulncheck + golangci-lint + gosec on
  windows-latest, and CodeQL) and a `.golangci.yml`
- Cleaned up all golangci-lint findings: checked previously-ignored cleanup errors
  (`CloseHandle`/`CancelIoEx`/`Close`), switched `t.Sub(time.Now())` to
  `time.Until`, replaced deprecated `io/ioutil` with `io`, passed the test
  `sync.WaitGroup` by pointer, and moved `t.Fatalf`/`t.Fatal` calls out of
  non-test goroutines (now `t.Errorf`/`t.Error` + return)

### Fixed

- Wrapped Windows API errors with `%w` instead of `%s` and switched every errno
  comparison to `errors.Is`/`errors.As`. The previous `%s` wrapping flattened the
  `windows.Errno` to a string, which silently broke every sentinel check in the
  package:
  - `Dial` no longer waited for a pipe to become available — `isPipeNotReady`
    could not match `ERROR_FILE_NOT_FOUND`/`ERROR_PIPE_BUSY` through the wrap, so
    it returned immediately instead of retrying.
  - `DialTimeout` could not recognize its own `ERROR_SEM_TIMEOUT`.
  - `Dial` returned an opaque wrapped error instead of a `PipeError` for a badly
    formatted pipe name (`ERROR_BAD_PATHNAME`).
  - `PipeListener.Accept` returned a wrapped error instead of `ErrClosed` when the
    listener was closed during a blocking accept (`ERROR_OPERATION_ABORTED`),
    breaking its documented `net.Listener`-compatible contract.
  - `PipeConn` reads/writes no longer translated a closed pipe
    (`ERROR_BROKEN_PIPE`) into `io.EOF`, which Go's `net/rpc` relies on.
- Updated the test suite to assert error identity with `errors.As`/`errors.Is`.

## 1.1.0 - 2023-04-23

### Changed

- Upgraded `golang.org/x/sys` from v0.4.0 to the latest v0.7.0
- Moved `iodata` struct from `npipe_windows.go` to `con_windows.go`

## 1.0.0 - 2023-01-25

### Changed

- Replaced all `syscall` packages with `golang.org/x/sys/windows`

### Added

- Added following factories to create and return a PipeListener
  - `NewPipeListener()` provides full access to all configurable options
  - `NewPipeListenerQuick()` just provide a pipe name all defaults will be used for everything else
