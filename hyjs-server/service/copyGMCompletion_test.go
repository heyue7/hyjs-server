package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 有限前置进度仅隔离整关结算合同，不作为原引擎胜负证明。
func TestCopyGMCompletionCasualtyStarsReceiptBestAndCold(t *testing.T) {
	s, ctx, id, read, cold := copyEncounterOrderSetup(t)
	if err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		s.refreshCopyProgress(g)
		for _, info := range g.Copy.PassedCopyInfo {
			if info.GetMissionId() == "7" {
				info.Star = proto.Int32(5)
			}
		}
		if q := s.table("CopyMission", "MissionId", "8")["OpenId"]; q != "" {
			g.Quests = append(g.Quests, q)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	chapter := s.table("CopyMission", "MissionId", "8")["ChapterId"]
	for attempt, casualty := range []bool{true, false, true} {
		if _, err := s.copyEnter(ctx, id, &pb.CSCopyEnter{ChapterId: proto.String(chapter), MissionId: proto.String("8")}); err != nil {
			t.Fatal(err)
		}
		var final *pb.CSCopyFightEnd
		for i, task := range s.tasks("8") {
			out, err := s.copyFightStart(ctx, id, &pb.CSCopyFightStart{ChapterId: proto.String(chapter), MissionId: proto.String("8"), TaskId: proto.String(task["TaskId"])})
			if err != nil {
				t.Fatal(err)
			}
			start := out[0].(*pb.SCCopyFightStart)
			// 原请求校验仍核对冻结阵容；为本领域结算用例增加一个有限助阵快照。
			if i == len(s.tasks("8"))-1 {
				if err = updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
					helper := proto.Clone(g.Run.Fight.Start.BattlePlayer.Formation[0]).(*pb.BattleHero)
					helper.Guid = proto.String("isolated-star-companion")
					helper.Pose = proto.Int32(7)
					g.Run.Fight.Start.BattlePlayer.Formation = append(g.Run.Fight.Start.BattlePlayer.Formation, helper)
					start = proto.Clone(g.Run.Fight.Start).(*pb.SCCopyFightStart)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			party := proto.Clone(start.BattlePlayer).(*pb.BattlePlayer)
			if casualty && i == len(s.tasks("8"))-1 {
				party.Formation[1].Inst.CurHp = proto.Float64(0)
			}
			final = &pb.CSCopyFightEnd{FightId: start.FightId, BattlePlayer: party, Result: proto.Int32(1)}
			result, err := s.copyFightEnd(ctx, id, final)
			if err != nil {
				t.Fatal(err)
			}
			if i != len(s.tasks("8"))-1 {
				if result[0].(*pb.SCCopyFightEnd).GetStar() != 0 {
					t.Fatal("partial encounter granted completion stars")
				}
				continue
			}
			want := int32(5)
			if casualty {
				want = 4
			}
			settled := result[0].(*pb.SCCopyFightEnd)
			if settled.GetIsCopyComplete() != 1 || settled.GetStar() != want {
				t.Fatal("casualty score did not follow adopted GM rule", settled)
			}
			before, _ := read()
			cold()
			retry, err := s.copyFightEnd(ctx, id, final)
			after, saved := read()
			if err != nil || !proto.Equal(settled, retry[0]) || !proto.Equal(before, after) {
				t.Fatal("cold settlement replay drifted or rewarded twice", err)
			}
			for _, info := range saved.Copy.PassedCopyInfo {
				if info.GetMissionId() == "8" {
					best := int32(5)
					if attempt == 0 {
						best = 4
					}
					if info.GetStar() != best || info.GetChallengeTimes() != int32(attempt+1) {
						t.Fatal("best stars or one-per-completion count changed", info)
					}
				}
			}
		}
	}
	// 多人阵容伤亡的下限为一星，不产生零星通关或负数。
	party := &pb.BattlePlayer{}
	for i := 0; i < 9; i++ {
		party.Formation = append(party.Formation, &pb.BattleHero{Inst: &pb.BattleHeroInst{CurHp: proto.Float64(0)}})
	}
	if copyCompletionStars(party) != 1 {
		t.Fatal("completion star floor")
	}
}
