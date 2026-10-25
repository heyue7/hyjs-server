package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP13UFPlayerBattleCaptureLegacyHashBytes(t *testing.T) {
	// This exact JSON is the pre-stage schema's zero-value journal encoding.
	legacy := `{"id":"","match_id":"","player_id":0,"target_id":0,"stamp":0,"client_stamp":0,"status":"","fight_id":"","request_hash":"","offer_hash":"","p6_record_id":"","input_hash":"","start_pb":null,"start_hash":"","applied_properties":null,"before":null,"after":null,"terminal":null,"terminal_hash":"","scope_proof":"","dead_player_id":0,"reward_player_id":0,"revive_seconds":0,"created_at":0,"due_at":0,"settled_at":0,"source_hash":""}`
	row := db.P13UnionFightPlayerBattleAttempt{}
	if got := p13UFPlayerBattleHash(&row); got != p6Digest([]byte(legacy)) {
		t.Fatal("empty stage changed legacy bytes", got)
	}
	row.CalculationStage = constUtil.P13UFPlayerBattleCaptured
	raw, err := json.Marshal(row)
	if err != nil || !strings.Contains(string(raw), `"calculation_stage":"captured"`) || p13UFPlayerBattleHash(&row) == p6Digest([]byte(legacy)) {
		t.Fatal("new stage is not bound", err)
	}
}

func TestP13UFPlayerBattleCaptureCancellationColdRetryAndLockBoundary(t *testing.T) {
	f, id, enemy, clock, _ := p13UFPlayerBattle172ActualSetup(t)
	request := &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}
	begin, handled, err := f.s.DispatchP13UnionFight(f.ctx, id, request, 0)
	if err != nil || !handled || len(begin) != 2 {
		t.Fatal("capture admission", err)
	}
	var captured db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().First(&captured, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if captured.CalculationStage != constUtil.P13UFPlayerBattleCaptured || captured.DueAt-captured.CreatedAt != 5000 {
		t.Fatal("capture duration/stage")
	}
	// A single connection proves the calculator holds neither an open DB
	// transaction nor any profile/guild lock while it waits. MySQL exercises
	// the same boundary with actual row locks in the integration suite.
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	previousMax := sqlDB.Stats().MaxOpenConnections
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.SetMaxOpenConns(previousMax) })
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	entered := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- f.s.computeP13UnionFightPlayerBattleWith(ctx, &captured, func(ctx context.Context, _ *model.FrozenBattleInput, _ *p13UFPlayerBattleRules) (*model.CombatFinalHPOutcome, error) {
			close(entered)
			<-ctx.Done()
			return nil, ctx.Err()
		})
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatal("calculator did not receive capture", err)
	case <-time.After(5 * time.Second):
		t.Fatal("capture verification blocked")
	}
	probe, stop := context.WithTimeout(f.ctx, time.Second)
	defer stop()
	err = db.GetIns().WithContext(probe).Transaction(func(tx *gorm.DB) error {
		var guild db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, 1).Error; err != nil {
			return err
		}
		var profile db.Profile
		return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, enemy).Error
	})
	if err != nil {
		t.Fatal("slow calculation retained shared locks/transaction", err)
	}
	// Retries keep the exact pair busy and return the original Begin without
	// creating another battle while a calculator is still running.
	if replies, _, err := f.s.DispatchP13UnionFight(f.ctx, id, request, 0); err != nil || len(replies) != 2 {
		t.Fatal("pending retry blocked by calculation", err)
	}
	if _, _, err := f.s.DispatchP13UnionFight(f.ctx, enemy, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(id)}, 81); err == nil {
		t.Fatal("reverse pair bypassed capture busy")
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled calculator", err)
	}
	var retained db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().First(&retained, "id=?", captured.ID).Error; err != nil || retained.SourceHash != captured.SourceHash || retained.CalculationStage != constUtil.P13UFPlayerBattleCaptured {
		t.Fatal("cancelled work lost durable capture", err)
	}
	// A fresh service resumes solely from the stored frozen input. Disabling
	// admission after acceptance cannot strand or change the existing battle.
	if err = db.GetIns().Model(&db.P13UnionFightPlayerBattlePolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	cold := &GameService{}
	if err = cold.computeP13UnionFightPlayerBattle(f.ctx, &retained); err != nil {
		t.Fatal("cold calculation", err)
	}
	if err = db.GetIns().First(&retained, "id=?", captured.ID).Error; err != nil || retained.CalculationStage != constUtil.P13UFPlayerBattleComputed || retained.Status != constUtil.P13UFPlayerBattlePending {
		t.Fatal("computed journal", err)
	}
	// The optional stage can be absent on a certified legacy receipt. Its hash
	// still verifies and the original due-time/application path remains usable.
	retained.CalculationStage = ""
	retained.SourceHash = p13UFPlayerBattleHash(&retained)
	if err = db.GetIns().Save(&retained).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = cold.p13UFPlayerBattleReceipt(db.GetIns(), &retained, id); err != nil {
		t.Fatal("legacy certified receipt", err)
	}
	if err = cold.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(captured.DueAt-1)); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&retained, "id=?", captured.ID).Error; err != nil || retained.Status != constUtil.P13UFPlayerBattlePending {
		t.Fatal("applied before five seconds", err)
	}
	clock.Store(captured.DueAt)
	if err = cold.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&retained, "id=?", captured.ID).Error; err != nil || retained.Status != constUtil.P13UFPlayerBattleSettled {
		t.Fatal("due real terminal", err)
	}
	hash := retained.SourceHash
	if err = cold.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&retained, "id=?", captured.ID).Error; err != nil || retained.SourceHash != hash {
		t.Fatal("duplicate terminal application", err)
	}
}

