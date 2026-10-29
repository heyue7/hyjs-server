package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP12Wanted84CommittedManualReceiptCannotMasqueradeAsAutomatic(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	start, _, recovery := p12Wanted84RejectedFight(t, f, "wanted84-receipt", 984301, now)
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, recovery)
	if _, err := f.s.p12WantedStart(startCtx, f.id, recovery, 984302, now); err != nil {
		t.Fatal("reacquire paid fight", err)
	}
	var fight db.P12WantedFight
	if err := db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var input model.FrozenBattleInput
	var automatic model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &automatic) != nil {
		t.Fatal("original frozen input and automatic terminal missing")
	}
	var manual *model.CombatFinalHPOutcome
	for _, clock := range []float64{1.2, 2.4} {
		derived, err := cloneP6CalculationInput(&input)
		if err != nil {
			t.Fatal(err)
		}
		derived.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
		derived.DiagnosticManualAutoStartRoundTime = proto.Float64(clock)
		manual, err = f.s.computeFrozenBattleFinalHP(f.ctx, derived, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(manual.OriginalOperates, automatic.OriginalOperates) {
			break
		}
	}
	if manual == nil || proto.Equal(manual.OriginalOperates, automatic.OriginalOperates) {
		t.Fatal("test needs a distinct real manual-auto recording")
	}
	accepted, err := f.s.computeP12WantedRecorded(f.ctx, &input, manual.OriginalOperates)
	if err != nil {
		t.Fatal("independent recorded computation", err)
	}
	result := int32(0)
	if accepted.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: manual.OriginalOperates}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	if _, err = f.s.p12WantedEnd(endCtx, f.id, end, now); err != nil {
		t.Fatal("manual End", err)
	}
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || !fight.Committed || fight.RecordedProof == "" {
		t.Fatal("committed manual receipt missing", err)
	}
	before, _ := f.profile(t)
	stored := map[string]any{"recorded_end_request": fight.RecordedEndRequest, "recorded_terminal_json": fight.RecordedTerminalJSON,
		"recorded_terminal_hash": fight.RecordedTerminalHash, "recorded_record_hash": fight.RecordedRecordHash, "recorded_proof": fight.RecordedProof}
	empty := map[string]any{"recorded_end_request": []byte{}, "recorded_terminal_json": "", "recorded_terminal_hash": "", "recorded_record_hash": "", "recorded_proof": ""}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Updates(empty).Error; err != nil {
		t.Fatal(err)
	}
	_, erasedErr := f.s.p12WantedStart(startCtx, f.id, recovery, 984303, now)
	conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
	conflict.Result = proto.Int32(1 - result)
	_, conflictErr := f.s.p12WantedEnd(endCtx, f.id, conflict, now)
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Updates(stored).Error; err != nil {
		t.Fatal("restore isolated receipt fixture", err)
	}
	if erasedErr == nil || erasedErr.Error() != constUtil.P12WantedInvalid || conflictErr == nil || conflictErr.Error() != constUtil.P12WantedInvalid {
		t.Fatal("missing manual proof was mistaken for a committed automatic receipt", erasedErr, conflictErr)
	}
	if _, err = f.s.p12WantedStart(startCtx, f.id, recovery, 984304, now); err == nil || err.Error() != constUtil.P12WantedRecoveryCommitted {
		t.Fatal("intact manual proof did not report existing settlement", err)
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("receipt checks changed wallet or attempts")
	}
}
