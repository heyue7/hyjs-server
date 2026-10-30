package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func profileRollback86MySQLInput(t *testing.T) (string, string) {
	t.Helper()
	hero := `{"id":"hero_001","guid":"owned-fixture","futureHero":{"exact":9007199254740993}}`
	legacy := `{"futureTop":{"exact":9223372036854775807},"warriors":[` + strings.TrimSuffix(strings.Repeat(hero+",", 400), ",") + `]}`
	stored, err := model.CompactProfileGame86([]byte(legacy))
	if err != nil || !bytes.Contains(stored, []byte(constUtil.ProfileWarriorsCodec86)) {
		t.Fatal("cannot construct compressed isolated profile")
	}
	return legacy, string(stored)
}

func profileRollback86MySQLInsert(t *testing.T, database *gorm.DB, id int64, game *string) {
	t.Helper()
	player := fmt.Sprintf(`{"fixture":%d}`, id)
	createdAt := int64(123456789)
	row := db.Profile{
		Id: id, Uid: fmt.Sprintf("rollback86-mysql-%d", id), Session: []byte{0, 1, byte(id)},
		Player: &player, Game: game, GGOCredit: 73, GGOCreditMigrated: true,
		AccountID: 42, AccountAuthRevision: 3, ServerID: 1, CreatedAt: &createdAt,
		LastSproutLoginAt: 123, ReturnSproutStartedAt: 456,
		PasswordSalt: []byte("isolated-salt"), PasswordHash: []byte("isolated-hash"),
	}
	if err := database.Create(&row).Error; err != nil {
		t.Fatal("cannot create isolated profile fixture")
	}
}

func profileRollback86MySQLRead(t *testing.T, database *gorm.DB, ids []int64) map[int64]db.Profile {
	t.Helper()
	var rows []db.Profile
	if err := database.Where("id IN ?", ids).Order("id ASC").Find(&rows).Error; err != nil || len(rows) != len(ids) {
		t.Fatal("cannot read isolated profile fixtures")
	}
	out := make(map[int64]db.Profile, len(rows))
	for _, row := range rows {
		out[row.Id] = row
	}
	return out
}

func profileRollback86MySQLOnlyGameChanged(t *testing.T, before, after db.Profile) {
	t.Helper()
	before.Game = after.Game
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a non-game profile column changed")
	}
}

type profileRollback86MySQLContextKey struct{}

type profileRollback86MySQLResult struct {
	summary ProfileStorageRollback86Summary
	err     error
}