func TestP13UFPlayerBattleCaptureCorruptInputNeverComputes(t *testing.T) {
	f, id, enemy, _, _ := p13UFPlayerBattle172ActualSetup(t)
	if _, _, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0); err != nil {
		t.Fatal(err)
	}
	var captured db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&captured, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", captured.P6RecordID).Update("input", []byte("damaged")).Error; err != nil {
		t.Fatal(err)
	}
	called := false
	err := f.s.computeP13UnionFightPlayerBattleWith(f.ctx, &captured, func(context.Context, *model.FrozenBattleInput, *p13UFPlayerBattleRules) (*model.CombatFinalHPOutcome, error) {
		called = true
		return nil, errors.New("must not run")
	})
	if err == nil || called {
		t.Fatal("damaged frozen input reached engine")
	}
	if err = f.s.computeP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(captured.CreatedAt)); err == nil {
		t.Fatal("damaged capture did not report quarantine")
	}
	var after db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&after, "id=?", captured.ID).Error; err != nil || after.SourceHash != captured.SourceHash || after.CalculationStage != constUtil.P13UFPlayerBattleCaptured || !after.CalculationQuarantined || after.CalculationFailures != 0 || after.NextCalculationAt != 0 {
		t.Fatal("corruption fabricated terminal or re-signed source", err)
	}
}

func TestP13UFPlayerBattleCaptureConcurrentPairAdmission(t *testing.T) {
	f, id, enemy, _, _ := p13UFPlayerBattle172ActualSetup(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, pair := range [][2]int64{{id, enemy}, {enemy, id}} {
		go func(actor, target int64) {
			<-start
			_, _, err := f.s.DispatchP13UnionFight(f.ctx, actor, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(target)}, 0)
			results <- err
		}(pair[0], pair[1])
	}
	close(start)
	accepted := 0
	for range 2 {
		if <-results == nil {
			accepted++
		}
	}
	var count int64
	if err := db.GetIns().Model(&db.P13UnionFightPlayerBattleAttempt{}).Where("status=?", constUtil.P13UFPlayerBattlePending).Count(&count).Error; err != nil || count != 1 || accepted != 1 {
		t.Fatal("concurrent pair created duplicate reservations", accepted, count, err)
	}
}

