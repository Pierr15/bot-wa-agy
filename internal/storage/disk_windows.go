//go:build windows

package storage

import (
	"golang.org/x/sys/windows"
)

func GetFreeDiskBytes(path string) (uint64, error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	err = windows.GetDiskFreeSpaceEx(p, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)
	if err != nil {
		return 0, err
	}
	return freeBytesAvailable, nil
}
