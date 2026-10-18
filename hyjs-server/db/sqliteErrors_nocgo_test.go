//go:build !cgo

package db

import (
	"errors"
	"testing"
)

func TestSQLiteDriverUnavailableClassification(t *testing.T) {
	for _, err := range []error{nil, errors.New("UNIQUE constraint failed"), errors.New("SQLite requires cgo")} {
		if IsSQLiteDriverError(err) || IsSQLiteUniqueConstraint(err) {
			t.Fatal("non-CGO build cannot classify an unavailable SQLite driver")
		}
	}
}