// This concurrency check uses the original profile row and a separate InnoDB
// transaction. It verifies lock waiting without asserting a performance target.
func profileRollback86MySQLConcurrent(t *testing.T, database *gorm.DB, id, afterID int64, original db.Profile) {
	t.Helper()
	lockCtx, cancelLock := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelLock()
	holder := database.WithContext(lockCtx).Begin()
	if holder.Error != nil {
		t.Fatal("cannot begin the isolated lock holder")
	}
	t.Cleanup(func() { _ = holder.Rollback().Error })
	var held db.Profile
	if err := holder.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&held).Error; err != nil {
		t.Fatal("cannot lock isolated profile")
	}
	changedPlayer := `{"fixture":"concurrent-update"}`
	changedSession := []byte{9, 8, 7, 6}
	changedCredit := int64(314159)
	update := holder.Model(&db.Profile{}).Where("id = ?", id).Updates(map[string]any{
		"player": changedPlayer, "session": changedSession, "ggo_credit": changedCredit,
	})
	if update.Error != nil || update.RowsAffected != 1 {
		t.Fatal("cannot update locked isolated profile")
	}

	observed := make(chan struct{}, 2)
	name := "profile_storage_rollback86_mysql_lock_observer"
	if err := database.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(profileRollback86MySQLContextKey{}) != true || tx.Statement.Table != "profiles" {
			return
		}
		locked, ok := tx.Statement.Clauses["FOR"]
		if !ok {
			return
		}
		if value, ok := locked.Expression.(clause.Locking); !ok || value.Strength != "UPDATE" {
			return
		}
		select {
		case observed <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal("cannot install isolated query observer")
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(name) })

	start := func(ctx context.Context) <-chan profileRollback86MySQLResult {
		result := make(chan profileRollback86MySQLResult, 1)
		go func() {
			summary, err := RunProfileStorageRollback86(ctx, database, ProfileStorageRollback86Options{
				After: &afterID, BatchSize: 1, MaxRows: 1, Apply: true,
			}, nil)
			result <- profileRollback86MySQLResult{summary: summary, err: err}
		}()
		return result
	}
	awaitQuery := func(ctx context.Context, result <-chan profileRollback86MySQLResult) {
		t.Helper()
		select {
		case <-observed:
		case <-result:
			t.Fatal("rollback ended before reaching the locked profile query")
		case <-ctx.Done():
			t.Fatal("rollback did not reach the locked profile query")
		}
	}

	// The holder remains locked until this bounded call reaches its deadline.
	// An unrelated early failure is rejected by checking the context outcome.
	base := context.WithValue(t.Context(), profileRollback86MySQLContextKey{}, true)
	waitCtx, cancelWait := context.WithTimeout(base, 3*time.Second)
	defer cancelWait()
	blocked := start(waitCtx)
	awaitQuery(waitCtx, blocked)
	select {
	case result := <-blocked:
		if result.err == nil || !errors.Is(waitCtx.Err(), context.DeadlineExceeded) || result.summary.Scanned != 0 {
			t.Fatal("locked profile was not held until cancellation")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("cancelled rollback did not exit")
	}

	// A cancelled MySQL client closes its connection, but the server can keep
	// its blocked statement until the blocking transaction releases the row.
	// Release that blocker and cross the maintenance lock before a fresh run.
	if err := holder.Commit().Error; err != nil {
		t.Fatal("cannot release the cancelled query blocker")
	}
	barrierCtx, cancelBarrier := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancelBarrier()
	if err := database.WithContext(barrierCtx).Transaction(func(tx *gorm.DB) error {
		var policy db.ClientUpdatePolicy
		return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", 1).Take(&policy).Error
	}); err != nil {
		t.Fatal("cancelled server transaction did not release maintenance")
	}
	holder = database.WithContext(lockCtx).Begin()
	if holder.Error != nil || holder.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&held).Error != nil {
		t.Fatal("cannot establish the second isolated profile lock")
	}
	// A fresh operation must wait on the actual row and preserve the first
	// blocker's committed non-game edit once the second lock is released.
	resumeCtx, cancelResume := context.WithTimeout(base, 12*time.Second)
	defer cancelResume()
	resumed := start(resumeCtx)
	awaitQuery(resumeCtx, resumed)
	if err := holder.Commit().Error; err != nil {
		t.Fatal("cannot commit concurrent profile edit")
	}
	select {
	case result := <-resumed:
		if result.err != nil || result.summary.Scanned != 1 || result.summary.Expanded != 1 || result.summary.NextAfter == nil || *result.summary.NextAfter != id {
			t.Fatal("rollback did not complete after the lock release")
		}
	case <-resumeCtx.Done():
		t.Fatal("rollback did not complete after the lock release")
	}
	actual := profileRollback86MySQLRead(t, database, []int64{id})[id]
	if original.Game == nil {
		t.Fatal("compressed input missing")
	}
	expanded, err := model.ExpandProfileGame86([]byte(*original.Game))
	if err != nil {
		t.Fatal("cannot independently expand the fixture")
	}
	game := string(expanded)
	original.Game = &game
	original.Player = &changedPlayer
	original.Session = changedSession
	original.GGOCredit = changedCredit
	if !reflect.DeepEqual(original, actual) {
		t.Fatal("rollback lost the committed concurrent profile change")
	}
}

