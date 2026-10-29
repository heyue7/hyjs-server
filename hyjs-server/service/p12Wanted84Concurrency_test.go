package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p12Wanted84RejectedFight(t *testing.T, f *p1Fixture, id string, stamp int64, now time.Time) (*pb.SCRebelFightStart, *pb.CSRebelFightEnd, *pb.CSRebelFightStart) {
	t.Helper()
	instance := p12WantedOwnershipInstance(t, f, id, now)
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	started, err := f.s.p12WantedStart(startCtx, f.id, startRequest, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	start := started[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("original frozen terminal missing")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	bad := proto.Clone(end).(*pb.CSRebelFightEnd)
	bad.OptRecord.DefenderTotalLeftHp = proto.Float64(bad.OptRecord.GetDefenderTotalLeftHp() + 1)
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, bad)
	_, err = f.s.p12WantedEnd(ctx, f.id, bad, now)
	var rejected *P12WantedRetryableEndError
	if !errors.As(err, &rejected) {
		t.Fatal("strict rejected recording fixture", err)
	}
	recovery := &pb.CSRebelFightStart{RebelId: proto.String(constUtil.P12WantedRecoveryPrefix + fight.ID), Type: proto.Int32(1)}
	return start, end, recovery
}

func TestP12Wanted84MySQLRecoveryCompetesWithEndWithoutNewCost(t *testing.T) {
	if os.Getenv("HYJS_P7_MYSQL_DATABASE") == "" {
		t.Skip("requires project-allocated isolated MySQL")
	}
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	start, end, recovery := p12Wanted84RejectedFight(t, f, "wanted84-race-end", 984101, now)
	paid, _ := f.profile(t)
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, recovery)
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	ready := make(chan struct{})
	var wg sync.WaitGroup
	var endReplies, startReplies []proto.Message
	var endErr, startErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-ready; endReplies, endErr = f.s.p12WantedEnd(endCtx, f.id, end, now) }()
	go func() {
		defer wg.Done()
		<-ready
		startReplies, startErr = f.s.p12WantedStart(startCtx, f.id, recovery, 984102, now)
	}()
	close(ready)
	wg.Wait()
	if endErr != nil {
		t.Fatal("valid End did not commit", endErr)
	}
	if startErr == nil {
		if !proto.Equal(startReplies[0], start) {
			t.Fatal("concurrent recovery returned a different Start")
		}
	} else if startErr.Error() != constUtil.P12WantedRecoveryCommitted {
		t.Fatal("recovery after committed End returned an unexpected error", startErr)
	}
	accepted := endReplies[0].(*pb.SCRebelFightEnd)
	after, _ := f.profile(t)
	if p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 1 || after.GetWantedAttTimes() != paid.GetWantedAttTimes() ||
		after.GetScore()-paid.GetScore() != accepted.GetScoreAdd() {
		t.Fatal("End/recovery race created another fight, cost or score award")
	}
	cold, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(cold[0], accepted) {
		t.Fatal("same End did not return its committed receipt", err)
	}
}

func TestP12Wanted84MySQLRecoveryCompetesWithOriginalAwakeEntry(t *testing.T) {
	if os.Getenv("HYJS_P7_MYSQL_DATABASE") == "" {
		t.Skip("requires project-allocated isolated MySQL")
	}
	f := p12WantedPersistenceFixture(t)
	if err := f.s.LoadP7AwakeTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP7Awake(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(player *pb.Player, _ *model.GameState) error { player.Level = proto.Int32(70); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", constUtil.P7AwakeExploreCopy).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// Original forest exploration is open Friday; this finite date is a fixture.
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	start, _, recovery := p12Wanted84RejectedFight(t, f, "wanted84-race-awake", 984201, now)
	paid, _ := f.profile(t)
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, recovery)
	ready := make(chan struct{})
	var wg sync.WaitGroup
	var recoveryReplies []proto.Message
	var recoveryErr, awakeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-ready
		recoveryReplies, recoveryErr = f.s.p12WantedStart(startCtx, f.id, recovery, 984202, now)
	}()
	go func() {
		defer wg.Done()
		<-ready
		_, awakeErr = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now)
	}()
	close(ready)
	wg.Wait()
	if recoveryErr == nil {
		if awakeErr == nil || !strings.Contains(awakeErr.Error(), constUtil.P12WantedBusy) || !proto.Equal(recoveryReplies[0], start) {
			t.Fatal("reacquired Wanted did not exclude the other original mode", awakeErr)
		}
	} else {
		if awakeErr != nil || !strings.Contains(recoveryErr.Error(), "请先退出当前觉醒探索") {
			t.Fatal("active original mode did not exclude Wanted recovery", recoveryErr, awakeErr)
		}
		if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now); err != nil {
			t.Fatal("settle the original exploration", err)
		}
		recovered, err := f.s.p12WantedStart(startCtx, f.id, recovery, 984203, now)
		if err != nil || !proto.Equal(recovered[0], start) {
			t.Fatal("paid recovery failed after the other mode settled", err)
		}
	}
	var fight db.P12WantedFight
	if err := db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || !fight.RecoveryLeaseActive || fight.RejectedAt <= 0 || fight.RejectedReason != constUtil.P6ReplayConflict {
		t.Fatal("recovered ownership or rejection evidence lost", err)
	}
	after, _ := f.profile(t)
	if after.GetWantedAttTimes() != paid.GetWantedAttTimes() || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != 1 {
		t.Fatal("competing mode or recovery duplicated Wanted cost")
	}
}
