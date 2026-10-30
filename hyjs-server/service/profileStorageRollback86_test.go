package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func profileRollback86DB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rollback.sqlite")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	for _, statement := range []string{
		`CREATE TABLE client_update_policy (id INTEGER PRIMARY KEY, maintenance BOOLEAN NOT NULL)`,
		`INSERT INTO client_update_policy (id, maintenance) VALUES (1, 1)`,
		`CREATE TABLE profiles (id INTEGER PRIMARY KEY, game TEXT NULL, player TEXT, session BLOB, ggo_credit INTEGER NOT NULL, extra TEXT)`,
	} {
		if err = database.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func profileRollback86Game(t *testing.T) string {
	t.Helper()
	warrior := `{"id":"hero_001","guid":"stable","unknown":{"large":9007199254740993}}`
	game := `{"position":{"scene":"field_003"},"warriors":[` + strings.TrimSuffix(strings.Repeat(warrior+",", 400), ",") + `],"future":{"keep":9223372036854775807}}`
	stored, err := model.CompactProfileGame86([]byte(game))
	if err != nil || !bytes.Contains(stored, []byte(constUtil.ProfileWarriorsCodec86)) {
		t.Fatal("compressed fixture unavailable", err)
	}
	return string(stored)
}

func profileRollback86Insert(t *testing.T, database *gorm.DB, id int64, game any) {
	t.Helper()
	if err := database.Exec(`INSERT INTO profiles (id, game, player, session, ggo_credit, extra) VALUES (?, ?, ?, ?, ?, ?)`,
		id, game, fmt.Sprintf("player-%d", id), []byte{0, 1, byte(id)}, int64(19), "unknown-column").Error; err != nil {
		t.Fatal(err)
	}
}

type profileRollback86Saved struct {
	ID        int64
	Game      *string
	Player    string
	Session   []byte
	GGOCredit int64
	Extra     string
}

func profileRollback86SavedRows(t *testing.T, database *gorm.DB) []profileRollback86Saved {
	t.Helper()
	var rows []profileRollback86Saved
	if err := database.Table("profiles").Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestProfileStorageRollback86PreviewApplyResumeAndLegacy(t *testing.T) {
	database := profileRollback86DB(t)
	stored := profileRollback86Game(t)
	legacy := " { \"warriors\" : [], \"future\" : 7 } "
	profileRollback86Insert(t, database, -2, stored)
	profileRollback86Insert(t, database, 0, nil)
	profileRollback86Insert(t, database, 3, legacy)
	profileRollback86Insert(t, database, 7, stored)
	original := profileRollback86SavedRows(t, database)
	var preview []ProfileStorageRollback86Batch
	options := ProfileStorageRollback86Options{BatchSize: 2, MaxRows: 2}
	summary, err := RunProfileStorageRollback86(t.Context(), database, options, func(batch ProfileStorageRollback86Batch) error {
		preview = append(preview, batch)
		return nil
	})
	if err != nil || summary.Scanned != 2 || summary.Expanded != 1 || summary.Exhausted || summary.NextAfter == nil || *summary.NextAfter != 0 {
		t.Fatalf("preview summary %+v: %v", summary, err)
	}
	if len(preview) != 1 || preview[0].Rows[0].ID != -2 || !preview[0].Rows[0].Changed ||
		preview[0].Rows[1].ID != 0 || !preview[0].Rows[1].GameNull || !reflect.DeepEqual(original, profileRollback86SavedRows(t, database)) {
		t.Fatal("preview changed rows or skipped nonpositive IDs")
	}
	options.Apply = true
	var applied []ProfileStorageRollback86Batch
	summary, err = RunProfileStorageRollback86(t.Context(), database, options, func(batch ProfileStorageRollback86Batch) error {
		applied = append(applied, batch)
		return nil
	})
	if err != nil || summary.Scanned != 2 || summary.Expanded != 1 || !reflect.DeepEqual(preview, applied) {
		t.Fatalf("apply differs from preview %+v: %v", summary, err)
	}
	afterFirst := profileRollback86SavedRows(t, database)
	if afterFirst[0].Game == nil || *afterFirst[0].Game == stored || bytes.Contains([]byte(*afterFirst[0].Game), []byte(constUtil.ProfileWarriorsCodec86)) {
		t.Fatal("roster was not expanded")
	}
	for index := range original {
		if afterFirst[index].Player != original[index].Player || !bytes.Equal(afterFirst[index].Session, original[index].Session) ||
			afterFirst[index].GGOCredit != original[index].GGOCredit || afterFirst[index].Extra != original[index].Extra {
			t.Fatal("non-game columns changed")
		}
	}
	var legacyReader struct {
		Warriors []json.RawMessage `json:"warriors"`
		Future   json.RawMessage   `json:"future"`
	}
	if err := json.Unmarshal([]byte(*afterFirst[0].Game), &legacyReader); err != nil || len(legacyReader.Warriors) != 400 ||
		!bytes.Contains(legacyReader.Warriors[0], []byte("9007199254740993")) || !bytes.Contains(legacyReader.Future, []byte("9223372036854775807")) {
		t.Fatal("legacy reader or unknown exact integers differ", err)
	}
	options.After = summary.NextAfter
	options.MaxRows = 10
	summary, err = RunProfileStorageRollback86(t.Context(), database, options, nil)
	if err != nil || !summary.Exhausted || summary.Scanned != 2 || summary.Expanded != 1 || *summary.NextAfter != 7 {
		t.Fatalf("resume summary %+v: %v", summary, err)
	}
	finished := profileRollback86SavedRows(t, database)
	if *finished[2].Game != legacy || finished[1].Game != nil {
		t.Fatal("legacy or null bytes changed")
	}
	options.After = nil
	summary, err = RunProfileStorageRollback86(t.Context(), database, options, nil)
	if err != nil || !summary.Exhausted || summary.Scanned != 4 || summary.Expanded != 0 || !reflect.DeepEqual(finished, profileRollback86SavedRows(t, database)) {
		t.Fatal("repeat run changed bytes", summary, err)
	}
}

func TestProfileStorageRollback86FailureRollsBackCurrentBatch(t *testing.T) {
	database := profileRollback86DB(t)
	stored := profileRollback86Game(t)
	for id := int64(1); id <= 3; id++ {
		profileRollback86Insert(t, database, id, stored)
	}
	profileRollback86Insert(t, database, 4, `{"warriors":{"codec":"hyjs-warriors-zlib-v1","bytes":1,"sha256":"bad","data":"AA=="}}`)
	var batches []ProfileStorageRollback86Batch
	summary, err := RunProfileStorageRollback86(t.Context(), database, ProfileStorageRollback86Options{BatchSize: 2, MaxRows: 10, Apply: true}, func(batch ProfileStorageRollback86Batch) error {
		batches = append(batches, batch)
		return nil
	})
	if err == nil || summary.Scanned != 2 || summary.Expanded != 2 || len(batches) != 1 || summary.NextAfter == nil || *summary.NextAfter != 2 {
		t.Fatal("failed batch reported committed", summary, err)
	}
	rows := profileRollback86SavedRows(t, database)
	if *rows[0].Game == stored || *rows[1].Game == stored || *rows[2].Game != stored {
		t.Fatal("previous commit or current rollback differs")
	}
}

func TestProfileStorageRollback86MaintenanceAndDuplicateGuard(t *testing.T) {
	database := profileRollback86DB(t)
	stored := profileRollback86Game(t)
	profileRollback86Insert(t, database, 1, stored)
	profileRollback86Insert(t, database, 2, stored)
	var batches int
	summary, err := RunProfileStorageRollback86(t.Context(), database, ProfileStorageRollback86Options{BatchSize: 1, MaxRows: 10, Apply: true}, func(_ ProfileStorageRollback86Batch) error {
		batches++
		return database.Exec(`UPDATE client_update_policy SET maintenance = 0 WHERE id = 1`).Error
	})
	if err == nil || batches != 1 || summary.Scanned != 1 || *profileRollback86SavedRows(t, database)[1].Game != stored {
		t.Fatal("maintenance was not checked for each batch", summary, err)
	}
	if _, err = RunProfileStorageRollback86(t.Context(), database, ProfileStorageRollback86Options{BatchSize: 1, MaxRows: 1}, nil); err == nil {
		t.Fatal("preview allowed disabled maintenance")
	}
	if err = database.Exec(`UPDATE client_update_policy SET maintenance = 1 WHERE id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	profileRollback86Insert(t, database, 3, `{"warriors":[],"future":1,"future":2}`)
	after := int64(2)
	if _, err = RunProfileStorageRollback86(t.Context(), database, ProfileStorageRollback86Options{After: &after, BatchSize: 1, MaxRows: 1, Apply: true}, nil); err == nil {
		t.Fatal("duplicate top-level fields accepted")
	}
}

func TestProfileStorageRollback86DatabaseFailureRedacted(t *testing.T) {
	database := profileRollback86DB(t)
	stored := profileRollback86Game(t)
	profileRollback86Insert(t, database, 1, stored)
	if err := database.Exec(`CREATE TRIGGER fail_profile BEFORE UPDATE ON profiles BEGIN SELECT RAISE(ABORT, 'secret storage payload'); END`).Error; err != nil {
		t.Fatal(err)
	}
	summary, err := RunProfileStorageRollback86(t.Context(), database, ProfileStorageRollback86Options{BatchSize: 1, MaxRows: 1, Apply: true}, nil)
	if err == nil || strings.Contains(err.Error(), "secret storage payload") || !strings.Contains(err.Error(), "profile 1 update failed") || summary.Scanned != 0 {
		t.Fatal("database failure leaked details or committed", summary, err)
	}
	if *profileRollback86SavedRows(t, database)[0].Game != stored {
		t.Fatal("failed update changed profile")
	}
}

func TestProfileStorageRollback86BoundsAndBytePages(t *testing.T) {
	database := profileRollback86DB(t)
	for _, options := range []ProfileStorageRollback86Options{{BatchSize: 0, MaxRows: 1}, {BatchSize: 51, MaxRows: 1}, {BatchSize: 1, MaxRows: 0}} {
		if _, err := RunProfileStorageRollback86(context.Background(), database, options, nil); err == nil {
			t.Fatal("invalid bounds accepted", options)
		}
	}
	metadata := []profileStorageRollback86Metadata{
		{ID: 1, GameBytes: sql.NullInt64{Int64: 4, Valid: true}},
		{ID: 2, GameBytes: sql.NullInt64{}},
		{ID: 3, GameBytes: sql.NullInt64{Int64: 7, Valid: true}},
	}
	ids, err := profileStorageRollback86Page(metadata, 10)
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatal("bounded prefix differs", ids, err)
	}
	if _, err = profileStorageRollback86Page(metadata[2:], 6); err == nil {
		t.Fatal("oversized single profile accepted")
	}
	metadata[0].GameBytes.Int64 = -1
	if _, err = profileStorageRollback86Page(metadata, 10); err == nil {
		t.Fatal("negative byte length accepted")
	}
}
