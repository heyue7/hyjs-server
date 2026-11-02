package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// These cases require a separately managed InnoDB fixture. SQLite does not
// demonstrate the row-lock and committed-read behavior covered here.
func sharedFieldBossReadsRound2Fixture(t *testing.T) *p1Fixture {
	t.Helper()
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeSharedFieldBoss55(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

type sharedFieldBossReadsRound2Result struct {
	row  *db.SharedFieldBoss55
	boss *pb.SideBoss
	hp   map[string]float64
	err  error
}

func sharedFieldBossReadsRound2Read(f *p1Fixture, ctx context.Context, scene, pos string, line, now int64) sharedFieldBossReadsRound2Result {
	var result sharedFieldBossReadsRound2Result
	result.err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		result.row, result.boss, result.hp, err = f.s.sharedFieldBoss55Read(tx, scene, pos, line, now)
		return err
	})
	return result
}

func TestSharedFieldBoss55ReadsRound2MySQLStableProjection(t *testing.T) {
	f := sharedFieldBossReadsRound2Fixture(t)
	peer := p1GatherSecondPlayer(t, f)
	database := db.GetIns()
	const scene = "field_001"
	seed := &model.GameState{Field: &model.FieldState{Scene: scene}}
	if err := database.Transaction(func(tx *gorm.DB) error {
		return f.s.refreshSharedFieldBosses55(tx, f.id, seed)
	}); err != nil {
		t.Fatal(err)
	}
	if len(seed.Field.Bosses) != 1 {
		t.Fatalf("expected one fixture boss, got %d", len(seed.Field.Bosses))
	}
	boss := seed.Field.Bosses[0]
	pos, line := boss.GetBossPosId(), seed.Field.BossLine85
	if pos == "" || line < 1 {
		t.Fatal("fixture did not project a boss position and line")
	}
	committedHP := map[string]float64{"round2-member": 50}
	committedRaw, err := json.Marshal(committedHP)
	if err != nil {
		t.Fatal(err)
	}
	updated := sharedFieldBoss55Query(database.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), scene, pos, line).
		Update("hp", committedRaw)
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("seed committed hp: rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	txA := database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if txA.Error != nil {
		cancel()
		t.Fatal(txA.Error)
	}
	type projection struct {
		boss *pb.SideBoss
		hp   map[string]float64
		err  error
	}
	var pending <-chan projection
	defer func() {
		_ = txA.Rollback().Error
		cancel()
		if pending != nil {
			<-pending
		}
	}()
	var locked db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(txA.Clauses(clause.Locking{Strength: "UPDATE"}), f.s.OriginalServerID(), scene, pos, line).First(&locked).Error; err != nil {
		t.Fatal(err)
	}
	uncommittedHP := map[string]float64{"round2-member": 7}
	uncommittedRaw, err := json.Marshal(uncommittedHP)
	if err != nil {
		t.Fatal(err)
	}
	updated = sharedFieldBoss55Query(txA.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), scene, pos, line).
		Update("hp", uncommittedRaw)
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("write held boss row: rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	result := make(chan projection, 1)
	pending = result
	go func() {
		var p projection
		p.err = updateProfileWithTx(ctx, peer, func(tx *gorm.DB, _ *pb.Player, g *model.GameState) error {
			g.Field = &model.FieldState{Scene: scene}
			if err := f.s.refreshSharedFieldBosses55(tx, peer, g); err != nil {
				return err
			}
			if len(g.Field.Bosses) != 1 {
				return errors.New("independent profile projected an unexpected number of bosses")
			}
			p.boss = proto.Clone(g.Field.Bosses[0]).(*pb.SideBoss)
			p.hp = g.Field.BossHP[p.boss.GetUuid()]
			return nil
		})
		result <- p
	}()
	var got projection
	select {
	case got = <-pending:
		pending = nil
	case <-time.After(2 * time.Second):
		t.Fatal("independent profile projection waited for the held boss row")
	case <-ctx.Done():
		t.Fatalf("independent profile projection context ended: %v", ctx.Err())
	}
	if got.err != nil || got.boss == nil || !proto.Equal(got.boss, boss) || !reflect.DeepEqual(got.hp, committedHP) {
		t.Fatalf("projection did not read the committed generation and hp: boss=%v hp=%v err=%v", got.boss, got.hp, got.err)
	}
	var peerRow db.Profile
	if err := database.WithContext(ctx).First(&peerRow, peer).Error; err != nil {
		t.Fatal(err)
	}
	_, saved, err := decodeProfile(&peerRow)
	if err != nil || saved.Field == nil || len(saved.Field.Bosses) != 1 || !proto.Equal(saved.Field.Bosses[0], boss) || !reflect.DeepEqual(saved.Field.BossHP[boss.GetUuid()], committedHP) {
		t.Fatalf("independent committed profile lost the boss projection: %v", err)
	}
	if err := txA.Commit().Error; err != nil {
		t.Fatal(err)
	}
	after := sharedFieldBossReadsRound2Read(f, ctx, scene, pos, line, time.Now().UnixMilli())
	if after.err != nil || after.row == nil || after.boss == nil || after.row.UUID != boss.GetUuid() || !reflect.DeepEqual(after.hp, uncommittedHP) {
		t.Fatalf("new transaction did not see the committed writer: hp=%v err=%v", after.hp, after.err)
	}
}

