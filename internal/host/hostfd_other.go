//go:build !unix

package host

import (
	"errors"
	"os"
)

// OpenHostFDs: the host protocol's transport is POSIX only in 0.3b (SPEC
// v0.3b §19.1: "On Windows, host is a usage error (exit 1), since os/exec
// cannot pass extra descriptors there").
func OpenHostFDs() (fd3, fd4 *os.File, err error) {
	return nil, nil, errors.New("the host protocol needs fd 3 and fd 4, which this platform cannot pass (POSIX only)")
}
