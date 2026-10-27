package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type rechargeOpeningScope75 struct{}

// BackfillGrowth75PublishedAt restores this one audited V12 publication boundary.
// It never infers a publication date from a player's first visit or today's offset.
func (s *GameService) BackfillGrowth75PublishedAt(ctx context.Context, a *model.AdminPrincipal) error {
	if s.OriginalServerID() != 1 {
		return constUtil.ErrAdminConfiguration
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, a, true); err != nil {
			return err
		}
		var proof db.AdminAudit
		if err := tx.First(&proof, "id=?", 321).Error; err != nil {
			return err
		}
		if proof.Time != 1793026861377 || proof.Action != "activity.schedule.save" || proof.Target != "1" || proof.Detail != "realm=1 revision=10 slots=820" {
			return constUtil.ErrAdminConfiguration
		}
		rows, err := growth75AuditedPublicationRows(tx, proof.Time)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(rows))
		for _, row := range rows {
			if row.FirstPublishedAt != 0 {
				continue
			}
			ids = append(ids, row.ID)
		}
		if len(ids) == 0 {
			return nil
		}
		if len(ids) > 820 {
			return constUtil.ErrAdminConfiguration
		}
		result := tx.Model(&db.AdminActivityOccurrence{}).Where("id IN ? AND first_published_at=0", ids).Update("first_published_at", proof.Time)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return constUtil.ErrAdminConfigConflict
		}
		return adminAudit(tx, a, "growth75.activity.publication", "1", fmt.Sprintf("restored=%d; sourceAudit=%d; realPublishedAt=%d; original orders, projections, claims and property retained", result.RowsAffected, proof.ID, proof.Time))
	})
}

// A backdated game window does not move its real publication into the past.
// Character windows share their source publication and retain natural role birth.
func rechargeActivityRealStart75(tx *gorm.DB, playerID int64, key string) (int64, error) {
	if !adminActivitySchemaHasTable(tx, &db.AdminActivityOccurrence{}) {
		return 0, nil
	}
	var source db.AdminActivityOccurrence
	occurrenceID := ""
	if key == constUtil.OpenServerActivityID {
		if scope, _ := tx.Statement.Context.Value(rechargeOpeningScope75{}).(*adminOpeningScope); scope != nil {
			if scope.PlayerID != playerID {
				return 0, constUtil.ErrAdminConfiguration
			}
			occurrenceID = scope.OccurrenceID
		}
	}
	if strings.Contains(key, "_character_") {
		var issued db.AdminCharacterActivityInstance
		if err := tx.Select("player_id", "occurrence_id").Where("consumer_key=?", key).Take(&issued).Error; err != nil {
			return 0, err
		}
		if issued.PlayerID != playerID {
			return 0, constUtil.ErrAdminConfiguration
		}
		occurrenceID = issued.OccurrenceID
	}
	if occurrenceID != "" {
		if err := tx.Select("id", "first_published_at").First(&source, "id=?", occurrenceID).Error; err != nil {
			return 0, err
		}
		var profile db.Profile
		if err := tx.Select("id", "created_at").First(&profile, playerID).Error; err != nil {
			return 0, err
		}
		if profile.CreatedAt != nil {
			return max(source.FirstPublishedAt, *profile.CreatedAt), nil
		}
		return source.FirstPublishedAt, nil
	}
	err := tx.Select("id", "first_published_at").Where("consumer_key=?", key).Take(&source).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return source.FirstPublishedAt, err
}

func rechargePeriodValue75(tx *gorm.DB, playerID int64, cfg db.PeriodInstance, now int64) (int64, string, error) {
	if value, day, carried, err := rechargeCarryValue(tx, playerID, cfg, now); carried || err != nil {
		return value, day, err
	}
	orders, err := localPlayActivityPaymentOrders(tx, playerID, cfg.ID, cfg.StartAt, min(cfg.EndAt, now+1))
	if err != nil {
		return 0, "", err
	}
	var value int64
	lastDay := ""
	for _, order := range orders {
		if order.Cents <= 0 {
			return 0, "", p3Error()
		}
		day := p3Day(time.UnixMilli(order.ConfirmedAt))
		if cfg.Mode == "daily" && day != lastDay {
			value = 0
		}
		if cfg.Mode == "single" {
			value = max(value, order.Cents)
		} else {
			if value > math.MaxInt64-order.Cents {
				return 0, "", p3Error()
			}
			value += order.Cents
		}
		lastDay = day
	}
	return value, lastDay, nil
}

func (s *GameService) reconcileRechargeProgress75(tx *gorm.DB, progress *db.PeriodProgress, cfg db.PeriodInstance, now int64) error {
	value, day, err := rechargePeriodValue75(tx, progress.PlayerID, cfg, now)
	if err != nil {
		return err
	}
	if progress.Value == value && progress.LastDay == day {
		return nil
	}
	progress.Value, progress.LastDay = value, day
	return tx.Save(progress).Error
}

