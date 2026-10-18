//go:build cgo

package db

import (
	"errors"

	"github.com/mattn/go-sqlite3"
)

// IsSQLiteDriverError recognizes the original SQLite driver error, including wrapped errors.
func IsSQLiteDriverError(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError)
}

// IsSQLiteUniqueConstraint preserves the registration path's exact unique-constraint classification.
func IsSQLiteUniqueConstraint(err error) bool {
	var sqliteError sqlite3.Error
	return errors.As(err, &sqliteError) && sqliteError.ExtendedCode == sqlite3.ErrConstraintUnique
}
