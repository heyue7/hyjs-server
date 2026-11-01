package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// A real timing rejection must settle the certified automatic terminal and
// keep the original immutable receipt across retries and a cold restart.
func TestP12WantedTimingMismatchSettlesServerTerminalOnce(t *testing.T) {
	testP12WantedRecordingFallback(t, 0, errP6ReplayTiming)
}

func TestP12WantedInvalidRecordingSettlesServerTerminalOnce(t *testing.T) {
	testP12WantedRecordingFallback(t, 99, errP6ReplayInvalid)
}

func testP12WantedRecordingFallback(t *testing.T, skillIndex int32, expected error) {
	t.Helper()
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "wanted-timing-fallback", now)
	before, _ := f.profile(t)
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	started, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 118201, now)
	if err != nil {
		t.Fatal(err)
	}
	var fight db.P12WantedFight
	if err := db.GetIns().First(&fight, "id=?", started[0].(*pb.SCRebelFightStart).GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var input model.FrozenBattleInput
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("missing frozen battle or certified terminal")
	}
	record := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	record.SkillOpts = []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(0), Camp: proto.Int32(0), ActorUid: proto.String("foreign-actor"), SkillIndex: proto.Int32(skillIndex)}}
	if _, err = f.s.ValidateP6GenericMechanisms(f.ctx, &input); err != nil {
		t.Fatalf("frozen mechanism validation: %v", err)
	}
	if _, err = f.s.computeP12WantedRecorded(f.ctx, &input, record); !errors.Is(err, expected) {
		t.Fatalf("fixture did not reach the expected recorded-input failure: %v", err)
	}
	winner := int32(0)
	if terminal.Outcome.Winner == 0 {
		winner = 1
	}
	request := &pb.CSRebelFightEnd{FightId: proto.String(fight.ID), Result: proto.Int32(1 - winner), OptRecord: record}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	if db.GetIns().Dialector.Name() == "mysql" {
		ctx, cancel := context.WithTimeout(endCtx, 15*time.Second)
		defer cancel()
		held, release := make(chan struct{}), make(chan struct{})
		holderDone := make(chan error, 1)
		go func() {
			holderDone <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if _, err := p1LockPeriods(tx); err != nil {
					return err
				}
				close(held)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
		}()
		select {
		case <-held:
		case err := <-holderDone:
			t.Fatalf("could not hold activity lock: %v", err)
		case <-ctx.Done():
			t.Fatal("activity holder timed out")
		}
		var once sync.Once
		finish := func() { once.Do(func() { close(release); <-holderDone }) }
		defer finish()
		computed := make(chan *p12WantedPreparedRecording, 1)
		go func() { computed <- f.s.p12WantedPrepareRecording(ctx, f.id, request) }()
		select {
		case prepared := <-computed:
			if prepared == nil || !errors.Is(prepared.err, expected) || prepared.inputHash != fight.InputHash || prepared.terminalHash != fight.TerminalHash {
				t.Fatalf("prepared recording lost its frozen identity: %+v", prepared)
			}
		case <-time.After(8 * time.Second):
			finish()
			<-computed
			t.Fatal("recorded computation waited on another player's activity lock")
		}
		finish()
	}
	first, err := f.s.p12WantedEnd(endCtx, f.id, request, now)
	if err != nil {
		t.Fatal(err)
	}
	result := first[0].(*pb.SCRebelFightEnd)
	if result.GetResult() != winner {
		t.Fatal("settlement did not use the frozen server winner")
	}
	var committed db.P12WantedFight
	if err := db.GetIns().First(&committed, "id=?", fight.ID).Error; err != nil || !committed.Committed || committed.RecordedProof != constUtil.P12WantedAutomaticFallback || committed.TerminalHash != fight.TerminalHash {
		t.Fatalf("timing fallback receipt: %v", err)
	}
	after, game := f.profile(t)
	if after.GetWantedAttTimes() != before.GetWantedAttTimes()-1 {
		t.Fatal("attack count changed more than once")
	}
	var afterInstance db.P12WantedInstance
	if err := db.GetIns().First(&afterInstance, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	replayed, err := f.s.p12WantedEnd(endCtx, f.id, request, now)
	if err != nil || !proto.Equal(result, replayed[0]) {
		t.Fatalf("cold timing fallback replay: %v", err)
	}
	final, finalGame := f.profile(t)
	var finalInstance db.P12WantedInstance
	if err := db.GetIns().First(&finalInstance, "id=?", instance.ID).Error; err != nil || !proto.Equal(after, final) || !p6CopySeriesJSONEqual(t, game, finalGame) || finalInstance.MonsterHPJSON != afterInstance.MonsterHPJSON {
		t.Fatalf("replay changed rewards or monster health: %v", err)
	}
	if p12WantedCanFallbackRecording(errors.New("录像操作不符合原技能时序: untrusted")) || p12WantedCanFallbackRecording(errors.New(constUtil.CombatInvalid)) || p12WantedCanFallbackRecording(context.DeadlineExceeded) {
		t.Fatal("untyped or cancelled computation bypassed the fallback boundary")
	}
}