func TestSharedFieldBoss55ReadsRound2MySQLCreateExpireCorrupt(t *testing.T) {
	f := sharedFieldBossReadsRound2Fixture(t)
	database := db.GetIns()
	const scene, pos = "field_001", "boss_pos_001"
	line, now := time.Now().UnixNano(), time.Now().UnixMilli()
	var absent db.SharedFieldBoss55
	err := sharedFieldBoss55Query(database, f.s.OriginalServerID(), scene, pos, line).First(&absent).Error
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("new line was not empty: %v", err)
	}
	created := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
	if created.err != nil || created.row == nil || created.boss == nil || created.row.UUID == "" || created.row.UUID != created.boss.GetUuid() || created.boss.GetBossPosId() != pos || created.boss.GetState() != 1 || created.row.Line != line {
		t.Fatalf("missing generation was not created: %+v", created)
	}
	repeated := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
	if repeated.err != nil || repeated.row == nil || repeated.row.UUID != created.row.UUID || !proto.Equal(repeated.boss, created.boss) {
		t.Fatalf("repeat read changed the created generation: %+v", repeated)
	}
	var count int64
	if err := sharedFieldBoss55Query(database.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), scene, pos, line).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("missing generation created %d rows: %v", count, err)
	}
	dead := *created.row
	dead.RespawnAt, dead.KillerID = now-1, f.id
	dead.DeathRules73 = []byte(`{"deathSettlement":true}`)
	deadBoss := proto.Clone(created.boss).(*pb.SideBoss)
	deadBoss.State, deadBoss.CurHp = proto.Int32(3), proto.Float64(0)
	if err := database.Transaction(func(tx *gorm.DB) error {
		return saveSharedFieldBoss55(tx, &dead, deadBoss, map[string]float64{"round2-member": 0})
	}); err != nil {
		t.Fatal(err)
	}
	fresh := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
	if fresh.err != nil || fresh.row == nil || fresh.boss == nil || fresh.row.UUID == dead.UUID || fresh.row.UUID != fresh.boss.GetUuid() || fresh.boss.GetState() != 1 || fresh.boss.GetCurHp() != 1 || fresh.row.RespawnAt != 0 || fresh.row.KillerID != 0 || len(fresh.row.DeathRules73) != 0 || len(fresh.hp) != 0 {
		t.Fatalf("expired generation was not replaced cleanly: %+v", fresh)
	}
	again := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
	if again.err != nil || again.row == nil || again.row.UUID != fresh.row.UUID || !proto.Equal(again.boss, fresh.boss) {
		t.Fatalf("stable replacement changed on a later read: %+v", again)
	}
	for _, tc := range []struct {
		name      string
		payload   []byte
		hp        []byte
		respawnAt int64
	}{
		{name: "payload", payload: []byte("{"), hp: fresh.row.HP},
		{name: "hp", payload: fresh.row.Payload, hp: []byte("{")},
		{name: "expired-payload", payload: []byte("{"), hp: fresh.row.HP, respawnAt: now - 1},
		{name: "expired-hp", payload: fresh.row.Payload, hp: []byte("{"), respawnAt: now - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			updated := sharedFieldBoss55Query(database.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), scene, pos, line).
				Updates(map[string]any{"payload": tc.payload, "hp": tc.hp, "respawn_at": tc.respawnAt})
			if updated.Error != nil || updated.RowsAffected != 1 {
				t.Fatalf("seed corrupt %s: rows=%d err=%v", tc.name, updated.RowsAffected, updated.Error)
			}
			bad := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
			if bad.err == nil {
				t.Fatalf("accepted corrupt %s", tc.name)
			}
			var persisted db.SharedFieldBoss55
			if err := sharedFieldBoss55Query(database, f.s.OriginalServerID(), scene, pos, line).First(&persisted).Error; err != nil {
				t.Fatal(err)
			}
			if persisted.UUID != fresh.row.UUID || persisted.RespawnAt != tc.respawnAt || string(persisted.Payload) != string(tc.payload) || string(persisted.HP) != string(tc.hp) {
				t.Fatalf("corrupt %s was silently changed", tc.name)
			}
		})
	}
}

