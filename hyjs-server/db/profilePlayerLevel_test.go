package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func assertProfilePlayerLevelSQLiteSchema(t *testing.T, database *sql.DB) {
	t.Helper()
	columns, err := database.Query("PRAGMA table_xinfo(profiles)")
	if err != nil {
		t.Fatal(err)
	}
	var levelFound bool
	for columns.Next() {
		var cid, notNull, primaryKey, hidden int
		var name, columnType string
		var defaultValue sql.NullString
		if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey, &hidden); err != nil {
			columns.Close()
			t.Fatal(err)
		}
		if name == "player_level" {
			levelFound = true
			if columnType != "INTEGER" || hidden != 2 {
				columns.Close()
				t.Fatalf("player_level is not a virtual INTEGER column: type=%q hidden=%d", columnType, hidden)
			}
		}
	}
	if err := columns.Err(); err != nil {
		columns.Close()
		t.Fatal(err)
	}
	if err := columns.Close(); err != nil {
		t.Fatal(err)
	}
	if !levelFound {
		t.Fatal("player_level column missing")
	}

	indexes, err := database.Query("PRAGMA index_list(profiles)")
	if err != nil {
		t.Fatal(err)
	}
	var indexFound bool
	for indexes.Next() {
		var sequence, unique, partial int
		var name, origin string
		if err := indexes.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			indexes.Close()
			t.Fatal(err)
		}
		if name == "idx_profiles_server_level" {
			indexFound = true
			if unique != 0 {
				indexes.Close()
				t.Fatal("player-level index unexpectedly unique")
			}
		}
	}
	if err := indexes.Err(); err != nil {
		indexes.Close()
		t.Fatal(err)
	}
	if err := indexes.Close(); err != nil {
		t.Fatal(err)
	}
	if !indexFound {
		t.Fatal("player-level index missing")
	}
	parts, err := database.Query("PRAGMA index_info(idx_profiles_server_level)")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for parts.Next() {
		var sequence, cid int
		var name string
		if err := parts.Scan(&sequence, &cid, &name); err != nil {
			parts.Close()
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := parts.Err(); err != nil {
		parts.Close()
		t.Fatal(err)
	}
	if err := parts.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"server_id", "player_level", "id"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("player-level index columns: got %v, want %v", names, want)
	}
}

func assertProfilePlayerLevelSQLiteValue(t *testing.T, database *sql.DB, uid string, valid bool, want int64) {
	t.Helper()
	var got sql.NullInt64
	if err := database.QueryRow("SELECT player_level FROM profiles WHERE uid=?", uid).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Valid != valid || valid && got.Int64 != want {
		t.Fatalf("uid %q projected %+v, want valid=%t level=%d", uid, got, valid, want)
	}
}

