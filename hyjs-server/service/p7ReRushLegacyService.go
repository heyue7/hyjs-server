package service

import (
	"bytes"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p7ReRushLegacyFirstSection 仅为本玩家本轮已接受的旧冻结首组保留三层资格。
// 新玩家不能借此请求发布后关闭的旧目录。
func (s *GameService) p7ReRushLegacyFirstSection(tx *gorm.DB, state *db.P7RushState) (bool, error) {
	if state.Floor > 3 {
		return false, nil
	}
	enabled, err := p7ReRushEnabled(tx)
	if err != nil {
		return false, err
	}
	return s.p7ReRushLegacyFirstSectionEnabled(tx, state, enabled)
}

func (s *GameService) p7ReRushLegacyFirstSectionEnabled(tx *gorm.DB, state *db.P7RushState, enabled bool) (bool, error) {
	if state.Floor > 3 || !enabled {
		return false, nil
	}
	offer, _, err := s.p7ReadSectionOffer(tx, state)
	if err != nil || offer != nil {
		return offer != nil, err
	}
	addition, _, err := p7ReadRushAdditionOffer(tx, state)
	return addition != nil, err
}

func (s *GameService) p7ReRushLegacyStage(tx *gorm.DB, state *db.P7RushState, stage db.P7RushStageRule) (bool, error) {
	if stage.Source != constUtil.P7SectionStageSource {
		return false, nil
	}
	legacy, err := s.p7ReRushLegacyFirstSection(tx, state)
	if err != nil || !legacy {
		return false, err
	}
	rows, err := p7SectionCatalogue(true)
	if err != nil {
		return false, err
	}
	return p7ReRushLegacyStageMatches(stage, rows)
}

// p7ReRushLegacyStageMatches 验证旧目录原文；调用方在同请求已确认冻结资格时复用。
func p7ReRushLegacyStageMatches(stage db.P7RushStageRule, rows []db.P7RushStageRule) (bool, error) {
	if stage.Source != constUtil.P7SectionStageSource {
		return false, nil
	}
	stage.Enabled = true
	for _, row := range rows {
		if row.ID != stage.ID {
			continue
		}
		a, _ := json.Marshal(row)
		b, _ := json.Marshal(stage)
		if !bytes.Equal(a, b) || !bytes.Equal(row.Enemies, stage.Enemies) {
			return false, errors.New("旧试炼首组与已冻结目录冲突")
		}
		return true, nil
	}
	return false, nil
}

// p7ReRushLegacyRequest 保留旧已领回执和未消费的真实冻结offer对应原消费者。
func (s *GameService) p7ReRushLegacyRequest(tx *gorm.DB, state *db.P7RushState, request proto.Message, stamp int64) (bool, error) {
	domain := ""
	switch request.(type) {
	case *pb.CSRushRewardPreview, *pb.CSRushGetSectionReward:
		domain = constUtil.P7SectionRewardReceipt
	case *pb.CSRushGetSectionAddition, *pb.CSRushAddition:
		domain = constUtil.P7RushAdditionReceipt
	case *pb.CSRushMazeTrader, *pb.CSRushMazeTraderBuy:
		domain = constUtil.P7MazeTraderReceipt
	default:
		return false, nil
	}
	if stamp > 0 {
		var count int64
		if err := tx.Model(&db.P7RushReceipt{}).Where("player_id=? AND domain=? AND timestamp=?", state.PlayerID, domain, stamp).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	if domain == constUtil.P7SectionRewardReceipt {
		offer, _, err := s.p7ReadSectionOffer(tx, state)
		return offer != nil && offer.ClaimedAt == 0, err
	}
	if domain == constUtil.P7MazeTraderReceipt {
		var offer db.P7MazeTraderOffer
		err := tx.First(&offer, "player_id=? AND run_id=?", state.PlayerID, state.RunID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var fight db.P7RushFight
		if err = tx.First(&fight, "id=? AND player_id=? AND run_id=?", offer.FightID, state.PlayerID, state.RunID).Error; err != nil {
			return false, err
		}
		var stage db.P7RushStageRule
		if p6Digest(fight.Rule) != fight.RuleHash || json.Unmarshal(fight.Rule, &stage) != nil {
			return false, errors.New("旧迷宫商人冻结来源损坏")
		}
		// 包括已购买、不出现和已过期的旧冻结offer，仍由原消费者决定资格。
		return stage.Source != constUtil.P7ReRushSource, nil
	}
	offer, _, err := p7ReadRushAdditionOffer(tx, state)
	return offer != nil && offer.Selected == "", err
}
