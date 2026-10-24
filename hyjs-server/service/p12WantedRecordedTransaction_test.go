package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Domain instance creation is an explicit isolated prerequisite. Start and End
// use their real cost, frozen-input, HP, reward and committed-receipt consumers.
func TestP12Wanted65ManualAutoEndColdConflict(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "wanted65-manual-auto-end", now)
	before, _ := f.profile(t)
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	replies, err := f.s.p12WantedStart(startCtx, f.id, startRequest, 9651200, now)
	if err != nil {
		t.Fatal(err)
	}
	start := replies[0].(*pb.SCRebelFightStart)
	spent, _ := f.profile(t)
	if spent.GetWantedAttTimes() != before.GetWantedAttTimes()-1 {
		t.Fatal("Start attempt not charged once")
	}
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	originalJSON, originalHash := fight.TerminalJSON, fight.TerminalHash
	var input model.FrozenBattleInput
	var automatic model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.InputJSON), &input) != nil || json.Unmarshal([]byte(fight.TerminalJSON), &automatic) != nil {
		t.Fatal("frozen Start decode")
	}
	var manual *model.CombatFinalHPOutcome
	var clock float64
	for _, candidate := range []float64{1.2, 2.4} {
		derived, e := cloneP6CalculationInput(&input)
		if e != nil {
			t.Fatal(e)
		}
		derived.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
		derived.DiagnosticManualAutoStartRoundTime = proto.Float64(candidate)
		manual, e = f.s.computeFrozenBattleFinalHP(f.ctx, derived, nil, true)
		if e != nil {
			t.Fatal(e)
		}
		clock = candidate
		if !proto.Equal(manual.OriginalOperates, automatic.OriginalOperates) {
			break
		}
	}
	if manual == nil || proto.Equal(manual.OriginalOperates, automatic.OriginalOperates) {
		t.Fatal("diagnostic live manual source unexpectedly equals automatic")
	}
	accepted, err := f.s.computeP12WantedRecorded(f.ctx, &input, manual.OriginalOperates)
	if err != nil {
		t.Fatal("exact live source", clock, err)
	}
	result := int32(0)
	if accepted.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: proto.Clone(manual.OriginalOperates).(*pb.BattleOperate)}
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	invalid := proto.Clone(end).(*pb.CSRebelFightEnd)
	invalid.OptRecord.DefenderTotalLeftHp = proto.Float64(invalid.OptRecord.GetDefenderTotalLeftHp() + 1)
	if _, err = f.s.p12WantedEnd(endCtx, f.id, invalid, now); err == nil {
		t.Fatal("forged terminal accepted")
	}
	unchanged, _ := f.profile(t)
	if !proto.Equal(spent, unchanged) {
		t.Fatal("failed End changed profile")
	}
	var pending db.P12WantedFight
	if err = db.GetIns().First(&pending, "id=?", fight.ID).Error; err != nil || pending.Committed || pending.RecordedProof != "" {
		t.Fatal("failed End wrote receipt", err)
	}
	// Both concurrent requests carry the same actual source; only one commit may apply damage/score.
	var wg sync.WaitGroup
	outcomes := make([]*pb.SCRebelFightEnd, 2)
	errors := make([]error, 2)
	for i := range outcomes {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			messages, e := f.s.p12WantedEnd(endCtx, f.id, proto.Clone(end).(*pb.CSRebelFightEnd), now)
			errors[index] = e
			if e == nil {
				outcomes[index] = messages[0].(*pb.SCRebelFightEnd)
			}
		}(i)
	}
	wg.Wait()
	for _, e := range errors {
		if e != nil {
			t.Fatal("concurrent same End", e)
		}
	}
	if !proto.Equal(outcomes[0], outcomes[1]) {
		t.Fatal("same End receipts differ")
	}
	after, _ := f.profile(t)
	if after.GetWantedAttTimes() != spent.GetWantedAttTimes() || after.GetScore()-spent.GetScore() != outcomes[0].GetScoreAdd() {
		t.Fatal("End changed attempt or duplicated score")
	}
	var stored db.P12WantedFight
	var hpAfter db.P12WantedInstance
	if err = db.GetIns().First(&stored, "id=?", fight.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&hpAfter, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	var hpBefore, hpActual map[string]float64
	if json.Unmarshal([]byte(instance.MonsterHPJSON), &hpBefore) != nil || json.Unmarshal([]byte(hpAfter.MonsterHPJSON), &hpActual) != nil {
		t.Fatal("HP decode")
	}
	expectedHP, damage, err := p12WantedApplyTerminalHP(&input, accepted, hpBefore)
	if err != nil || damage != outcomes[0].GetDamage() || !p6CopySeriesJSONEqual(t, expectedHP, hpActual) {
		t.Fatal("HP not applied once", err)
	}
	if !stored.Committed || stored.TerminalJSON != originalJSON || stored.TerminalHash != originalHash || len(stored.RecordedEndRequest) == 0 || stored.RecordedTerminalJSON == "" || stored.RecordedTerminalHash == "" || stored.RecordedRecordHash == "" || stored.RecordedProof != constUtil.P12WantedRecordedProof {
		t.Fatal("independent recorded receipt or original auto evidence missing")
	}
	f.restart(t)
	replayed, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(replayed[0], outcomes[0]) {
		t.Fatal("cold same End", err)
	}
	cold, _ := f.profile(t)
	if !proto.Equal(after, cold) {
		t.Fatal("cold replay changed wallet/attempt")
	}
	conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
	conflict.Result = proto.Int32(1 - result)
	if _, err = f.s.p12WantedEnd(endCtx, f.id, conflict, now); err == nil {
		t.Fatal("different request hash accepted")
	}
	for _, field := range []string{"recorded_terminal_hash", "recorded_record_hash", "recorded_proof"} {
		original := map[string]string{"recorded_terminal_hash": stored.RecordedTerminalHash, "recorded_record_hash": stored.RecordedRecordHash, "recorded_proof": stored.RecordedProof}[field]
		if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update(field, "foreign").Error; err != nil {
			t.Fatal(err)
		}
		_, checkErr := f.s.p12WantedEnd(endCtx, f.id, end, now)
		if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update(field, original).Error; err != nil {
			t.Fatal(err)
		}
		if checkErr == nil {
			t.Fatal("tampered cold receipt accepted", field)
		}
	}
	emptyRecorded := map[string]any{"recorded_end_request": []byte{}, "recorded_terminal_json": "", "recorded_terminal_hash": "", "recorded_record_hash": "", "recorded_proof": ""}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Updates(emptyRecorded).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	_, missingErr := f.s.p12WantedEnd(endCtx, f.id, end, now)
	restoredRecorded := map[string]any{"recorded_end_request": stored.RecordedEndRequest, "recorded_terminal_json": stored.RecordedTerminalJSON, "recorded_terminal_hash": stored.RecordedTerminalHash, "recorded_record_hash": stored.RecordedRecordHash, "recorded_proof": stored.RecordedProof}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Updates(restoredRecorded).Error; err != nil {
		t.Fatal(err)
	}
	if missingErr == nil {
		t.Fatal("all five missing recorded columns masqueraded as legacy automatic receipt")
	}
	last, _ := f.profile(t)
	var lastHP db.P12WantedInstance
	if err = db.GetIns().First(&lastHP, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(after, last) || lastHP.MonsterHPJSON != hpAfter.MonsterHPJSON {
		t.Fatal("retry/conflict/tamper changed score or HP")
	}
	t.Logf("real Start/live-auto End/cold/conflict: clock=%g operations=%d round=%d", clock, len(end.OptRecord.SkillOpts), accepted.Outcome.FinalRound)
}

func TestP12Wanted65AutomaticEmptyRecordedColumnsColdReceipt(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "wanted65-legacy-auto-cold", now)
	startRequest := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, startRequest)
	replies, err := f.s.p12WantedStart(ctx, f.id, startRequest, 9651201, now)
	if err != nil {
		t.Fatal(err)
	}
	start := replies[0].(*pb.SCRebelFightStart)
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var automatic model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &automatic) != nil || automatic.Outcome == nil {
		t.Fatal("automatic terminal")
	}
	result := int32(0)
	if automatic.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: automatic.OriginalOperates}
	ctx = context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	finished, err := f.s.p12WantedEnd(ctx, f.id, end, now)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if err = db.GetIns().First(&fight, "id=?", fight.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(fight.RecordedEndRequest) != 0 || fight.RecordedTerminalJSON != "" || fight.RecordedTerminalHash != "" || fight.RecordedRecordHash != "" || fight.RecordedProof != "" {
		t.Fatal("automatic path populated manual evidence")
	}
	f.restart(t)
	replayed, err := f.s.p12WantedEnd(ctx, f.id, end, now)
	if err != nil || !proto.Equal(replayed[0], finished[0]) {
		t.Fatal("legacy automatic five-empty cold retry rejected", err)
	}
	cold, _ := f.profile(t)
	if !proto.Equal(after, cold) {
		t.Fatal("automatic cold replay changed wallet/attempt")
	}
}
