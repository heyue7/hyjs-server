package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// InitializeP13GuildDonationBadge seeds only a disabled, explicitly local source.
func (s *GameService) InitializeP13GuildDonationBadge(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P13GuildDonationBadgePolicy{}, &db.P13GuildDonationBadgeSource{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P13GuildDonationBadgePolicy{ID: 1, Version: constUtil.P13GuildDonationBadgeVersion, Source: constUtil.P13GuildDonationBadgeSource, CoinCost: constUtil.P13GuildDonationBadgeCoinCost, BadgeAmount: constUtil.P13GuildDonationBadgeAmount, DailyLimit: constUtil.P13GuildDonationBadgeDailyLimit}).Error
}

func p13GuildDonationBadgePolicyValid(policy *db.P13GuildDonationBadgePolicy, now time.Time) bool {
	return policy.Version == constUtil.P13GuildDonationBadgeVersion && policy.Source == constUtil.P13GuildDonationBadgeSource && policy.CoinCost == constUtil.P13GuildDonationBadgeCoinCost && policy.BadgeAmount == constUtil.P13GuildDonationBadgeAmount && policy.DailyLimit == constUtil.P13GuildDonationBadgeDailyLimit && policy.PublishedAt > 0 && policy.PublishedAt <= now.UnixMilli()
}

func p13GuildDonationBadgeSourceHash(row db.P13GuildDonationBadgeSource) string {
	row.SourceHash = ""
	raw, _ := json.Marshal(row)
	return p6Digest(raw)
}

// RecordP13GuildDonationBadge follows the real coin debit inside the existing
// guild/profile transaction. The original precise request receipt prevents any
// delayed retry from creating another delivery after reopening the policy.
func (s *GameService) RecordP13GuildDonationBadge(tx *gorm.DB, member *db.P13GuildMember, request *pb.CSUnionDonate, stamp int64, amount int32, now time.Time) error {
	// Badge donations deliberately do not produce badges, preventing a costless cycle.
	if request.GetDonateType() != "coin" || !tx.Migrator().HasTable(&db.P13GuildDonationBadgePolicy{}) {
		return nil
	}
	var policy db.P13GuildDonationBadgePolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	if !p13GuildDonationBadgePolicyValid(&policy, now) {
		return errors.New("公会捐献徽章来源未提前发布或与具名规则不符")
	}
	minimum, err := s.p10Setting("UnionMinDailyDonateCoin")
	if err != nil {
		return err
	}
	maximum, err := s.p10Setting("UnionMaxDailyDonateCoin")
	if err != nil {
		return err
	}
	if minimum != policy.CoinCost || int64(maximum) != int64(policy.CoinCost)*policy.DailyLimit || amount != minimum || member.GuildID == "" || member.JoinedAt <= 0 || member.JoinedAt > now.UnixMilli() || member.DonateDay != p13GuildDay(now) || member.TodayCoin < int64(amount) || member.TodayCoin > int64(maximum) || stamp <= 0 {
		return errors.New("公会捐献徽章实际金币来源与原限额不符")
	}
	day := p13GuildDay(now)
	var previous []db.P13GuildDonationBadgeSource
	if err = tx.Where("player_id=? AND day=?", member.PlayerID, day).Order("ordinal").Find(&previous).Error; err != nil {
		return err
	}
	for index, saved := range previous {
		if saved.Ordinal != int64(index+1) || saved.SourceHash != p13GuildDonationBadgeSourceHash(saved) || saved.Version != policy.Version || saved.Source != policy.Source || saved.CoinCost != policy.CoinCost || saved.BadgeAmount != policy.BadgeAmount || saved.DailyLimit != policy.DailyLimit || saved.Stamp <= 0 || saved.GuildID == "" || saved.JoinedAt <= 0 || saved.JoinedAt > saved.CreatedAt || saved.PublishedAt <= 0 || saved.PublishedAt > saved.CreatedAt || saved.CreatedAt > now.UnixMilli() || saved.DeliveryID <= 0 || saved.DeliveryKey != fmt.Sprintf("p13:coin-badge:%d:%d", saved.PlayerID, saved.Stamp) {
			return errors.New("公会捐献徽章已冻结来源损坏")
		}
	}
	if int64(len(previous)) >= policy.DailyLimit {
		return errors.New("公会捐献徽章每日实际来源已达上限")
	}
	if int64(len(previous)+1)*int64(amount) > member.TodayCoin {
		return errors.New("公会捐献徽章来源超过当日实际金币捐献")
	}
	// Reopening cannot rewrite the earlier publication frozen for today's source.
	if len(previous) > 0 {
		policy.PublishedAt = previous[0].PublishedAt
	}
	rewards := fmt.Sprintf("item_089,%d", policy.BadgeAmount)
	key := fmt.Sprintf("p13:coin-badge:%d:%d", member.PlayerID, stamp)
	delivery, err := s.EnqueueReward(tx, member.PlayerID, key, constUtil.P13GuildDonationBadgeTitle, constUtil.P13GuildDonationBadgeContent, rewards, 0)
	if err != nil {
		return err
	}
	if delivery.PlayerID != member.PlayerID || delivery.SourceKey != key || delivery.Rewards != rewards || delivery.ClaimedAt != 0 || delivery.ExpiresAt != 0 {
		return errors.New("公会捐献徽章P1投递来源冲突")
	}
	row := db.P13GuildDonationBadgeSource{PlayerID: member.PlayerID, Stamp: stamp, Day: day, Ordinal: int64(len(previous) + 1), GuildID: member.GuildID, JoinedAt: member.JoinedAt, CoinCost: amount, BadgeAmount: policy.BadgeAmount, DailyLimit: policy.DailyLimit, Version: policy.Version, Source: policy.Source, PublishedAt: policy.PublishedAt, CreatedAt: now.UnixMilli(), DeliveryID: delivery.ID, DeliveryKey: key}
	row.SourceHash = p13GuildDonationBadgeSourceHash(row)
	return tx.Create(&row).Error
}
