package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12WantedMismatchedReplaySettlesServerTerminalOnce(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "wanted-server-fallback", now)
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	before, _ := f.profile(t)
	starts, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 118001, now)
	if err != nil {
		t.Fatal(err)
	}
	start := starts[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("missing independently certified terminal")
	}
	winner := int32(0)
	if terminal.Outcome.Winner == 0 {
		winner = 1
	}
	record := proto.Clone(terminal.OriginalOperates).(*pb.BattleOperate)
	record.DefenderTotalLeftHp = proto.Float64(record.GetDefenderTotalLeftHp() + 1)
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(1 - winner), OptRecord: record}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	type response struct {
		messages []proto.Message
		err      error
	}
	var ready sync.WaitGroup
	ready.Add(2)
	begin := make(chan struct{})
	results := make(chan response, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ready.Done()
			<-begin
			messages, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
			results <- response{messages: messages, err: err}
		}()
	}
	ready.Wait()
	close(begin)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || len(first.messages) != 1 || len(second.messages) != 1 || !proto.Equal(first.messages[0], second.messages[0]) {
		t.Fatalf("concurrent recording fallback: first=%v second=%v", first.err, second.err)
	}
	result := first.messages[0].(*pb.SCRebelFightEnd)
	if result.GetResult() != winner {
		t.Fatal("fallback used client winner instead of the certified server terminal")
	}
	var committed db.P12WantedFight
	var afterInstance db.P12WantedInstance
	if err = db.GetIns().First(&committed, "id=?", fight.ID).Error; err != nil || !committed.Committed || committed.RecordedProof != constUtil.P12WantedAutomaticFallback || committed.TerminalHash != fight.TerminalHash {
		t.Fatalf("fallback receipt: %v", err)
	}
	if err = db.GetIns().First(&afterInstance, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	after, game := f.profile(t)
	if after.GetWantedAttTimes() != before.GetWantedAttTimes()-1 {
		t.Fatal("attack count was not deducted exactly once")
	}
	f.restart(t)
	replay, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(replay[0], result) {
		t.Fatalf("cold fallback receipt: %v", err)
	}
	final, finalGame := f.profile(t)
	var finalInstance db.P12WantedInstance
	if err = db.GetIns().First(&finalInstance, "id=?", instance.ID).Error; err != nil || !proto.Equal(after, final) || !p6CopySeriesJSONEqual(t, game, finalGame) || finalInstance.MonsterHPJSON != afterInstance.MonsterHPJSON {
		t.Fatalf("replay changed rewards or shared HP: %v", err)
	}
	conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
	conflict.OptRecord.DefenderTotalLeftHp = proto.Float64(record.GetDefenderTotalLeftHp() + 1)
	if _, err = f.s.p12WantedEnd(endCtx, f.id, conflict, now); err == nil {
		t.Fatal("different end replaced a committed receipt")
	}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("recorded_record_hash", "damaged").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p12WantedEnd(endCtx, f.id, end, now); err == nil {
		t.Fatal("damaged fallback receipt was accepted")
	}
	for _, transient := range []error{context.Canceled, context.DeadlineExceeded, errors.New("database connection lost")} {
		if p12WantedCanFallbackRecording(transient) {
			t.Fatalf("transient error triggered fallback: %v", transient)
		}
	}
}
