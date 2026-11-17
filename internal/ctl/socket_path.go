package ctl

import (
	"fmt"
	"runtime"
)

// socketPathLimit reserves one byte in sun_path for the trailing NUL.
// Windows keeps its existing socket behavior without this Unix path check.
func socketPathLimit() int {
	switch runtime.GOOS {
	case "windows":
		return 0
	case "darwin", "ios", "freebsd", "netbsd", "openbsd", "dragonfly":
		return 103
	default:
		return 107
	}
}

// SocketPathError means the control socket path is longer than unix sockets
// allow here. No server can listen on such a path, so it also matches
// ErrNotRunning for callers that treat a missing server as a normal state.
type SocketPathError struct {
	Path  string
	Limit int
}

func (e *SocketPathError) Error() string {
	return fmt.Sprintf("control socket path %q is %d bytes, longer than the %d this system allows: set LFR_SOCKET to a shorter path (such as /tmp/lfr-<name>.sock) or use a shorter --data/LFR_DATA", e.Path, len(e.Path), e.Limit)
}

// Is reports a too-long path as ErrNotRunning.
func (e *SocketPathError) Is(target error) bool { return target == ErrNotRunning }

func checkSocketPath(path string) error {
	limit := socketPathLimit()
	if limit != 0 && len(path) > limit {
		return &SocketPathError{Path: path, Limit: limit}
	}
	return nil
}
