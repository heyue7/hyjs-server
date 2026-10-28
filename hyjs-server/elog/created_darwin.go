//go:build darwin

package elog

import (
	"golang.org/x/sys/unix"
	"time"
)

func logFileCreatedAt(path string) (time.Time, error) {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return time.Time{}, err
	}
	return time.Unix(stat.Btim.Sec, stat.Btim.Nsec), nil
}
