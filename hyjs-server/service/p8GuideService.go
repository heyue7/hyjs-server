package service

import (
	"context"
	"errors"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p8GuideResource completes the original fixed-resource tutorial at its separate pos0.
// Profile serialization and the original completed guide order make this a lifetime award.
func (s *GameService) p8GuideResource(ctx context.Context, id int64, request proto.Message, stamp int64, fish bool) ([]proto.Message, error) {
	var (
		operation               = constUtil.P8GuideMineOperation
		position                = "mine_pos_0"
		config                  = "GuideMine"
		response  proto.Message = &pb.SCGuideMine{}
	)
	if fish {
		operation, position, config = constUtil.P8GuideFishOperation, "fish_pos_0", "GuideFish"
		response = &pb.SCGuideFish{}
	}
	return s.p8GatherMutation(ctx, id, operation, stamp, request, response, func(tx *gorm.DB, p *pb.Player, g *model.GameState, _ int64) error {
		// The client creates these guide-only points in the first map, independently of its random nodes.
		if g.Position.GetScene() != "field_001" || p.GetLevel() < 18 ||
			fish && p.GetNoviceGuideSchedule() >= constUtil.P8GuideFishSchedule ||
			!fish && p.GetNoviceGuideSchedule() != constUtil.P8GuideFishSchedule {
			return errors.New("生活引导等级、顺序或完成进度不满足")
		}
		row := s.table("FieldResourcePos", "posId", position)
		point, err := fieldPoint(row["pos"])
		if err != nil {
			return err
		}
		radius, err := strconv.ParseFloat(row["r"], 64)
		if err != nil || radius <= 0 || radius > 100 || !p8Near(g.Position.GetCurrent(), &p8GatherConfig{Point: point, Radius: radius}) {
			return p8GatherError()
		}
		var active int64
		if err = tx.Model(&db.P8GatherAttempt{}).Where("player_id=? AND active=?", id, true).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errors.New("普通采集尚未结束，不能领取生活引导")
		}
		// Preserve the original configured amounts; ordinary weighted rewards and fatigue remain separate.
		reward := s.table("GameConfigure", "StaticName", config)["NameValue"]
		if reward == "" {
			return errors.New("原生活引导固定材料配置缺失")
		}
		gain, err := s.GrantFixedReward(p, g, reward)
		if err != nil {
			return err
		}
		if fish {
			response.(*pb.SCGuideFish).Get = gain
			p.NoviceGuideSchedule = proto.Int32(constUtil.P8GuideFishSchedule)
		} else {
			response.(*pb.SCGuideMine).Get = gain
			p.NoviceGuideSchedule = proto.Int32(constUtil.P8GuideMineSchedule)
		}
		g.Field.Side.State = proto.Int32(constUtil.P8FieldIdle)
		return nil
	})
}
