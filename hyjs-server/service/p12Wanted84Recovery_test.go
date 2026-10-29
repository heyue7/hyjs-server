package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The original instance is a finite fixture. The real consumers own attempts,
// frozen inputs, independently calculated HP, the receipt and the score.
func TestP12Wanted84RejectedRecordingAndUnknownReceiptRecovery(t *testing.T) {
	f := p12WantedPersistenceFixture(t)
	now := time.Now()
	if _, err := f.s.p12WantedList(f.ctx, f.id, now); err != nil {
		t.Fatal(err)
	}
	instance := p12WantedOwnershipInstance(t, f, "wanted84-recovery", now)
	request := &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)}
	startCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, request)
	before, _ := f.profile(t)
	startReplies, err := f.s.p12WantedStart(startCtx, f.id, request, 984000, now)
	if err != nil {
		t.Fatal(err)
	}
	start := startReplies[0].(*pb.SCRebelFightStart)
	paid, _ := f.profile(t)
	if paid.GetWantedAttTimes() != before.GetWantedAttTimes()-1 {
		t.Fatal("Start did not consume exactly one attempt")
	}
	var fight db.P12WantedFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("missing frozen original terminal")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if result == 0 {
		// The original client can omit a zero result; preserve that presence.
		end.Result = nil
	}
	bad := proto.Clone(end).(*pb.CSRebelFightEnd)
	bad.OptRecord.DefenderTotalLeftHp = proto.Float64(bad.OptRecord.GetDefenderTotalLeftHp() + 1)
	endCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, bad)
	_, err = f.s.p12WantedEnd(endCtx, f.id, bad, now)
	var replay *P12WantedRetryableEndError
	var mismatch *p6CompletedRecordedMismatchError
	if !errors.As(err, &replay) || !errors.As(err, &mismatch) ||
		!strings.HasPrefix(err.Error(), constUtil.P6ReplayConflict+": 原录像终局或完整技能记录不符") ||
		replay.Error() != mismatch.Error() || errors.Unwrap(replay) == nil || errors.Unwrap(replay).Error() != replay.Error() {
		t.Fatal("recording rejection lost its original error or pending classification", err)
	}
	var rejected db.P12WantedFight
	var sameInstance db.P12WantedInstance
	if err = db.GetIns().First(&rejected, "id=?", fight.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&sameInstance, "id=?", instance.ID).Error; err != nil {
		t.Fatal(err)
	}
	current, _ := f.profile(t)
	if rejected.Committed || rejected.AbandonedAt != 0 || rejected.RejectedAt <= 0 || rejected.RejectedReason != constUtil.P6ReplayConflict ||
		rejected.InputHash != fight.InputHash || rejected.StartHash != fight.StartHash || rejected.TerminalHash != fight.TerminalHash ||
		sameInstance.MonsterHPJSON != instance.MonsterHPJSON || !proto.Equal(current, paid) {
		t.Fatal("rejected End changed paid state, frozen proof, HP or wallet")
	}
	f.restart(t)
	recovery := &pb.CSRebelFightStart{RebelId: proto.String(constUtil.P12WantedRecoveryPrefix + fight.ID), Type: proto.Int32(1)}
	recoveryCtx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, recovery)
	wrongType := proto.Clone(recovery).(*pb.CSRebelFightStart)
	wrongType.Type = proto.Int32(2)
	if _, err = f.s.p12WantedStart(recoveryCtx, f.id, wrongType, 984001, now); err == nil {
		t.Fatal("named recovery accepted the wrong paid attack type")
	}
	var other db.Profile
	if err = db.GetIns().Select("id").Where("id<>?", f.id).First(&other).Error; err != nil {
		t.Fatal("isolated second owner fixture missing", err)
	}
	otherFights := p1Count(t, &db.P12WantedFight{}, "player_id=?", other.Id)
	if _, err = f.s.p12WantedStart(recoveryCtx, other.Id, recovery, 984001, now); err == nil ||
		p1Count(t, &db.P12WantedFight{}, "player_id=?", other.Id) != otherFights {
		t.Fatal("foreign player recovered or created a paid fight", err)
	}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("instance_id", "foreign-instance").Error; err != nil {
		t.Fatal(err)
	}
	_, wrongInstanceErr := f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984001, now)
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("instance_id", fight.InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	if wrongInstanceErr == nil || wrongInstanceErr.Error() != constUtil.P12WantedInvalid {
		t.Fatal("fight and frozen instance identity were not checked", wrongInstanceErr)
	}
	var costFact db.EventFact
	if err = db.GetIns().Where("player_id=? AND source_key=? AND kind=?", f.id, "wanted:start:"+fight.ID, "spend:wantedAttTimes").First(&costFact).Error; err != nil {
		t.Fatal("paid attempt fact missing", err)
	}
	originalCost := costFact.Amount
	if err = db.GetIns().Model(&costFact).Update("amount", originalCost+1).Error; err != nil {
		t.Fatal(err)
	}
	_, damagedCostErr := f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984001, now)
	if err = db.GetIns().Model(&costFact).Update("amount", originalCost).Error; err != nil {
		t.Fatal(err)
	}
	if damagedCostErr == nil || damagedCostErr.Error() != constUtil.P12WantedInvalid {
		t.Fatal("recovery accepted a mismatched paid attempt fact", damagedCostErr)
	}
	retry, err := f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984002, now)
	if err != nil || !proto.Equal(retry[0], start) {
		t.Fatal("cold named recovery did not resume the same paid fight", err)
	}
	var resumed db.P12WantedFight
	if err = db.GetIns().First(&resumed, "id=?", fight.ID).Error; err != nil || !resumed.RecoveryLeaseActive ||
		resumed.RejectedAt != rejected.RejectedAt || resumed.RejectedReason != rejected.RejectedReason {
		t.Fatal("recovery lease or original rejection evidence lost", err)
	}
	afterRetry, _ := f.profile(t)
	if !proto.Equal(afterRetry, paid) {
		t.Fatal("pending Start consumed another attempt")
	}
	// A corrupted frozen input may not be advertised as a retryable recording.
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("input_hash", "foreign").Error; err != nil {
		t.Fatal(err)
	}
	_, err = f.s.p12WantedEnd(endCtx, f.id, end, now)
	replay = nil
	if err == nil || errors.As(err, &replay) || err.Error() != constUtil.P12WantedInvalid {
		t.Fatal("damaged input incorrectly classified for replay", err)
	}
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("input_hash", fight.InputHash).Error; err != nil {
		t.Fatal(err)
	}
	_, err = f.s.p12WantedEnd(endCtx, f.id, bad, now)
	replay = nil
	if !errors.As(err, &replay) {
		t.Fatal("restored original data did not retain a verifiable recording rejection", err)
	}
	if _, err = f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984002, now); err != nil {
		t.Fatal("second recording rejection did not reacquire its paid lease", err)
	}
	endCtx = context.WithValue(f.ctx, constUtil.GameRequestContextKey, end)
	finished, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil {
		t.Fatal(err)
	}
	accepted := finished[0].(*pb.SCRebelFightEnd)
	after, _ := f.profile(t)
	if after.GetWantedAttTimes() != paid.GetWantedAttTimes() || after.GetScore()-paid.GetScore() != accepted.GetScoreAdd() {
		t.Fatal("End changed attempts or applied score more than once")
	}
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	if !fight.Committed {
		t.Fatal("accepted End was not committed")
	}
	if err = p12WantedAutomaticCommittedReceipt(&fight, &terminal); err != nil {
		t.Fatal("actual original automatic receipt did not validate", err)
	}
	if result == 0 {
		explicitZero := proto.Clone(end).(*pb.CSRebelFightEnd)
		explicitZero.Result = proto.Int32(0)
		raw, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(explicitZero)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		explicitReceipt := fight
		explicitReceipt.EndRequestHash = p6Digest(raw)
		if err = p12WantedAutomaticCommittedReceipt(&explicitReceipt, &terminal); err != nil {
			t.Fatal("explicit zero-result presence was not preserved", err)
		}
	}
	// The 90005 response and a different accepted End can cross in flight.
	// A later named recovery may not fall through to a new paid Start.
	countBefore := p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id)
	if _, err = f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984003, now); err == nil || err.Error() != constUtil.P12WantedRecoveryCommitted {
		t.Fatal("named recovery did not report the verified committed fight", err)
	}
	postRecovery, _ := f.profile(t)
	if p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != countBefore || !proto.Equal(postRecovery, after) {
		t.Fatal("committed named recovery created a fight or changed resources")
	}
	for _, field := range []string{"start_hash", "end_hash"} {
		original := map[string]string{"start_hash": fight.StartHash, "end_hash": fight.EndHash}[field]
		if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update(field, "foreign").Error; err != nil {
			t.Fatal(err)
		}
		_, checkErr := f.s.p12WantedStart(recoveryCtx, f.id, recovery, 984003, now)
		if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update(field, original).Error; err != nil {
			t.Fatal(err)
		}
		if checkErr == nil || checkErr.Error() != constUtil.P12WantedInvalid {
			t.Fatal("damaged committed receipt was reported as complete", field, checkErr)
		}
	}
	// A different End after commit is an unknown/conflicting receipt, never a
	// signal to replay a paid fight. No rejection metadata may be overwritten.
	conflict := proto.Clone(end).(*pb.CSRebelFightEnd)
	conflict.Result = proto.Int32(1 - result)
	_, err = f.s.p12WantedEnd(endCtx, f.id, conflict, now)
	replay = nil
	if err == nil || errors.As(err, &replay) || err.Error() != constUtil.P12WantedRecoveryCommitted {
		t.Fatal("committed recovered fight was not safely reported", err)
	}
	// The special completed status is restricted to a recovered lease.
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("recovery_lease_active", false).Error; err != nil {
		t.Fatal(err)
	}
	_, ordinaryConflict := f.s.p12WantedEnd(endCtx, f.id, conflict, now)
	if err = db.GetIns().Model(&db.P12WantedFight{}).Where("id=?", fight.ID).Update("recovery_lease_active", true).Error; err != nil {
		t.Fatal(err)
	}
	if ordinaryConflict == nil || ordinaryConflict.Error() != constUtil.P12WantedInvalid {
		t.Fatal("ordinary committed conflict changed", ordinaryConflict)
	}
	pending, metaErr := recordP12WantedRejectedEnd(f.ctx, f.id, fight.ID, now, errors.New(constUtil.P6ReplayConflict))
	if metaErr != nil || pending {
		t.Fatal("committed fight appeared pending during metadata recheck", pending, metaErr)
	}
	var afterConflict db.P12WantedFight
	if err = db.GetIns().First(&afterConflict, "id=?", fight.ID).Error; err != nil ||
		afterConflict.RejectedAt != fight.RejectedAt || afterConflict.RejectedReason != fight.RejectedReason {
		t.Fatal("committed rejection evidence changed", err)
	}
	f.restart(t)
	cold, err := f.s.p12WantedEnd(endCtx, f.id, end, now)
	if err != nil || !proto.Equal(cold[0], accepted) {
		t.Fatal("cold exact End did not return its existing receipt", err)
	}
	last, _ := f.profile(t)
	if !proto.Equal(last, after) {
		t.Fatal("cold receipt retry repeated resource changes")
	}
	var remaining db.P12WantedInstance
	if err = db.GetIns().First(&remaining, "id=?", instance.ID).Error; err != nil || remaining.State != constUtil.P12WantedActive || remaining.CurrentHP <= 0 {
		t.Fatal("fixture needs a still-live original boss for a new paid attack", err)
	}
	fresh, err := f.s.p12WantedStart(startCtx, f.id, request, 984004, now)
	if err != nil || fresh[0].(*pb.SCRebelFightStart).GetFightId() == fight.ID {
		t.Fatal("normal new attack after completed recovery failed", err)
	}
	newPaid, _ := f.profile(t)
	if newPaid.GetWantedAttTimes() != after.GetWantedAttTimes()-1 || p1Count(t, &db.P12WantedFight{}, "player_id=?", f.id) != countBefore+1 {
		t.Fatal("normal explicit new attack did not charge exactly once")
	}
}