func TestProfilePlayerLevelSQLiteSchemaAndRawWrites(t *testing.T) {
	database, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(profileSchema); err != nil {
		t.Fatalf("execute standalone profile SQL: %v", err)
	}
	assertProfilePlayerLevelSQLiteSchema(t, database)
	if _, err := database.Exec("INSERT INTO profiles(uid,session) VALUES(?,?)", "mutable", []byte{1}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		doc   any
		valid bool
		level int64
	}{
		{name: "zero", doc: `{"level":0}`, valid: true, level: 0},
		{name: "seventy", doc: `{"level":70}`, valid: true, level: 70},
		{name: "maximum", doc: `{"level":2147483647}`, valid: true, level: 2147483647},
		{name: "sql-null", doc: nil},
		{name: "json-null", doc: `{"level":null}`},
		{name: "invalid-json", doc: `{"level":`},
		{name: "string", doc: `{"level":"70"}`},
		{name: "negative", doc: `{"level":-1}`},
		{name: "out-of-range", doc: `{"level":2147483648}`},
		{name: "large-integer", doc: `{"level":9223372036854775808}`},
		{name: "decimal", doc: `{"level":70.0}`},
		{name: "boolean", doc: `{"level":true}`},
		{name: "missing", doc: `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uid := "insert-" + tc.name
			if _, err := database.Exec("INSERT INTO profiles(uid,session,player) VALUES(?,?,?)", uid, []byte{1}, tc.doc); err != nil {
				t.Fatalf("raw insert: %v", err)
			}
			assertProfilePlayerLevelSQLiteValue(t, database, uid, tc.valid, tc.level)
			if _, err := database.Exec("UPDATE profiles SET player=? WHERE uid=?", tc.doc, "mutable"); err != nil {
				t.Fatalf("raw update: %v", err)
			}
			assertProfilePlayerLevelSQLiteValue(t, database, "mutable", tc.valid, tc.level)
		})
	}
	if _, err := database.Exec("UPDATE profiles SET player_level=123 WHERE uid='mutable'"); err == nil {
		t.Fatal("raw write to generated player_level was accepted")
	}
}

func TestProfilePlayerLevelSQLiteInitGameAndIndexRepair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.sqlite3")
	if err := InitGame(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	database := GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	assertProfilePlayerLevelSQLiteSchema(t, pool)
	if err := database.Exec("DROP INDEX idx_profiles_server_level").Error; err != nil {
		t.Fatal(err)
	}
	if database.Migrator().HasIndex(&Profile{}, "idx_profiles_server_level") {
		t.Fatal("fixture did not remove the index")
	}
	for i := 0; i < 2; i++ {
		if err := migrateProfilePlayerLevel(database); err != nil {
			t.Fatalf("repair run %d: %v", i, err)
		}
		assertProfilePlayerLevelSQLiteSchema(t, pool)
	}
	player := `{"level":70}`
	ignoredLevel := int64(999)
	profile := &Profile{Uid: "gorm-read-only", Session: []byte{1}, Player: &player, PlayerLevel: &ignoredLevel}
	if err := database.Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	var saved Profile
	if err := database.First(&saved, "uid=?", profile.Uid).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PlayerLevel == nil || *saved.PlayerLevel != 70 {
		t.Fatalf("gorm read-only generated level: %v", saved.PlayerLevel)
	}
}

func TestProfilePlayerLevelSQLiteLegacyMigration(t *testing.T) {
	for _, existingScope := range []bool{false, true} {
		t.Run(fmt.Sprintf("scope-present-%t", existingScope), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.sqlite3")
			legacy, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			scope := ""
			if existingScope {
				scope = `, scene_scope TEXT GENERATED ALWAYS AS (CASE WHEN game IS NULL THEN NULL ELSE '' END) VIRTUAL`
			}
			schema := `CREATE TABLE profiles (id INTEGER PRIMARY KEY AUTOINCREMENT, uid TEXT NOT NULL UNIQUE, session BLOB NOT NULL, player TEXT, game TEXT` + scope + `)`
			if _, err := legacy.Exec(schema); err != nil {
				legacy.Close()
				t.Fatal(err)
			}
			if _, err := legacy.Exec("INSERT INTO profiles(id,uid,session,player) VALUES(?,?,?,?)", 9, "legacy", []byte{1}, `{"level":70}`); err != nil {
				legacy.Close()
				t.Fatal(err)
			}
			if _, err := legacy.Exec("INSERT INTO profiles(id,uid,session,player) VALUES(?,?,?,?)", 10, "legacy-invalid", []byte{1}, `{"level":`); err != nil {
				legacy.Close()
				t.Fatal(err)
			}
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err := InitGame(context.Background(), path); err != nil {
					t.Fatalf("legacy migration run %d: %v", i, err)
				}
				pool, err := GetIns().DB()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = pool.Close() })
				assertProfilePlayerLevelSQLiteSchema(t, pool)
				assertProfilePlayerLevelSQLiteValue(t, pool, "legacy", true, 70)
				assertProfilePlayerLevelSQLiteValue(t, pool, "legacy-invalid", false, 0)
				if err := pool.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
