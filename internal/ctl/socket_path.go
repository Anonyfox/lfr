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

func checkSocketPath(path string) error {
	limit := socketPathLimit()
	if limit != 0 && len(path) > limit {
		return fmt.Errorf("control socket path %q is %d bytes, longer than the %d this system allows: set LFR_SOCKET to a shorter path (such as /tmp/lfr-<name>.sock) or use a shorter --data/LFR_DATA", path, len(path), limit)
	}
	return nil
}
