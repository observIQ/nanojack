// +build !linux

package nanojack

import (
	"os"
)

func chown(_ string, _ os.FileInfo) error {
	return nil
}