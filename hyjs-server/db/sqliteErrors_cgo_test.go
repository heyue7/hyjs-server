//go:build cgo

package db

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mattn/go-sqlite3"
)

func TestSQLiteDriverErrorClassification(t *testing.T) {
	for _, test := range []struct {
		name           string
		err            error
		driver, unique bool
	}{
		{"nil", nil, false, false},
		{"ordinary", errors.New("UNIQUE constraint failed"), false, false},
		{"unique", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique}, true, true},
		{"wrapped unique", fmt.Errorf("insert: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintUnique}), true, true},
		{"primary key", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintPrimaryKey}, true, false},
		{"not null", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: sqlite3.ErrConstraintNotNull}, true, false},
		{"busy", sqlite3.Error{Code: sqlite3.ErrBusy}, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if IsSQLiteDriverError(test.err) != test.driver || IsSQLiteUniqueConstraint(test.err) != test.unique {
				t.Fatal("SQLite error classification changed")
			}
		})
	}
}
