//go:build linux

package elog

import (
	"errors"
	"golang.org/x/sys/unix"
	"time"
)

func logFileCreatedAt(path string) (time.Time, error) {
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BTIME, &stat); err != nil {
		return time.Time{}, err
	}
	if stat.Mask&unix.STATX_BTIME == 0 {
		return time.Time{}, errors.New("log filesystem does not expose creation time")
	}
	return time.Unix(stat.Btime.Sec, int64(stat.Btime.Nsec)), nil
}