// The paid status and immutable projection are saved in the original confirmation
// transaction before activity facts; a failed activity rolls the whole payment back.
func rechargeEventRealAt75(tx *gorm.DB, playerID int64, source string) (int64, error) {
	if !strings.HasPrefix(source, "payment:") {
		return 0, p3Error()
	}
	var row struct{ RealAt int64 }
	err := tx.Table("p3_payment_orders AS pay").Select("COALESCE(play.real_at,pay.confirmed_at) AS real_at").Joins("LEFT JOIN local_play_payment_times AS play ON play.order_id=pay.id AND play.player_id=pay.player_id").Where("pay.id=? AND pay.player_id=? AND pay.status=?", strings.TrimPrefix(source, "payment:"), playerID, constUtil.P3PaymentConfirmed).Take(&row).Error
	if err != nil || row.RealAt <= 0 {
		if err != nil {
			return 0, err
		}
		return 0, p3Error()
	}
	return row.RealAt, nil
}

// rechargeRewardEligibility75 保留历史充值资格核算供迁移回归复用；
// 领奖中心已排除活动附件，活动页按冻结周期单独核验领取资格。
func rechargeRewardEligibility75(tx *gorm.DB, rows []db.RewardDelivery, now int64) (map[int64]bool, error) {
	allowed := map[int64]bool{}
	keys := map[string]bool{}
	byReward := map[int64]string{}
	for _, row := range rows {
		allowed[row.ID] = true
		if row.ClaimedAt != 0 || !strings.HasPrefix(row.SourceKey, constUtil.P1ActivityRewardSourcePrefix) {
			continue
		}
		key := strings.TrimPrefix(row.SourceKey, constUtil.P1ActivityRewardSourcePrefix)
		if index := strings.LastIndex(key, ":tier:"); index >= 0 {
			key = key[:index]
		}
		keys[key] = true
		byReward[row.ID] = key
	}
	if len(keys) == 0 {
		return allowed, nil
	}
	ids := make([]string, 0, len(keys))
	for key := range keys {
		ids = append(ids, key)
	}
	var periods []db.PeriodInstance
	if err := tx.Where("id IN ?", ids).Find(&periods).Error; err != nil {
		return nil, err
	}
	configs := map[string]db.PeriodInstance{}
	paymentIDs := []string{}
	for _, live := range periods {
		cfg, err := p1PeriodSnapshot(live)
		if err != nil {
			return nil, err
		}
		if cfg.Kind != "payment_amount" {
			continue
		}
		configs[cfg.ID] = cfg
		paymentIDs = append(paymentIDs, cfg.ID)
	}
	if len(paymentIDs) == 0 {
		return allowed, nil
	}
	var progress []db.PeriodProgress
	if err := tx.Select("instance_id", "snapshot").Where("player_id=? AND instance_id IN ?", rows[0].PlayerID, paymentIDs).Find(&progress).Error; err != nil {
		return nil, err
	}
	for _, saved := range progress {
		var frozen db.PeriodInstance
		if json.Unmarshal(saved.Snapshot, &frozen) != nil || frozen.ID != saved.InstanceID || frozen.Kind != "payment_amount" {
			return nil, constUtil.ErrAdminConfiguration
		}
		configs[frozen.ID] = frozen
	}
	for _, cfg := range configs {
		value, _, err := rechargePeriodValue75(tx, rows[0].PlayerID, cfg, now)
		if err != nil {
			return nil, err
		}
		sources := p1PeriodSources(cfg)
		tiers := p1PeriodMilestones(cfg)
		for _, row := range rows {
			if row.ClaimedAt != 0 || byReward[row.ID] != cfg.ID {
				continue
			}
			allowed[row.ID] = false
			for index, source := range sources {
				if source == row.SourceKey && value >= tiers[index].Goal && now >= cfg.StartAt && now < cfg.RewardEndAt {
					allowed[row.ID] = true
				}
			}
		}
	}
	return allowed, nil
}

// eligiblePendingRewards75 在 SQL 内排除活动附件，按 ID 分批读取中心奖励；
// 活动历史不占客户端页数，也不触发充值资格查询。
func eligiblePendingRewards75(tx *gorm.DB, playerID, now int64, wanted int, brief bool) ([]db.RewardDelivery, error) {
	if wanted < 1 {
		return []db.RewardDelivery{}, nil
	}
	result := make([]db.RewardDelivery, 0, wanted)
	var cursor int64
	for len(result) < wanted {
		var rows []db.RewardDelivery
		limit := min(wanted-len(result), constUtil.P1RewardMaximumBatch)
		// 源键按字节区分大小写；SUBSTR 在 MySQL varbinary 和 SQLite 上均保留精确前缀语义。
		query := tx.Where("player_id=? AND id>? AND claimed_at=0 AND (expires_at=0 OR expires_at>?) AND SUBSTR(source_key,1,?)<>?", playerID, cursor, now, len(constUtil.P1ActivityRewardSourcePrefix), constUtil.P1ActivityRewardSourcePrefix).Order("id ASC").Limit(limit)
		if brief {
			query = query.Select("id", "player_id", "source_key", "claimed_at")
		}
		if err := query.Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		result = append(result, rows...)
		cursor = rows[len(rows)-1].ID
		if len(rows) < limit {
			break
		}
	}
	return result, nil
}
