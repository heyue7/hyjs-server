//go:build !linux && !darwin

package elog

import (
	"errors"
	"time"
)

func logFileCreatedAt(string) (time.Time, error) {
	return time.Time{}, errors.New("log creation-time retention is unsupported on this platform")
}
