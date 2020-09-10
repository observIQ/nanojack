// +build !windows

package nanojack

import (
	"os"
	"syscall"
)

// os_Chown is a var so we can mock it out during tests.
var os_Chown = os.Chown

func chown(name string, info os.FileInfo) error {
	stat := info.Sys().(*syscall.Stat_t)
	return os_Chown(name, int(stat.Uid), int(stat.Gid))
}
