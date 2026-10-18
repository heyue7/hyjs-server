//go:build !cgo

package db

// IsSQLiteDriverError remains false when this build has no working SQLite driver.
func IsSQLiteDriverError(error) bool { return false }

// IsSQLiteUniqueConstraint remains false for the MySQL-only non-CGO runtime.
func IsSQLiteUniqueConstraint(error) bool { return false }
