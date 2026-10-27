package service

import (
	"context"
	"errors"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"slices"
	"strconv"

	"google.golang.org/protobuf/proto"
)

// RepairLegacyFirstMission 修复早期已结算首关漏发经验的存档。
// 仅匹配已有首关通关记录但仍为 1 级零经验的早期存档，金币和物品保持原值。
func (s *GameService) RepairLegacyFirstMission(ctx context.Context) error {
	clockCtx, _, err := s.bindServerGameClockWorker75(ctx)
	if err != nil {
		return err
	}
	ctx = clockCtx
	var rows []db.Profile
	if err := db.GetIns().WithContext(ctx).Where("player IS NOT NULL AND game IS NOT NULL").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := updateProfile(ctx, row.Id, func(p *pb.Player, g *model.GameState) error {
			// 已领取的任务提供重登引导断点，离开副本也保留此进度。
			if slices.Contains(g.Quests, "quest_1") && p.GetNoviceGuideSchedule() < 1 {
				p.NoviceGuideSchedule = proto.Int32(1)
			}
			if p.GetLevel() != 1 || p.GetExp() != 0 {
				return nil
			}
			passed := false
			for _, copy := range g.Copy.GetPassedCopyInfo() {
				if copy.GetChapterId() == "1" && copy.GetMissionId() == "1" && copy.GetStar() > 0 && copy.GetChallengeTimes() == 1 {
					passed = true
				}
			}
			if !passed {
				return nil
			}
			// 旧版离开副本会删除 run；已持久化的通关记录仍能证明首关完成。
			var result *pb.SCCopyFightEnd
			if g.Run != nil {
				run := g.Run
				if !run.Complete || run.Chapter != "1" || run.Mission != "1" || run.Fight == nil || !run.Fight.Settled {
					return nil
				}
				result = run.Fight.Result
				if result.GetIsCopyComplete() != 1 || result.GetFightResult() != 1 {
					return nil
				}
				for _, gain := range result.GetGain().GetSimps() {
					if gain.GetId() == "exp" {
						return nil
					}
				}
			}

			reward := s.table("CopyExp", "level", "1")
			if reward == nil {
				return errors.New("Missing first-mission reward table")
			}

			exp := int64(tableNumber(reward, "exp"))
			s.addExperience(p, exp)
			if result != nil {
				if result.Gain == nil {
					result.Gain = &pb.ResultObject{}
				}
				result.Gain.Simps = append(result.Gain.Simps, &pb.SimpleItem{Id: proto.String("exp"), Num: proto.Int32(int32(exp))})
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) addExperience(player *pb.Player, exp int64) {
	player.Exp = proto.Int64(player.GetExp() + exp)
	for {
		level := s.table("UserExp", "level", strconv.Itoa(int(player.GetLevel())))
		next := s.table("UserExp", "level", strconv.Itoa(int(player.GetLevel()+1)))
		if level == nil || next == nil || player.GetExp() < int64(tableNumber(level, "exp")) {
			break
		}
		player.Exp = proto.Int64(player.GetExp() - int64(tableNumber(level, "exp")))
		player.Level = proto.Int32(player.GetLevel() + 1)
		player.Stamina = proto.Int32(player.GetStamina() + int32(tableNumber(next, "stamina")))
		player.Energy = proto.Int32(player.GetEnergy() + int32(tableNumber(next, "energy")))
	}
}
