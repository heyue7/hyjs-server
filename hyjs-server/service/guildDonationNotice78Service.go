package service

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// projectGuildDonationNotices78 explains the original paid source without changing receipts.
func projectGuildDonationNotices78(tx *gorm.DB, playerID int64, rows []db.RewardDelivery) error {
	var ids []int64
	for _, row := range rows {
		if strings.HasPrefix(row.SourceKey, "p13:coin-badge:") {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 || !gameSchemaHasTable(tx, &db.P13GuildDonationBadgeSource{}) {
		return nil
	}
	var sources []db.P13GuildDonationBadgeSource
	if err := tx.Where("player_id=? AND delivery_id IN ?", playerID, ids).Find(&sources).Error; err != nil {
		return err
	}
	byID := make(map[int64]db.P13GuildDonationBadgeSource, len(sources))
	for _, source := range sources {
		if source.SourceHash == p13GuildDonationBadgeSourceHash(source) {
			byID[source.DeliveryID] = source
		}
	}
	for i := range rows {
		source, ok := byID[rows[i].ID]
		if ok && source.DeliveryKey == rows[i].SourceKey && rows[i].Rewards == fmt.Sprintf("item_089,%d", source.BadgeAmount) {
			rows[i].Content = fmt.Sprintf(constUtil.P13GuildDonationBadgeNotice78, source.CoinCost)
		}
	}
	return nil
}
