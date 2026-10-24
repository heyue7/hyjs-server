package service

import (
	"errors"
	"math"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p7RushProjectedPower 仅补齐旧目录缺失的展示战力，保留规则、敌人及冻结摘要。
// 评分权重沿用已发布Re目录，逐名合计支持历史单人和当前多人阵容。
func p7RushProjectedPower(stage db.P7RushStageRule) (float64, error) {
	if stage.RecommendedPower > 0 && !math.IsNaN(stage.RecommendedPower) && !math.IsInf(stage.RecommendedPower, 0) {
		return stage.RecommendedPower, nil
	}
	monster := &pb.BattleMonster{}
	if proto.Unmarshal(stage.Enemies, monster) != nil || len(monster.Formation) == 0 {
		return 0, errors.New(constUtil.P7RushContentMissing)
	}
	power := float64(0)
	for _, hero := range monster.Formation {
		prop := hero.GetProp()
		power += (prop.GetHp()*.25 + prop.GetAtt()*6 + prop.GetPdef()*10 + prop.GetMdef()*10) * .1
	}
	if power <= 0 || math.IsNaN(power) || math.IsInf(power, 0) {
		return 0, errors.New(constUtil.P7RushContentMissing)
	}
	return math.RoundToEven(power), nil
}
