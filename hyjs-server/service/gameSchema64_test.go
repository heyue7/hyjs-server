package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"hyjs-server/db"
)

type schema64Pool struct{ *sql.DB }

func TestGameSchema64OwnerDatabaseAndDynamicFallback(t *testing.T) {
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "schema64.db")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	connection, _ := d.DB()
	t.Cleanup(func() { connection.Close() })
	s := &GameService{}
	ctx := bindFieldRuntime(context.Background(), s)
	recorder := &adminActivityQueryRecorder{Interface: d.Config.Logger}
	d.Config.Logger = recorder
	tx := d.WithContext(ctx)
	check := func(label string, q *gorm.DB, table any, want bool, metadata bool) {
		t.Helper()
		recorder.reset()
		if got := gameSchemaHasTable(q, table); got != want {
			t.Fatalf("%s got %v want %v", label, got, want)
		}
		count := recorder.countContaining("sqlite_master")
		if (count > 0) != metadata {
			t.Fatalf("%s metadata=%d want probe=%v", label, count, metadata)
		}
	}
	check("uninitialized", tx, &db.Profile{}, true, true)
	if err := s.InitializeGameSchema64(ctx); err != nil {
		t.Fatal(err)
	}
	check("model", tx, &db.Profile{}, true, false)
	check("string", tx, "profiles", true, false)
	clone := s.withRuntimeTables(nil)
	check("clone", d.WithContext(bindFieldRuntime(ctx, clone)), &db.Profile{}, true, false)
	check("other owner", d.WithContext(bindFieldRuntime(ctx, &GameService{})), &db.Profile{}, true, true)
	check("no owner", d.WithContext(context.Background()), &db.Profile{}, true, true)
	check("table override", tx.Table("missing_schema64"), &db.Profile{}, false, true)
	check("alias", tx.Table("profiles AS p"), &db.Profile{}, false, true)
	check("missing", tx, "late_schema64", false, true)
	if err := d.Exec("CREATE TABLE late_schema64(id INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	check("late table", tx, "late_schema64", true, true)
	if err := d.Exec("CREATE VIEW schema64_view AS SELECT id FROM profiles").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeGameSchema64(ctx); err != nil {
		t.Fatal(err)
	}
	check("view", tx, "schema64_view", false, true)
	if err := tx.Transaction(func(transaction *gorm.DB) error {
		check("transaction", transaction, &db.Profile{}, true, false)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "other.db")), &gorm.Config{Logger: recorder})
	if err != nil {
		t.Fatal(err)
	}
	otherConnection, _ := other.DB()
	defer otherConnection.Close()
	check("other database", other.WithContext(ctx), &db.Profile{}, false, true)
	if err := other.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		check("other transaction", transaction, &db.Profile{}, false, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Unknown driver wrappers retain the original metadata path.
	previousPool, previousStatementPool := d.Config.ConnPool, d.Statement.ConnPool
	wrapped := &schema64Pool{connection}
	d.Config.ConnPool = wrapped
	d.Statement.ConnPool = wrapped
	check("pool changed", tx, &db.Profile{}, true, true)
	if err := s.InitializeGameSchema64(ctx); err != nil {
		t.Fatal(err)
	}
	check("unknown pool snapshot", d.WithContext(ctx), &db.Profile{}, true, true)
	d.Config.ConnPool = previousPool
	d.Statement.ConnPool = previousStatementPool
	if err := s.InitializeGameSchema64(ctx); err != nil {
		t.Fatal(err)
	}
	// Switching the global database invalidates the old service snapshot.
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "replacement.db")); err != nil {
		t.Fatal(err)
	}
	replacement, _ := db.GetIns().DB()
	defer replacement.Close()
	check("global switched", tx, &db.Profile{}, true, true)
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeGameSchema64(ctx); err == nil {
		t.Fatal("closed database initialization succeeded")
	}
	if s.gameSchema64.Load() != nil {
		t.Fatal("failed initialization published snapshot")
	}
}
