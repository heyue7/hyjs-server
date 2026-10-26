package service

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
	"hyjs-server/db"
)

func TestP13UFOriginalDigestMySQLHotCacheStillChecksStoredBytes(t *testing.T) {
	database := os.Getenv("HYJS_P13_MYSQL_DATABASE")
	if database == "" {
		t.Skip("explicit registered isolated P13 MySQL test schema required")
	}
	if !strings.HasPrefix(database, "hyjs_test_p13_") || strings.Contains(database, "_guild_ui_") {
		t.Fatal("digest corruption test requires its own isolated P13 schema; retained UI runtime is forbidden")
	}
	f, _, _, _, match := p13UFFieldSetup(t)
	if db.GetIns().Dialector.Name() != "mysql" {
		t.Fatal("MySQL digest test did not use requested isolated schema")
	}
	day, err := p13UFRegistrationDay(db.GetIns(), match.Day)
	if err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	if err = db.GetIns().First(&entry, "guild_id=? AND day=?", match.LeftGuildID, day).Error; err != nil {
		t.Fatal(err)
	}
	originalBytes := append([]byte(nil), entry.FrozenTables...)
	// Keep destructive proof checks local to this real fixture and restore even
	// when an assertion fails before the final missing-source check.
	t.Cleanup(func() {
		if err := db.GetIns().Save(&entry).Error; err != nil {
			t.Error("restore original entry fixture", err)
		}
	})
	var expected map[string][]map[string]string
	if err = json.Unmarshal(originalBytes, &expected); err != nil {
		t.Fatal(err)
	}
	warmed, err := f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(warmed.Tables, expected) {
		t.Fatal("warm original table content differs from frozen real entry")
	}
	var selects [][]string
	callback := "p13-original-hot-digest-projection"
	if err = db.GetIns().Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p13_union_fight_entries" {
			selects = append(selects, append([]string(nil), tx.Statement.Selects...))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Callback().Query().Remove(callback) })
	for i := 0; i < 3; i++ {
		hot, e := f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx))
		if e != nil || hot != warmed || !reflect.DeepEqual(hot.Tables, expected) {
			t.Fatal("hot table identity/content changed", i, e)
		}
	}
	if err = db.GetIns().Callback().Query().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if len(selects) != 3 {
		t.Fatal("hot original read did not make exactly one source-digest query per call", selects)
	}
	digest := regexp.MustCompile(`lower\(sha2\(\s*frozen_tables\s*,\s*256\s*\)\)`)
	for _, projection := range selects {
		selected := strings.ToLower(strings.Join(projection, ","))
		if selected == "" || strings.Contains(selected, "*") || !digest.MatchString(selected) || strings.Contains(digest.ReplaceAllString(selected, "digest-expression"), "frozen_tables") || !strings.Contains(selected, "client_version") || !strings.Contains(selected, "engine_version") {
			t.Fatal("hot read selected a source blob or omitted actual-byte/version verification", projection)
		}
	}
	query := func() *gorm.DB {
		return db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=? AND day=?", entry.GuildID, entry.Day)
	}
	reject := func(reason string) {
		t.Helper()
		if _, e := f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx)); e == nil {
			t.Fatal("warm cache accepted changed source", reason)
		}
	}
	// Trailing whitespace preserves JSON meaning but changes actual stored bytes.
	// Match.ConfigurationHash remains the genuine original frozen producer hash.
	if err = query().Update("frozen_tables", append(append([]byte(nil), originalBytes...), byte(' '))).Error; err != nil {
		t.Fatal(err)
	}
	reject("actual bytes")
	if err = query().Update("frozen_tables", originalBytes).Error; err != nil {
		t.Fatal(err)
	}
	restored, err := f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx))
	if err != nil || restored != warmed {
		t.Fatal("restored source failed original hot cache", err)
	}
	for _, version := range []struct{ column, value string }{{"client_version", entry.ClientVersion}, {"engine_version", entry.EngineVersion}} {
		if err = query().Update(version.column, version.value+"-corrupt-fixture").Error; err != nil {
			t.Fatal(err)
		}
		reject(version.column)
		if err = query().Update(version.column, version.value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().Where("guild_id=? AND day=?", entry.GuildID, entry.Day).Delete(&db.P13UnionFightEntry{}).Error; err != nil {
		t.Fatal(err)
	}
	reject("missing source")
	if !bytes.Equal(entry.FrozenTables, originalBytes) {
		t.Fatal("test mutated its original source backup")
	}
}

func TestP13UFOriginalDigestSQLiteFallbackRetainsSourceCheck(t *testing.T) {
	if os.Getenv("HYJS_P13_MYSQL_DATABASE") != "" {
		t.Skip("SQLite fallback runs only without an explicit MySQL allocation")
	}
	f, _, _, _, match := p13UFFieldSetup(t)
	if db.GetIns().Dialector.Name() != "sqlite" {
		t.Fatal("SQLite fallback requires SQLite fixture")
	}
	warmed, err := f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	day, err := p13UFRegistrationDay(db.GetIns(), match.Day)
	if err != nil {
		t.Fatal(err)
	}
	var entry db.P13UnionFightEntry
	if err = db.GetIns().First(&entry, "guild_id=? AND day=?", match.LeftGuildID, day).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Save(&entry).Error; err != nil {
			t.Error(err)
		}
	})
	var expected map[string][]map[string]string
	if err = json.Unmarshal(entry.FrozenTables, &expected); err != nil || !reflect.DeepEqual(warmed.Tables, expected) {
		t.Fatal("SQLite original frozen tables", err)
	}
	if err = db.GetIns().Model(&db.P13UnionFightEntry{}).Where("guild_id=? AND day=?", entry.GuildID, entry.Day).Update("frozen_tables", append(append([]byte(nil), entry.FrozenTables...), byte(' '))).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p13UFFieldOriginal(match, db.GetIns().WithContext(f.ctx)); err == nil {
		t.Fatal("SQLite warm cache skipped stored-byte SHA")
	}
}
