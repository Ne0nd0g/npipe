# Changelog
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).

** This project is a fork of https://github.com/natefinch/npipe **

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
