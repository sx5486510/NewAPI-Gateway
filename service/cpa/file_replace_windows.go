//go:build windows

package cpa

import "golang.org/x/sys/windows"

func replaceExistingFile(oldpath, newpath string) error {
	return windows.MoveFileEx(
		windows.StringToUTF16Ptr(oldpath),
		windows.StringToUTF16Ptr(newpath),
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}
