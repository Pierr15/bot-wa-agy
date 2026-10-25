//go:build !windows

package storage

import (
	"golang.org/x/sys/unix"
)

func GetFreeDiskBytes(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	// Bavail is the number of free blocks available to unprivileged user
	return stat.Bavail * uint64(stat.Bsize), nil
}
