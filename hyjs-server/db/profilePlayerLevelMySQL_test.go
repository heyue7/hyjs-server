package db

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
)

func profilePlayerLevelMySQLError(t *testing.T, cfg MySQLConfig, action string, err error) {
	t.Helper()
	t.Fatalf("%s: %s", action, strings.ReplaceAll(err.Error(), cfg.Password, "[redacted]"))
}

func assertProfilePlayerLevelMySQLSchema(t *testing.T, cfg MySQLConfig, database *sql.DB) {
	t.Helper()
	var dataType, extra, nullable string
	if err := database.QueryRow(`SELECT DATA_TYPE,EXTRA,IS_NULLABLE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles' AND COLUMN_NAME='player_level'`).Scan(&dataType, &extra, &nullable); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "read generated-column metadata", err)
	}
	if dataType != "bigint" || !strings.Contains(strings.ToUpper(extra), "STORED GENERATED") || nullable != "YES" {
		t.Fatalf("unexpected player-level column: type=%s extra=%s nullable=%s", dataType, extra, nullable)
	}
	rows, err := database.Query(`SELECT COLUMN_NAME,SEQ_IN_INDEX,NON_UNIQUE FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles' AND INDEX_NAME='idx_profiles_server_level'
		ORDER BY SEQ_IN_INDEX`)
	if err != nil {
		profilePlayerLevelMySQLError(t, cfg, "read player-level index metadata", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		var sequence, nonUnique int
		if err := rows.Scan(&name, &sequence, &nonUnique); err != nil {
			profilePlayerLevelMySQLError(t, cfg, "scan player-level index metadata", err)
		}
		if sequence != len(names)+1 || nonUnique != 1 {
			t.Fatalf("unexpected player-level index metadata: position=%d nonUnique=%d", sequence, nonUnique)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "iterate player-level index metadata", err)
	}
	if want := []string{"server_id", "player_level", "id"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("player-level index columns: got %v, want %v", names, want)
	}
}

func assertProfilePlayerLevelMySQLValue(t *testing.T, cfg MySQLConfig, database *sql.DB, uid string, valid bool, want int64) {
	t.Helper()
	var got sql.NullInt64
	if err := database.QueryRow("SELECT player_level FROM profiles WHERE uid=?", uid).Scan(&got); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "read projected level", err)
	}
	if got.Valid != valid || valid && got.Int64 != want {
		t.Fatalf("uid %q projected %+v, want valid=%t level=%d", uid, got, valid, want)
	}
}

