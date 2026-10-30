package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestCopyOptionalReplayDoesNotBlockSettlement(t *testing.T) {
	for _, variant := range []string{"valid", "missing", "invalid", "stored-input", "write-failure"} {
		t.Run(variant, func(t *testing.T) {
			s, ctx, id, read, cold := copyEncounterOrderSetup(t)
			if err := s.InitializeP6(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := s.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}); err != nil {
				t.Fatal(err)
			}
			messages, err := s.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")})
			if err != nil {
				t.Fatal(err)
			}
			start := messages[0].(*pb.SCCopyFightStart)
			if len(start.GetBattleMonsterList()) != 1 {
				t.Fatal("fixture must capture a single-group replay")
			}
			var record db.P6BattleRecord
			if err = db.GetIns().Where("player_id=? AND source=? AND source_fight=?", id, "copy", start.GetFightId()).First(&record).Error; err != nil {
				t.Fatal(err)
			}
			end := &pb.CSCopyFightEnd{FightId: start.FightId, BattlePlayer: proto.Clone(start.BattlePlayer).(*pb.BattlePlayer), Result: proto.Int32(1), OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1), AttackerTotalLeftHp: proto.Float64(1)}}
			switch variant {
			case "missing":
				end.OptRecord = nil
			case "invalid":
				end.OptRecord.FinalRoundNum = proto.Int32(1000)
			case "stored-input":
				if err = db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", record.ID).Update("input", []byte("bad")).Error; err != nil {
					t.Fatal(err)
				}
			case "write-failure":
				if err = db.GetIns().Exec("CREATE TRIGGER copy_replay_write_failure BEFORE UPDATE ON p6_battle_records BEGIN SELECT RAISE(ABORT, 'isolated replay failure'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			wrong := proto.Clone(end).(*pb.CSCopyFightEnd)
			wrong.BattlePlayer.PlayerId = proto.Int64(id + 1)
			if _, err = s.copyFightEnd(ctx, id, wrong); err == nil {
				t.Fatal("foreign battle player accepted")
			}
			messages, err = s.copyFightEnd(ctx, id, end)
			if err != nil || messages[0].(*pb.SCCopyFightEnd).GetFightResult() != 1 {
				t.Fatal("business settlement failed", err)
			}
			settled := messages[0].(*pb.SCCopyFightEnd)
			player, game := read()
			if err = db.GetIns().First(&record, "id=?", record.ID).Error; err != nil {
				t.Fatal(err)
			}
			if variant == "valid" && record.Status != constUtil.P6BattleObserved {
				t.Fatal("normal replay not preserved")
			}
			if variant != "valid" && record.Status != constUtil.P6BattleCaptured {
				t.Fatal("failed replay produced an observed record")
			}
			cold()
			retry, err := s.copyFightEnd(ctx, id, end)
			if err != nil || !proto.Equal(settled, retry[0]) {
				t.Fatal("cold settlement receipt changed", err)
			}
			again, gameAgain := read()
			if !proto.Equal(player, again) || !p6CopySeriesJSONEqual(t, game, gameAgain) {
				t.Fatal("retry duplicated progress or rewards")
			}
		})
	}
}
