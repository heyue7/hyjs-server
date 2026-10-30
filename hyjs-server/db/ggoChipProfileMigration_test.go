package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestGGOChipProjectionLegacySQLiteMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = legacy.Exec(`CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, uid TEXT NOT NULL UNIQUE,
		session BLOB NOT NULL, player TEXT, game TEXT, created_at INTEGER)`); err != nil {
		t.Fatal(err)
	}
	game := `{"position":{"scene":"field_003"},"ggo_field":{"chips":[1,2,3,4,5,6]}}`
	if _, err = legacy.Exec("INSERT INTO profiles(id,uid,session,game) VALUES(?,?,?,?)", 5, "legacy-chip", []byte{1}, game); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}
	if err = InitGame(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn, _ := GetIns().DB(); conn.Close() })
	var row Profile
	if err = GetIns().First(&row, 5).Error; err != nil {
		t.Fatal(err)
	}
	if row.GGOChipTotal != nil || row.Game == nil || *row.Game != game || row.Uid != "legacy-chip" {
		t.Fatal("legacy profile changed or projection was guessed", row.GGOChipTotal, row.Game)
	}
}
