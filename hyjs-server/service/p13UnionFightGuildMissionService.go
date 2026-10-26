package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"time"
)

// p13UFGuildMissionFactoryOpened runs after the real original cost and factory
// state were saved in the same guild-locked transaction. Old openings are never inferred.
func p13UFGuildMissionFactoryOpened(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer, factory *db.P13UnionFightFactoryState, now time.Time) (bool, error) {
	var offer db.P13UnionFightSettlementOffer
	err := tx.First(&offer, "match_id=?", match.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	rule, err := p13UFSettlementRules(&offer, match)
	if err != nil {
		return false, err
	}
	if rule.FactoryMissionPrestige == 0 {
		return false, nil
	}
	if rule.FactoryMissionPrestige < 0 || factory.Camp != actor.Camp || factory.OpenedAt != now.UnixMilli() || now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
		return false, errors.New("公会任务缺本场真实工厂开启")
	}
	var existing db.P13UnionFightGuildMissionFact
	err = tx.First(&existing, "match_id=? AND guild_id=?", match.ID, actor.GuildID).Error
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	frame, err := p13UFFactoryFrame(factory)
	if err != nil {
		return false, err
	}
	if frame.GetIsOpen() != 1 {
		return false, errors.New("实际工厂尚未开启")
	}
	fact := db.P13UnionFightGuildMissionFact{MatchID: match.ID, GuildID: actor.GuildID, FactoryGUID: factory.GUID, PlayerID: actor.PlayerID, Prestige: rule.FactoryMissionPrestige, OfferHash: offer.RulesHash, OccurredAt: now.UnixMilli(), Source: constUtil.P13UnionFightGuildMissionSource}
	if err = tx.Create(&fact).Error; err != nil {
		return false, err
	}
	if p13UFAbortEnabled(rule) {
		return true, nil
	} // The real fact is immediate; economic prestige waits for a certified terminal.
	day, err := time.ParseInLocation("2006-01-02", match.Day, time.FixedZone("Asia/Shanghai", 8*3600))
	if err != nil {
		return false, err
	}
	var ledger db.P13UnionFightSettlementPrestige
	err = tx.First(&ledger, "week_start=? AND guild_id=?", p13UFWeek(day), actor.GuildID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ledger = db.P13UnionFightSettlementPrestige{WeekStart: p13UFWeek(day), GuildID: actor.GuildID}
	} else if err != nil {
		return false, err
	}
	if ledger.Score < 0 || ledger.Score > math.MaxInt32-int64(fact.Prestige) {
		return false, errors.New("公会任务威望溢出")
	}
	ledger.Score += int64(fact.Prestige)
	return true, tx.Save(&ledger).Error
}

func p13UFGuildMissionPrestige(tx *gorm.DB, match *db.P13UnionFightMatch, guild string) (int32, error) {
	var fact db.P13UnionFightGuildMissionFact
	err := tx.First(&fact, "match_id=? AND guild_id=?", match.ID, guild).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var offer db.P13UnionFightSettlementOffer
	if err = tx.First(&offer, "match_id=?", match.ID).Error; err != nil {
		return 0, err
	}
	rule, err := p13UFSettlementRules(&offer, match)
	if err != nil {
		return 0, err
	}
	if fact.Source != constUtil.P13UnionFightGuildMissionSource || fact.OfferHash != offer.RulesHash || fact.Prestige <= 0 || fact.Prestige != rule.FactoryMissionPrestige || fact.OccurredAt < match.StartAt || fact.OccurredAt >= match.EndAt || (guild != match.LeftGuildID && guild != match.RightGuildID) {
		return 0, errors.New("公会任务真实开启事实无效")
	}
	return fact.Prestige, nil
}
func p13UFGuildMissionFrames(tx *gorm.DB, match *db.P13UnionFightMatch, actor *db.P13UnionFightFieldPlayer) ([]*pb.InstUFUnionMission, error) {
	rule, err := p13UFMissionRules(tx, match)
	if err != nil || rule == nil || rule.FactoryMissionPrestige == 0 {
		return nil, err
	}
	prestige, err := p13UFGuildMissionPrestige(tx, match, actor.GuildID)
	if err != nil {
		return nil, err
	}
	done := int32(0)
	progress := "0/1"
	if prestige > 0 {
		done = 1
		progress = "1/1"
	}
	return []*pb.InstUFUnionMission{{UnionId: proto.String(actor.GuildID), MissionType: proto.Int32(constUtil.P13UnionFightGuildMissionFactoryKind), MissionIndex: proto.Int32(0), MissionBalanceType: proto.Int32(0), Progress: proto.String(progress), RewardNum: proto.Int32(rule.FactoryMissionPrestige), IsComplete: proto.Int32(done), MissionDesc: proto.String("desc_battel_mission_09"), MissionIcon: proto.String("")}}, nil
}
func (s *GameService) p13UFGuildMissionNotify(ctx context.Context, scope, guild string) {
	var actors []db.P13UnionFightFieldPlayer
	if db.GetIns().WithContext(ctx).Where("match_id=? AND guild_id=? AND left_at=0", scope, guild).Find(&actors).Error != nil {
		return
	}
	for _, actor := range actors {
		frame, err := p13UFFieldReadActor(&actor)
		if err == nil {
			s.publishP10Private(ctx, actor.PlayerID, &pb.SCUFMissionPush{PlayerFightScore: frame.OwnScore})
		}
	}
}
