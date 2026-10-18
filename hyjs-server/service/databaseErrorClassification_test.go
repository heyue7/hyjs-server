package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
)

func TestAdminActivityValidationAndAccountDuplicateClassification(t *testing.T) {
	duplicate := fmt.Errorf("insert: %w", &mysqlDriver.MySQLError{Number: 1062, Message: "duplicate"})
	queryFailure := &mysqlDriver.MySQLError{Number: 1146, Message: "missing table"}
	if !gameAccountDuplicate(duplicate) || !gameAccountDuplicate(gorm.ErrDuplicatedKey) || gameAccountDuplicate(queryFailure) || gameAccountDuplicate(errors.New("duplicate entry")) || gameAccountDuplicate(nil) {
		t.Fatal("MySQL/gorm duplicate classification changed")
	}
	for _, err := range []error{duplicate, queryFailure, context.Canceled, gorm.ErrInvalidDB, errors.New("sql: connection failure")} {
		if adminActivityValidationError(err) != err {
			t.Fatal("database/runtime error became an input error")
		}
	}
	if adminActivityValidationError(nil) != nil || !errors.Is(adminActivityValidationError(errors.New("invalid reward")), constUtil.ErrAdminInput) {
		t.Fatal("ordinary configuration validation classification changed")
	}
}