// This test owns an initially empty managed hyjs_test_p1_ schema. It removes
// only its own generated column/index to model a pre-projection profiles table.
func TestProfilePlayerLevelMySQL(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a fresh managed hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || len(name) == len("hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			t.Fatal("managed test schema has an invalid name")
		}
	}
	cfg := MySQLConfig{
		Host:     os.Getenv("HYJS_P1_MYSQL_HOST"),
		User:     os.Getenv("HYJS_P1_MYSQL_USER"),
		Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"),
		Database: name,
	}
	if cfg.Host == "" || cfg.User == "" || cfg.Password == "" {
		t.Fatal("managed P1 MySQL connection settings are incomplete")
	}
	connection := driver.NewConfig()
	connection.Net, connection.Addr = "tcp", cfg.Host
	connection.User, connection.Passwd, connection.DBName = cfg.User, cfg.Password, cfg.Database
	connection.Timeout, connection.ReadTimeout, connection.WriteTimeout = 10*time.Second, 30*time.Second, 30*time.Second
	database, err := sql.Open("mysql", connection.FormatDSN())
	if err != nil {
		profilePlayerLevelMySQLError(t, cfg, "open managed fixture", err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	var existing int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLES
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_TYPE='BASE TABLE'`).Scan(&existing); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "check managed fixture is empty", err)
	}
	if existing != 0 {
		t.Fatal("test requires a newly allocated empty schema")
	}
	game, err := OpenGameMySQL(ctx, cfg, true)
	if err != nil {
		profilePlayerLevelMySQLError(t, cfg, "create new profile schema", err)
	}
	pool, err := game.DB()
	if err != nil {
		profilePlayerLevelMySQLError(t, cfg, "get game database pool", err)
	}
	defer pool.Close()
	assertProfilePlayerLevelMySQLSchema(t, cfg, database)
	if _, err := database.ExecContext(ctx, "INSERT INTO profiles(uid,session) VALUES(?,?)", "mutable", []byte{1}); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "seed mutable profile", err)
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
		ok := t.Run(tc.name, func(t *testing.T) {
			uid := "insert-" + tc.name
			if _, err := database.ExecContext(ctx, "INSERT INTO profiles(uid,session,player) VALUES(?,?,?)", uid, []byte{1}, tc.doc); err != nil {
				profilePlayerLevelMySQLError(t, cfg, "insert profile", err)
			}
			assertProfilePlayerLevelMySQLValue(t, cfg, database, uid, tc.valid, tc.level)
			if _, err := database.ExecContext(ctx, "UPDATE profiles SET player=? WHERE uid=?", tc.doc, "mutable"); err != nil {
				profilePlayerLevelMySQLError(t, cfg, "update profile", err)
			}
			assertProfilePlayerLevelMySQLValue(t, cfg, database, "mutable", tc.valid, tc.level)
		})
		if !ok {
			t.Fatal("fresh profile projection failed")
		}
	}
	player := `{"level":70}`
	ignoredLevel := int64(999)
	profile := &Profile{Uid: "gorm-save", Session: []byte{1}, Player: &player, PlayerLevel: &ignoredLevel}
	if err := game.Create(profile).Error; err != nil {
		profilePlayerLevelMySQLError(t, cfg, "gorm create", err)
	}
	assertProfilePlayerLevelMySQLValue(t, cfg, database, profile.Uid, true, 70)
	updatedPlayer := `{"level":2147483647}`
	profile.Player, profile.PlayerLevel = &updatedPlayer, &ignoredLevel
	if err := game.Save(profile).Error; err != nil {
		profilePlayerLevelMySQLError(t, cfg, "gorm save", err)
	}
	assertProfilePlayerLevelMySQLValue(t, cfg, database, profile.Uid, true, 2147483647)

	// All DDL below affects only the table created by this test in the empty schema.
	if _, err := database.ExecContext(ctx, "DROP INDEX idx_profiles_server_level ON profiles"); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "remove owned index for repair", err)
	}
	for i := 0; i < 2; i++ {
		if err := migrateProfilePlayerLevel(game.WithContext(ctx)); err != nil {
			profilePlayerLevelMySQLError(t, cfg, "repair existing-column index", err)
		}
		assertProfilePlayerLevelMySQLSchema(t, cfg, database)
	}
	if _, err := database.ExecContext(ctx, "DROP INDEX idx_profiles_server_level ON profiles"); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "remove owned index for legacy fixture", err)
	}
	if _, err := database.ExecContext(ctx, "ALTER TABLE profiles DROP COLUMN player_level"); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "remove owned projection for legacy fixture", err)
	}
	var columnCount, indexCount int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles' AND COLUMN_NAME='player_level'`).Scan(&columnCount); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "check legacy column", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles' AND INDEX_NAME='idx_profiles_server_level'`).Scan(&indexCount); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "check legacy index", err)
	}
	if columnCount != 0 || indexCount != 0 {
		t.Fatalf("legacy fixture still has the projection: columns=%d indexes=%d", columnCount, indexCount)
	}
	if err := pool.Close(); err != nil {
		profilePlayerLevelMySQLError(t, cfg, "close fresh game pool", err)
	}
	for i := 0; i < 2; i++ {
		migrated, err := OpenGameMySQL(ctx, cfg, true)
		if err != nil {
			profilePlayerLevelMySQLError(t, cfg, "migrate legacy profile schema", err)
		}
		migratedPool, err := migrated.DB()
		if err != nil {
			profilePlayerLevelMySQLError(t, cfg, "get migrated game pool", err)
		}
		t.Cleanup(func() { _ = migratedPool.Close() })
		assertProfilePlayerLevelMySQLSchema(t, cfg, database)
		assertProfilePlayerLevelMySQLValue(t, cfg, database, "insert-seventy", true, 70)
		assertProfilePlayerLevelMySQLValue(t, cfg, database, "insert-maximum", true, 2147483647)
		assertProfilePlayerLevelMySQLValue(t, cfg, database, "insert-invalid-json", false, 0)
		assertProfilePlayerLevelMySQLValue(t, cfg, database, "insert-string", false, 0)
		assertProfilePlayerLevelMySQLValue(t, cfg, database, profile.Uid, true, 2147483647)
		if err := migratedPool.Close(); err != nil {
			profilePlayerLevelMySQLError(t, cfg, "close migrated game pool", err)
		}
	}
}