type sharedFieldBossReadsRound2ContextKey struct{}

func TestSharedFieldBoss55ReadsRound2MySQLExpiredRechecksLockedRow(t *testing.T) {
	f := sharedFieldBossReadsRound2Fixture(t)
	database := db.GetIns()
	const scene, pos = "field_001", "boss_pos_001"
	line, now := time.Now().UnixNano(), time.Now().UnixMilli()
	created := sharedFieldBossReadsRound2Read(f, f.ctx, scene, pos, line, now)
	if created.err != nil || created.row == nil || created.boss == nil {
		t.Fatalf("create original generation: %+v", created)
	}
	old := *created.row
	old.RespawnAt, old.KillerID = now-1, f.id
	oldBoss := proto.Clone(created.boss).(*pb.SideBoss)
	oldBoss.State, oldBoss.CurHp = proto.Int32(3), proto.Float64(0)
	if err := database.Transaction(func(tx *gorm.DB) error {
		return saveSharedFieldBoss55(tx, &old, oldBoss, map[string]float64{"round2-member": 0})
	}); err != nil {
		t.Fatal(err)
	}
	newBoss, err := f.s.fieldBossSpawn(pos)
	if err != nil {
		t.Fatal(err)
	}
	newPayload, err := json.Marshal(newBoss)
	if err != nil {
		t.Fatal(err)
	}
	newHP := map[string]float64{"round2-member": 42}
	newRaw, err := json.Marshal(newHP)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	txA := database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if txA.Error != nil {
		cancel()
		t.Fatal(txA.Error)
	}
	var pending <-chan sharedFieldBossReadsRound2Result
	registered := false
	const callback = "test:shared-field-boss-round2-expired-lock"
	defer func() {
		_ = txA.Rollback().Error
		cancel()
		if pending != nil {
			<-pending
		}
		if registered {
			if err := database.Callback().Query().Remove(callback); err != nil {
				t.Errorf("remove lock callback: %v", err)
			}
		}
	}()
	var held db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(txA.Clauses(clause.Locking{Strength: "UPDATE"}), f.s.OriginalServerID(), scene, pos, line).First(&held).Error; err != nil {
		t.Fatal(err)
	}
	updated := sharedFieldBoss55Query(txA.Model(&db.SharedFieldBoss55{}), f.s.OriginalServerID(), scene, pos, line).
		Updates(map[string]any{"uuid": newBoss.GetUuid(), "payload": newPayload, "hp": newRaw, "respawn_at": int64(0), "killer_id": int64(0), "death_rules73": nil})
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("prepare concurrent replacement: rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	lockAttempted := make(chan struct{})
	var once sync.Once
	if err := database.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "shared_field_boss55" || tx.Statement.Context.Value(sharedFieldBossReadsRound2ContextKey{}) != true {
			return
		}
		locking, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if ok && locking.Strength == "UPDATE" {
			once.Do(func() { close(lockAttempted) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	registered = true
	result := make(chan sharedFieldBossReadsRound2Result, 1)
	pending = result
	go func() {
		workerCtx := context.WithValue(ctx, sharedFieldBossReadsRound2ContextKey{}, true)
		result <- sharedFieldBossReadsRound2Read(f, workerCtx, scene, pos, line, now)
	}()
	select {
	case <-lockAttempted:
	case got := <-pending:
		pending = nil
		t.Fatalf("expired reader completed before the locked re-read: %+v", got)
	case <-time.After(2 * time.Second):
		t.Fatal("expired reader did not attempt its locked re-read")
	case <-ctx.Done():
		t.Fatalf("expired reader context ended: %v", ctx.Err())
	}
	select {
	case got := <-pending:
		pending = nil
		t.Fatalf("expired reader passed a held row lock: %+v", got)
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatalf("expired reader context ended while row was held: %v", ctx.Err())
	}
	if err := txA.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var got sharedFieldBossReadsRound2Result
	select {
	case got = <-pending:
		pending = nil
	case <-ctx.Done():
		t.Fatalf("expired reader did not finish after release: %v", ctx.Err())
	}
	if got.err != nil || got.row == nil || got.boss == nil || got.row.UUID != newBoss.GetUuid() || !proto.Equal(got.boss, newBoss) || got.row.RespawnAt != 0 || got.row.KillerID != 0 || !reflect.DeepEqual(got.hp, newHP) {
		t.Fatalf("locked re-read did not use the replacement committed by the writer: %+v", got)
	}
	var persisted db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(database, f.s.OriginalServerID(), scene, pos, line).First(&persisted).Error; err != nil || persisted.UUID != newBoss.GetUuid() || persisted.RespawnAt != 0 {
		t.Fatalf("reader overwrote the committed replacement: uuid=%s err=%v", persisted.UUID, err)
	}
}