// TestProfileStorageRollback86MySQLInnoDB requires a fresh managed P1 schema.
// The runner owns and releases that schema; no production account is involved.
func TestProfileStorageRollback86MySQLInnoDB(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("isolated managed MySQL allocation required")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || len(name) == len("hyjs_test_p1_") {
		t.Fatal("isolated P1 database required")
	}
	f := p1Setup(t)
	database := db.GetIns()
	if database.Dialector.Name() != "mysql" {
		t.Fatal("MySQL connection required")
	}
	var selected string
	if err := database.Raw("SELECT DATABASE()").Scan(&selected).Error; err != nil || selected != name {
		t.Fatal("connected database differs from the owned allocation")
	}
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal("cannot initialize isolated maintenance policy")
	}
	for _, table := range []string{"profiles", "client_update_policy"} {
		var engine string
		if err := database.Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", table).Scan(&engine).Error; err != nil || engine != "InnoDB" {
			t.Fatal("isolated table does not use InnoDB")
		}
	}
	maintenance := func(enabled bool) {
		t.Helper()
		result := database.Model(&db.ClientUpdatePolicy{}).Where("id = ?", 1).UpdateColumn("maintenance", enabled)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("cannot set isolated maintenance state")
		}
	}
	maintenance(true)

	const (
		firstID  int64 = -200
		nullID   int64 = -199
		legacyID int64 = -198
		secondID int64 = -197
		goodID   int64 = -196
		badID    int64 = -195
		lockedID int64 = -194
	)
	legacy, stored := profileRollback86MySQLInput(t)
	expanded, err := model.ExpandProfileGame86([]byte(stored))
	if err != nil {
		t.Fatal("cannot independently expand the fixture")
	}
	bad := `{"warriors":{"codec":"hyjs-warriors-zlib-v1","bytes":1,"sha256":"bad","data":"AA=="}}`
	if f.id <= 0 {
		t.Fatal("P1 fixture must have a positive identity")
	}
	ids := []int64{firstID, nullID, legacyID, secondID, goodID, badID, lockedID, f.id}
	for _, row := range []struct {
		id   int64
		game *string
	}{{firstID, &stored}, {nullID, nil}, {legacyID, &legacy}, {secondID, &stored}, {goodID, &stored}, {badID, &bad}, {lockedID, &stored}} {
		profileRollback86MySQLInsert(t, database, row.id, row.game)
	}
	original := profileRollback86MySQLRead(t, database, ids)
	var previews []ProfileStorageRollback86Batch
	options := ProfileStorageRollback86Options{BatchSize: 2, MaxRows: 4}
	preview, err := RunProfileStorageRollback86(t.Context(), database, options, func(batch ProfileStorageRollback86Batch) error {
		previews = append(previews, batch)
		return nil
	})
	if err != nil || preview.Scanned != 4 || preview.Expanded != 2 || preview.Exhausted || preview.NextAfter == nil || *preview.NextAfter != secondID ||
		len(previews) != 2 || previews[0].Rows[0].ID != firstID || previews[0].Rows[1].ID != nullID || !previews[0].Rows[1].GameNull ||
		previews[0].Rows[0].BeforeSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(stored))) ||
		previews[0].Rows[0].AfterSHA256 != fmt.Sprintf("%x", sha256.Sum256(expanded)) ||
		!reflect.DeepEqual(original, profileRollback86MySQLRead(t, database, ids)) {
		t.Fatal("preview changed data or did not scan the negative IDs")
	}

	options.Apply = true
	var committed []ProfileStorageRollback86Batch
	stopped, err := RunProfileStorageRollback86(t.Context(), database, options, func(batch ProfileStorageRollback86Batch) error {
		committed = append(committed, batch)
		return errors.New("simulate a report interruption after commit")
	})
	if err == nil || stopped.Scanned != 2 || stopped.Expanded != 1 || stopped.NextAfter == nil || *stopped.NextAfter != nullID ||
		len(committed) != 1 || !reflect.DeepEqual(previews[0], committed[0]) {
		t.Fatal("committed cursor was lost on report interruption")
	}
	options.After = stopped.NextAfter
	options.MaxRows = 2
	resumed, err := RunProfileStorageRollback86(t.Context(), database, options, nil)
	if err != nil || resumed.Scanned != 2 || resumed.Expanded != 1 || resumed.NextAfter == nil || *resumed.NextAfter != secondID {
		t.Fatal("bounded cursor resume failed")
	}
	converted := profileRollback86MySQLRead(t, database, ids)
	for _, id := range ids {
		profileRollback86MySQLOnlyGameChanged(t, original[id], converted[id])
	}
	for _, id := range []int64{goodID, badID, lockedID, f.id} {
		if !reflect.DeepEqual(original[id], converted[id]) {
			t.Fatal("profile outside the bounded scan changed")
		}
	}
	if converted[firstID].Game == nil || converted[secondID].Game == nil ||
		*converted[firstID].Game != string(expanded) || *converted[secondID].Game != string(expanded) ||
		converted[nullID].Game != nil || converted[legacyID].Game == nil || *converted[legacyID].Game != legacy {
		t.Fatal("expanded, legacy, or null profile differs")
	}
	var oldReader struct {
		Warriors []json.RawMessage `json:"warriors"`
		Future   json.RawMessage   `json:"futureTop"`
	}
	if err := json.Unmarshal(expanded, &oldReader); err != nil || len(oldReader.Warriors) != 400 ||
		!bytes.Contains(oldReader.Warriors[0], []byte("9007199254740993")) || !bytes.Contains(oldReader.Future, []byte("9223372036854775807")) {
		t.Fatal("old-format reader or exact unknown fields differ")
	}
	options.After = nil
	options.MaxRows = 4
	again, err := RunProfileStorageRollback86(t.Context(), database, options, nil)
	if err != nil || again.Scanned != 4 || again.Expanded != 0 || !reflect.DeepEqual(converted, profileRollback86MySQLRead(t, database, ids)) {
		t.Fatal("repeat apply changed stored bytes")
	}

	// The good row is updated before the following invalid row is decoded;
	// the transaction must roll back both and emit no committed batch.
	after := secondID
	options.After, options.MaxRows = &after, 2
	var badEmitted bool
	failed, err := RunProfileStorageRollback86(t.Context(), database, options, func(ProfileStorageRollback86Batch) error {
		badEmitted = true
		return nil
	})
	if err == nil || failed.Scanned != 0 || badEmitted || !reflect.DeepEqual(converted, profileRollback86MySQLRead(t, database, ids)) {
		t.Fatal("invalid roster did not roll back its entire batch")
	}

	maintenance(false)
	options.After = profileStorageRollback86ID(badID)
	options.MaxRows = 1
	for _, apply := range []bool{false, true} {
		options.Apply = apply
		result, err := RunProfileStorageRollback86(t.Context(), database, options, nil)
		if err == nil || result.Scanned != 0 || !reflect.DeepEqual(converted, profileRollback86MySQLRead(t, database, ids)) {
			t.Fatal("disabled maintenance allowed profile access")
		}
	}
	maintenance(true)
	profileRollback86MySQLConcurrent(t, database, lockedID, badID, converted[lockedID])
}
