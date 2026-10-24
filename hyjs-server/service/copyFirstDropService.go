package service

import (
	"encoding/json"
	"errors"
	"slices"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// initializeCopyFirstDrops 单独登记首通兼容策略；普通掉落开关不改变首通配置。
func (s *GameService) initializeCopyFirstDrops(tx *gorm.DB) error {
	rule := db.CopyDropRule{ID: constUtil.CopyFirstDropRuleID, Enabled: true, Mode: constUtil.CopyDropRelativeWeights, Source: "本地兼容：原FirstDropOdds每条独立按数量/相对权重抽一次，单数量100/1000档按固定奖励；未知原服首通概率/保底不宣称复原，旧未冻结战斗保持已有原1000确定档"}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error
}

// copyFirstDropInput 随开始冻结完整首通配置，原1000确定档也不受热改影响。
func (s *GameService) copyFirstDropInput(tx *gorm.DB, content string) (*copyDropInput, error) {
	return s.copyDropInputForRule(tx, content, constUtil.CopyFirstDropRuleID)
}

// settleCopyFirstDrops 在首次实际胜利事务内执行冻结奖励；旧战斗保持旧确定档规则。
func (s *GameService) settleCopyFirstDrops(tx *gorm.DB, id int64, player *pb.Player, game *model.GameState, fight *model.CopyFight) (*pb.ResultObject, []*pb.BaseResource, error) {
	if slices.Contains(game.CopyFirstPassedTasks, fight.Task) || copyMissionPassed(game, game.Run.Chapter, game.Run.Mission) {
		return &pb.ResultObject{}, nil, nil
	}
	if !gameSchemaHasTable(tx, &db.CopyDropSnapshot{}) {
		return s.copyFirstDrops(tx, id, player, game, fight)
	}
	var saved db.CopyDropSnapshot
	err := tx.First(&saved, "player_id=? AND fight_id=?", id, fight.Start.GetFightId()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.copyFirstDrops(tx, id, player, game, fight)
	}
	if err != nil {
		return nil, nil, err
	}
	var input copyDropInput
	if saved.TaskID != fight.Task || saved.InputHash != p6Digest(saved.Input) || json.Unmarshal(saved.Input, &input) != nil {
		return nil, nil, errors.New(constUtil.CopyDropConfigurationError)
	}
	if input.First == nil {
		return s.copyFirstDrops(tx, id, player, game, fight)
	}
	gain, resources, err := s.applyCopyDropInputWithGrant(player, game, input.First, func(content string) (*pb.ResultObject, error) {
		return s.grantCopyBattleReward(tx, player, game, content, "first", func(fit string) (*pb.ResultObject, error) {
			return s.openServerGain(player, game, fit)
		})
	})
	if err != nil {
		return nil, nil, err
	}
	// 装备需要GUID投影，客户端结算与背包查询对应同一真实实例。
	for _, equip := range gain.GetEquips() {
		resources = append(resources, &pb.BaseResource{Id: equip.EquipId, Guid: equip.Guid, Num: proto.Int32(1)})
	}
	return gain, resources, nil
}
