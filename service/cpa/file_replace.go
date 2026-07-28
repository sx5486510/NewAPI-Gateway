//go:build !windows

package cpa

import "os"

func replaceExistingFile(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
