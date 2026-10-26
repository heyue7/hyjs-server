package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestP6UFPartyFreshAdmissionExactLegacySHAAndColdWorker(t *testing.T) {
	f, id, enemy, clock, match := p13UFPartyAdmissionActualSetup(t)
	replies, handled, err := f.s.p13UFTryPlayerBattle(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0, time.UnixMilli(clock.Load()))
	if err != nil || !handled || len(replies) != 2 {
		t.Fatal("actual party capture", handled, err)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	var typed pb.SCUFFightRecordReplay
	if err = proto.Unmarshal(attempt.StartPB, &typed); err != nil {
		t.Fatal(err)
	}
	original := &GameService{Tables: input.Tables}
	legacy, err := original.FreezeP6UFPlayerInput(&typed)
	if err != nil {
		t.Fatal(err)
	}
	legacy.AppliedProperties = input.AppliedProperties
	legacy, err = original.FreezeBattleInput(*legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyRaw, err := json.Marshal(legacy)
	if err != nil || p6Digest(legacyRaw) != record.InputHash {
		t.Fatal("fresh capture changed legacy input bytes/SHA", err)
	}
	fresh, err := original.newP6UFPartyPlayerAdmission(f.ctx, &typed, input.AppliedProperties)
	if err != nil {
		t.Fatal(err)
	}
	rollbackComparison := errors.New("rollback standalone captured comparison")
	if err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		got, hash, again, err := original.captureP6UFPartyPlayerAdmission(f.ctx, tx, id, attempt.FightID, fresh)
		if err != nil {
			return err
		}
		a, _ := json.Marshal(again)
		if got != record.ID || hash != record.InputHash || string(a) != string(legacyRaw) {
			t.Fatal("natural idempotent capture changed frozen source")
		}
		// Exercise the unchanged public capture/storage path as well. Its
		// standalone diagnostic identity is rolled back and never enters war.
		oldID, err := original.CaptureBattleReplay(f.ctx, tx, id, "fresh_exact_legacy", attempt.FightID, *legacy)
		if err != nil {
			return err
		}
		var oldRecord db.P6BattleRecord
		if err = tx.First(&oldRecord, "id=?", oldID).Error; err != nil {
			return err
		}
		oldInput, err := decodeP6Record(&oldRecord)
		if err != nil {
			return err
		}
		oldRaw, _ := json.Marshal(oldInput)
		if oldRecord.InputHash != record.InputHash || string(oldRaw) != string(legacyRaw) {
			t.Fatal("public/fresh capture input and record SHA differ")
		}
		return rollbackComparison
	}); !errors.Is(err, rollbackComparison) {
		t.Fatal(err)
	}
	clock.Store(attempt.DueAt)
	if err = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil || attempt.Status != constUtil.P13UFPlayerBattleSettled {
		t.Fatal("real fresh input worker terminal", err)
	}
	hash := attempt.SourceHash
	if err = f.s.AdvanceP13UnionFightPlayerBattles(f.ctx, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&attempt, "id=?", attempt.ID).Error; err != nil || attempt.SourceHash != hash {
		t.Fatal("cold repeat changed settled attempt", err)
	}
	if _, _, _, _, err = f.s.p13UFPlayerBattleVerify(db.GetIns().WithContext(f.ctx), &attempt); err != nil {
		t.Fatal("full persisted verifier was bypassed", err)
	}
}

func TestP6UFPartyFreshAdmissionRejectsMutatedOrUnknownInput(t *testing.T) {
	f, id, enemy, clock, match := p13UFPartyAdmissionActualSetup(t)
	if _, _, err := f.s.p13UFTryPlayerBattle(f.ctx, id, &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(enemy)}, 0, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var attempt db.P13UnionFightPlayerBattleAttempt
	if err := db.GetIns().First(&attempt, "match_id=? AND player_id=?", match.ID, id).Error; err != nil {
		t.Fatal(err)
	}
	var record db.P6BattleRecord
	if err := db.GetIns().First(&record, "id=?", attempt.P6RecordID).Error; err != nil {
		t.Fatal(err)
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		t.Fatal(err)
	}
	var typed pb.SCUFFightRecordReplay
	if err = proto.Unmarshal(attempt.StartPB, &typed); err != nil {
		t.Fatal(err)
	}
	original := &GameService{Tables: input.Tables}
	var before int64
	if err = db.GetIns().Model(&db.P6BattleRecord{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"unknownTyped", "overHP", "overAP", "unknownSkill"} {
		bad := proto.Clone(&typed).(*pb.SCUFFightRecordReplay)
		switch kind {
		case "unknownTyped":
			bad.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x01})
		case "overHP":
			bad.PlayerList[0].Formation[0].Inst.CurHp = proto.Float64(bad.PlayerList[0].Formation[0].Prop.GetHp() + 1)
		case "overAP":
			bad.PlayerList[0].Inst.Anger = proto.Float64(301)
		case "unknownSkill":
			bad.PlayerList[0].Formation[0].SkillOrder = proto.String("unknown-admission-skill")
		}
		if _, err := original.newP6UFPartyPlayerAdmission(f.ctx, bad, input.AppliedProperties); err == nil {
			t.Fatal("fresh factory accepted invalid source", kind)
		}
	}
	for _, kind := range []string{"table", "hp", "ap", "typedIdentity"} {
		fresh, err := original.newP6UFPartyPlayerAdmission(f.ctx, &typed, input.AppliedProperties)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "table":
			fresh.input.Tables["Hero"][0]["id"] = "tampered-original-table"
		case "hp":
			fresh.input.Attacker.Formation[0].Inst.CurHp = proto.Float64(0)
		case "ap":
			fresh.input.Attacker.Inst.Anger = proto.Float64(0.125)
		case "typedIdentity":
			fresh.fightID = "different-fight"
		}
		if _, _, _, err := original.captureP6UFPartyPlayerAdmission(f.ctx, db.GetIns(), id, attempt.FightID, fresh); err == nil {
			t.Fatal("mutated sealed source captured", kind)
		}
	}
	var after int64
	if err = db.GetIns().Model(&db.P6BattleRecord{}).Count(&after).Error; err != nil || after != before {
		t.Fatal("rejected source wrote a replay", before, after, err)
	}
}
