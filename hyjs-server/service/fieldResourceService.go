package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeFieldResources 保留存量不限次数行为；仅初始化一次全局设置。
func (s *GameService) InitializeFieldResources(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.FieldResourcePolicy{}); err != nil {
		return err
	}
	row := db.FieldResourcePolicy{ID: constUtil.FieldResourcePolicyID, BossDailyRewards: constUtil.FieldBossRewardsUnlimited, GatherRefreshHours: constUtil.FieldGatherRefreshDefault}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	if err := tx.Model(&db.FieldResourcePolicy{}).Where("id=? AND (gather_refresh_hours IS NULL OR gather_refresh_hours=?)", constUtil.FieldResourcePolicyID, "").Update("gather_refresh_hours", constUtil.FieldGatherRefreshDefault).Error; err != nil {
		return err
	}
	s.FieldResourcePolicyReady = true
	return nil
}

func (s *GameService) fieldBossRewardLimit(tx *gorm.DB) (int64, error) {
	base := s
	if s.RuntimeBase != nil {
		base = s.RuntimeBase
	}
	if !base.FieldResourcePolicyReady {
		return constUtil.FieldBossRewardsUnlimited, nil
	}
	var row db.FieldResourcePolicy
	if err := tx.Session(&gorm.Session{}).Clauses(clause.Locking{Strength: "SHARE"}).First(&row, constUtil.FieldResourcePolicyID).Error; err != nil {
		return 0, err
	}
	if row.BossDailyRewards < constUtil.FieldBossRewardsUnlimited || row.BossDailyRewards > constUtil.FieldBossRewardsDailyMaximum {
		return 0, constUtil.ErrAdminConfiguration
	}
	return row.BossDailyRewards, nil
}

// fieldBossResourceContent 在原Profile锁内共享跨地图次数；成功回执在此之前返回。
// 限额沿开战冻结，旧战斗缺省不限；每日计数按实际获奖时的游戏日期推进。
func fieldBossResourceContent(ctx context.Context, field *model.FieldState, fight *model.FieldFight, content string, transactions ...*gorm.DB) (string, error) {
	if content == "" {
		return "", nil
	}
	day := LocalPlayNow(ctx).Format("2006-01-02")
	if len(transactions) > 0 {
		if err := fieldBossQuota75(transactions[0], GameCalendarOwner(ctx), field, day); err != nil {
			return "", err
		}
	}
	if field.BossRewardDay != day {
		field.BossRewardDay, field.BossRewardClaims = day, 0
	}
	if field.BossRewardClaims < 0 || field.BossRewardClaims == math.MaxInt64 {
		return "", constUtil.ErrAdminConfiguration
	}
	limit := constUtil.FieldBossRewardsUnlimited
	if fight.BossRewardDailyLimit != nil {
		limit = *fight.BossRewardDailyLimit
	}
	if limit < constUtil.FieldBossRewardsUnlimited || limit > constUtil.FieldBossRewardsDailyMaximum {
		return "", constUtil.ErrAdminConfiguration
	}
	if limit >= 0 && field.BossRewardClaims >= limit {
		return "", nil
	}
	field.BossRewardClaims++
	return content, nil
}