// Scheduler fixtures duplicate only an already accepted immutable input. They
// exercise fairness, persistence and certification without inventing victories
// or bypassing admission as evidence for additional live participants.
func TestP13UFPlayerBattleCaptureRetryFairnessAndColdBackoff(t *testing.T) {
	f, id, enemy, clock, _ := p13UFPlayerBattle172ActualSetup(t)
	if _, _, err := f.s.DispatchP13UnionFight(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0); err != nil {
		t.Fatal(err)
	}
	var source db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&source, "player_id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	input, _, before, _, err := f.s.p13UFPlayerBattleVerify(db.GetIns(), &source)
	if err != nil {
		t.Fatal(err)
	}
	var original db.P6BattleRecord
	if err = db.GetIns().First(&original, "id=?", source.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	var ids []string
	ids = append(ids, source.ID)
	for i := int64(1); i <= 16; i++ {
		row := source
		row.Stamp = i
		row.ID = p6Digest([]byte(fmt.Sprintf("%s/%d/%d", row.MatchID, row.PlayerID, row.Stamp)))
		row.FightID = strconv.FormatInt(p13UFFieldGUID(row.MatchID, "ordinary-player14", row.ID), 10)
		row.P6RecordID = p6Digest([]byte(fmt.Sprintf("%d/%s/%s", row.PlayerID, constUtil.P13UFPlayerBattleSource, row.FightID)))
		var typed pb.SCUFFightRecordReplay
		if err = proto.Unmarshal(row.StartPB, &typed); err != nil {
			t.Fatal(err)
		}
		typed.FightId = proto.String(row.FightID)
		row.StartPB, err = proto.MarshalOptions{Deterministic: true}.Marshal(&typed)
		if err != nil {
			t.Fatal(err)
		}
		row.StartHash = p6Digest(row.StartPB)
		frozen, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		frozen.AppliedProperties = p13UFPlayerBattleProperties(&row, before, &typed)
		raw, err := json.Marshal(frozen)
		if err != nil {
			t.Fatal(err)
		}
		record := original
		record.ID, record.SourceFight = row.P6RecordID, row.FightID
		record.InputHash = p6Digest(raw)
		record.Input, err = encodeP6StoredInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		row.InputHash = record.InputHash
		row.SourceHash = p13UFPlayerBattleHash(&row)
		if err = db.GetIns().Create(&record).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
	}
	sort.Strings(ids)
	normalID := ids[16]
	now := time.UnixMilli(clock.Load())
	calls := 0
	compute := func(ctx context.Context, row *db.P13UnionFightPlayerBattleAttempt) error {
		calls++
		if row.ID == normalID {
			return f.s.computeP13UnionFightPlayerBattle(ctx, row)
		}
		return &p13UFPlayerBattleCalculationError{err: errors.New("isolated transient engine failure")}
	}
	if err = f.s.computeP13UnionFightPlayerBattlesWith(f.ctx, now, compute); err == nil || calls != 16 {
		t.Fatal("first failed batch", calls, err)
	}
	var deferred []db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().Where("calculation_failures>0").Find(&deferred).Error; err != nil || len(deferred) != 16 {
		t.Fatal("deferred failures", len(deferred), err)
	}
	for _, row := range deferred {
		if row.CalculationFailures != 1 || row.NextCalculationAt != now.UnixMilli()+1000 || row.SourceHash != p13UFPlayerBattleHash(&row) {
			t.Fatal("first backoff not durable")
		}
	}
	calls = 0
	if err = f.s.computeP13UnionFightPlayerBattlesWith(f.ctx, now, compute); err != nil || calls != 1 {
		t.Fatal("seventeenth accepted capture starved", calls, err)
	}
	var normal db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().First(&normal, "id=?", normalID).Error; err != nil || normal.CalculationStage != constUtil.P13UFPlayerBattleComputed {
		t.Fatal("seventeenth terminal", err)
	}
	cold := &GameService{}
	calls = 0
	cancelled := func(context.Context, *db.P13UnionFightPlayerBattleAttempt) error { calls++; return context.Canceled }
	if err = cold.computeP13UnionFightPlayerBattlesWith(f.ctx, now.Add(999*time.Millisecond), cancelled); err != nil || calls != 0 {
		t.Fatal("cold worker ignored backoff", calls, err)
	}
	if err = cold.computeP13UnionFightPlayerBattlesWith(f.ctx, now.Add(time.Second), cancelled); err == nil || calls != 16 {
		t.Fatal("due captures not retried", calls, err)
	}
	for _, wanted := range deferred {
		var row db.P13UnionFightPlayerBattleAttempt
		if err = db.GetIns().First(&row, "id=?", wanted.ID).Error; err != nil || row.SourceHash != wanted.SourceHash || row.NextCalculationAt != wanted.NextCalculationAt || row.CalculationFailures != 1 {
			t.Fatal("cancellation postponed accepted retry", err)
		}
	}
	// Persisted retry intervals grow to the published bound and remain bounded
	// after a restart; no terminal, HP or reservation is changed by scheduling.
	backoff := deferred[1]
	for _, milliseconds := range []int64{2000, 4000, 8000, 16000, 30000, 30000} {
		at := backoff.NextCalculationAt
		if err = cold.p13UFPlayerBattleRetryCalculation(f.ctx, &backoff, time.UnixMilli(at)); err != nil {
			t.Fatal("bounded retry update", err)
		}
		if err = db.GetIns().First(&backoff, "id=?", backoff.ID).Error; err != nil || backoff.NextCalculationAt-at != milliseconds || backoff.SourceHash != p13UFPlayerBattleHash(&backoff) || backoff.Status != constUtil.P13UFPlayerBattlePending {
			t.Fatal("retry backoff did not cap", milliseconds, err)
		}
	}
	// A successful retry preserves its failure history while clearing scheduling.
	if err = cold.computeP13UnionFightPlayerBattle(f.ctx, &deferred[0]); err != nil {
		t.Fatal("successful deferred retry", err)
	}
	var successful db.P13UnionFightPlayerBattleAttempt
	if err = db.GetIns().First(&successful, "id=?", deferred[0].ID).Error; err != nil || successful.NextCalculationAt != 0 || successful.CalculationFailures != 1 || successful.CalculationStage != constUtil.P13UFPlayerBattleComputed {
		t.Fatal("success lost retry history", err)
	}
}
