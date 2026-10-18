package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// Stored v2/v3 snapshots share this unchanged record schema; playback must use their content.
func TestP6PreviousEngineCompletedNativeReplaysStayReadable(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	out, err := s.ComputeFrozenBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"apk-10056262-p6-v2", "apk-10056262-p6-v3", "apk-10056262-p6-v41"} {
		for _, computed := range []bool{false, true} {
			fight := version
			if computed {
				fight += "-computed"
			}
			var recordID string
			if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
				var e error
				recordID, e = s.CaptureBattleReplay(ctx, tx, id, "compatibility", fight, *input)
				if e != nil {
					return e
				}
				if computed {
					_, e = s.ComputeBattleReplay(ctx, tx, id, "compatibility", fight, nil)
					return e
				}
				return s.FinishObservedBattleReplay(ctx, tx, id, "compatibility", fight, out.Operates, out.Winner == 0)
			}); err != nil {
				t.Fatal(err)
			}
			before, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
			if err != nil {
				t.Fatal(err)
			}
			var record db.P6BattleRecord
			if err := db.GetIns().First(&record, "id=?", recordID).Error; err != nil {
				t.Fatal(err)
			}
			var legacy model.FrozenBattleInput
			storedInput, err := decodeP6StoredInput(record.Input)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(storedInput, &legacy); err != nil {
				t.Fatal(err)
			}
			legacy.EngineVersion = version
			if version == "apk-10056262-p6-v41" {
				// v41 diagnostic records could carry the internal defending-first
				// flag with historical 0/0 wire. Query preserves that original wire.
				legacy.DefenderFirst = true
				legacy.Attacker.Inst.FirstOrder = proto.Int32(0)
				legacy.Defender.Inst.FirstOrder = proto.Int32(0)
			}
			record.Input, err = json.Marshal(&legacy)
			if err != nil {
				t.Fatal(err)
			}
			legacyOperate := proto.Clone(out.Operates).(*pb.BattleOperate)
			for _, op := range legacyOperate.SkillOpts {
				op.ActorUid = proto.String(fmt.Sprintf("%d_%s", op.GetCamp(), op.GetActorUid()))
			}
			record.Operates, err = proto.Marshal(legacyOperate)
			if err != nil {
				t.Fatal(err)
			}
			record.ReplayHash = p6Digest(append(append([]byte(nil), record.Operates...), byte(record.ClientResult)))
			if computed {
				var legacyOutcome model.CombatOutcome
				if err = json.Unmarshal(record.Outcome, &legacyOutcome); err != nil {
					t.Fatal(err)
				}
				legacyOutcome.Operates = legacyOperate
				record.Outcome, err = json.Marshal(legacyOutcome)
				if err != nil {
					t.Fatal(err)
				}
				record.OutcomeHash = p6Digest(record.Outcome)
			}
			record.InputHash = p6Digest(record.Input)
			record.EngineVersion = version
			if err := db.GetIns().Save(&record).Error; err != nil {
				t.Fatal(err)
			}
			before, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID), ServerId: proto.Int32(1)})
			if err != nil || !proto.Equal(before[0].(*pb.SCFightPlayback).OptRecord, legacyOperate) {
				t.Fatal("historical prefixed operations lost", err)
			}
			if version == "apk-10056262-p6-v41" && (before[0].(*pb.SCFightPlayback).Attacker.GetInst().GetFirstOrder() != 0 || before[0].(*pb.SCFightPlayback).Defender1.GetInst().GetFirstOrder() != 0) {
				t.Fatal("historical original initiative flags normalized")
			}
			s.table("SkillCast", "SkillCastId", "895")["ExportFactor"] = "999"
			after, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(fight), ServerId: proto.Int32(1)})
			if err != nil || !proto.Equal(before[0], after[0]) {
				t.Fatal("old completed replay changed", version, computed, err)
			}
			if _, err := s.ComputeFrozenBattle(ctx, &legacy, nil); err == nil {
				t.Fatal("legacy snapshot recomputed with current engine")
			}
		}
	}
}