// fieldBossQuota75 keeps the outgoing day's complete count, including legacy
// non-death rewards. The original profile lock owns this reversible date cache.
func fieldBossQuota75(tx *gorm.DB, id int64, field *model.FieldState, day string) error {
	if !ServerGameClockContextBound(tx.Statement.Context) {
		return nil
	}
	if id < 1 {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	oldDay := field.BossRewardDay
	days := []string{day}
	if oldDay != "" {
		days = append(days, oldDay)
	}
	counts, err := fieldBossQuota75Cached(tx, id, days)
	if err != nil {
		return err
	}
	// Recover already-claimed death receipts even when the reversible cache was
	// introduced after that date. max keeps legacy grants and death grants from
	// being counted twice when the profile already contains both.
	if s, ok := tx.Statement.Context.Value(constUtil.GameServiceContextKey).(*GameService); ok && s.FieldBossDeath73Ready {
		var claimed int64
		if err = tx.Model(&db.FieldBossContribution73{}).Where("server_id=? AND player_id=? AND reward_day=? AND claimed=? AND reward_rank>0", s.OriginalServerID(), id, day, true).Count(&claimed).Error; err != nil {
			return err
		}
		counts[day] = max(counts[day], claimed)
	}
	if field.BossRewardClaims < 0 {
		return errors.New(constUtil.CalendarQuota75Invalid)
	}
	if oldDay != "" && field.BossRewardClaims > counts[oldDay] {
		if err = calendarQuota75Save(tx, id, constUtil.CalendarQuota75FieldBoss, oldDay, field.BossRewardClaims); err != nil {
			return err
		}
		counts[oldDay] = field.BossRewardClaims
	}
	field.BossRewardDay = day
	field.BossRewardClaims = counts[day]
	return nil
}

func fieldBossQuota75Cached(tx *gorm.DB, id int64, days []string) (map[string]int64, error) {
	counts := map[string]int64{}
	if !ServerGameClockContextBound(tx.Statement.Context) {
		return counts, nil
	}
	keys := []string{}
	prefix := "calendar75:" + constUtil.CalendarQuota75FieldBoss + ":"
	for _, day := range days {
		if _, err := time.Parse("2006-01-02", day); err != nil {
			return nil, errors.New(constUtil.CalendarQuota75Invalid)
		}
		keys = append(keys, prefix+day)
	}
	var rows []db.LocalPlayActivityOperation
	if err := tx.Select("request_id,action,activity_id,response").Where("player_id=? AND request_id IN ?", id, keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var used int64
		if row.Action != constUtil.CalendarQuota75Action || row.ActivityID != constUtil.CalendarQuota75FieldBoss || len(row.Response) > constUtil.CalendarQuota75MaxBytes || json.Unmarshal(row.Response, &used) != nil || used < 0 {
			return nil, errors.New(constUtil.CalendarQuota75Invalid)
		}
		counts[strings.TrimPrefix(row.RequestID, prefix)] = used
	}
	return counts, nil
}

func fieldResourceResponse(row db.FieldResourcePolicy) *model.AdminFieldResourcesResponse {
	var hours []int64
	_ = json.Unmarshal([]byte(row.GatherRefreshHours), &hours)
	return &model.AdminFieldResourcesResponse{BossDailyRewards: row.BossDailyRewards, GatherRefreshHours: hours, Version: adminVersion(row)}
}

// AdminFieldResources 只读全局规则；不会创建周期或发放资源。
func (s *GameService) AdminFieldResources(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminFieldResourcesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var row db.FieldResourcePolicy
	if err := tx.First(&row, constUtil.FieldResourcePolicyID).Error; err != nil {
		return nil, err
	}
	return fieldResourceResponse(row), nil
}

// AdminSaveFieldResources 原子保存并审计，已接受战斗继续沿其冻结限额。
func (s *GameService) AdminSaveFieldResources(ctx context.Context, actor *model.AdminPrincipal, req model.AdminFieldResourcesSaveRequest) (*model.AdminFieldResourcesResponse, error) {
	if req.GatherRefreshHours != nil && !validFieldRefreshHours(req.GatherRefreshHours) || req.BossDailyRewards < constUtil.FieldBossRewardsUnlimited || req.BossDailyRewards > constUtil.FieldBossRewardsDailyMaximum || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len([]rune(req.Reason)) > 500 {
		return nil, constUtil.ErrAdminInput
	}
	var out *model.AdminFieldResourcesResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var row db.FieldResourcePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, constUtil.FieldResourcePolicyID).Error; err != nil {
			return err
		}
		if adminVersion(row) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		before := row
		hours := append([]int64(nil), req.GatherRefreshHours...)
		if req.GatherRefreshHours == nil {
			if json.Unmarshal([]byte(row.GatherRefreshHours), &hours) != nil || !validFieldRefreshHours(hours) {
				return constUtil.ErrAdminConfiguration
			}
		}
		sort.Slice(hours, func(i, j int) bool { return hours[i] < hours[j] })
		encoded, _ := json.Marshal(hours)
		if row.BossDailyRewards != req.BossDailyRewards || row.GatherRefreshHours != string(encoded) {
			row.GatherRefreshHours = string(encoded)
			row.BossDailyRewards, row.Revision = req.BossDailyRewards, row.Revision+1
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			if err := adminAudit(tx, actor, constUtil.FieldResourceAuditAction, constUtil.FieldResourceAuditTarget, fmt.Sprintf("before=%s after=%s; %s", adminJSON(before), adminJSON(row), strings.TrimSpace(req.Reason))); err != nil {
				return err
			}
		}
		out = fieldResourceResponse(row)
		return nil
	})
	return out, err
}

// validFieldRefreshHours bounds the persisted daily schedule and rejects duplicates.
func validFieldRefreshHours(hours []int64) bool {
	if len(hours) < 1 || len(hours) > 24 {
		return false
	}
	seen := map[int64]bool{}
	for _, h := range hours {
		if h < 0 || h > 23 || seen[h] {
			return false
		}
		seen[h] = true
	}
	return true
}

// fieldGatherBoundary uses fixed UTC+8 so server locale and DST cannot move refreshes.
func fieldGatherBoundary(now int64, hours []int64) int64 {
	local := time.UnixMilli(now).UTC().Add(8 * time.Hour)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC).Add(-8 * time.Hour)
	latest := int64(0)
	for _, h := range hours {
		candidate := midnight.Add(time.Duration(h) * time.Hour).UnixMilli()
		if candidate > now {
			candidate -= int64(24 * time.Hour / time.Millisecond)
		}
		if candidate > latest {
			latest = candidate
		}
	}
	return latest
}

func (s *GameService) fieldGatherRefreshBoundary(tx *gorm.DB, now int64) (int64, error) {
	base := s
	if s.RuntimeBase != nil {
		base = s.RuntimeBase
	}
	hours := []int64{0, 12}
	if base.FieldResourcePolicyReady {
		var row db.FieldResourcePolicy
		if err := tx.First(&row, constUtil.FieldResourcePolicyID).Error; err != nil {
			return 0, err
		}
		if json.Unmarshal([]byte(row.GatherRefreshHours), &hours) != nil || !validFieldRefreshHours(hours) {
			return 0, constUtil.ErrAdminConfiguration
		}
	}
	return fieldGatherBoundary(now, hours), nil
}
