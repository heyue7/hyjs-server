package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type adminKingRecruitSource struct {
	Pool  db.GameplayPool     `json:"pool"`
	Rules db.KingRecruitRules `json:"rules"`
}
type kingRecruitRulesScopeKey struct{}

func adminKingRecruitPoolID(key string) bool {
	return key == "recruit_3" || strings.HasPrefix(key, constUtil.AdminKingRecruitPrefix)
}
func validateKingRecruitActivityRules(r db.KingRecruitRules) error {
	if r.DailyLimit != 50 || r.LuckLimit != 500 || r.LuckPerDraw != 10 || r.RotationMillis != 172800000 || r.EpochMillis <= 0 {
		return constUtil.ErrAdminConfiguration
	}
	return nil
}
func (s *GameService) validateKingRecruitActivityPool(pool db.GameplayPool, rewards []db.GameplayPoolReward) error {
	if err := s.validateRecruitClientPrices(&pool, 3); err != nil {
		return err
	}
	if len(rewards) == 0 || len(rewards) > 1000 {
		return constUtil.ErrAdminInput
	}
	ids := map[int32]bool{}
	guaranteed := map[string]bool{}
	total := int64(0)
	for _, reward := range rewards {
		hero := s.table("Hero", "id", reward.ResourceID)
		if reward.ID < 1 || ids[reward.ID] || reward.Amount != 1 || reward.Stock != 0 || reward.Weight < 1 || reward.Weight > 1000000000000 || total > math.MaxInt64-reward.Weight || hero == nil || hero["heroClass"] != "0" || (hero["quality"] != "20" && hero["quality"] != "23") || (hero["space"] != "1" && hero["space"] != "2" && hero["space"] != "3") || reward.Guaranteed && hero["quality"] != "23" {
			return constUtil.ErrAdminInput
		}
		ids[reward.ID] = true
		total += reward.Weight
		if reward.Guaranteed {
			guaranteed[hero["space"]] = true
		}
	}
	for _, space := range []string{"1", "2", "3"} {
		if !guaranteed[space] {
			return constUtil.ErrAdminConfiguration
		}
	}
	return nil
}

// Only the fixed original rules are canonicalized. Operators choose actual
// heroes and weights explicitly; no extra rewards are inserted here.
func (s *GameService) adminKingRecruitCanonicalForm(tx *gorm.DB, form model.AdminActivityForm) (model.AdminActivityForm, error) {
	var source adminKingRecruitSource
	if err := tx.First(&source.Pool, "id=?", "recruit_3").Error; err != nil {
		return form, err
	}
	if err := tx.First(&source.Rules, 1).Error; err != nil {
		return form, err
	}
	if err := s.validateRecruitClientPrices(&source.Pool, 3); err != nil {
		return form, err
	}
	if err := validateKingRecruitActivityRules(source.Rules); err != nil {
		return form, err
	}
	form.Options.KingPoolSource = string(adminJSON(source))
	return form, nil
}

func (s *GameService) adminKingRecruitActivityForm(tx *gorm.DB, cfg db.GameplayPool, rewards []db.GameplayPoolReward) (model.AdminActivityForm, error) {
	form := model.AdminActivityForm{Mode: constUtil.AdminKingRecruitMode, TemplateID: constUtil.AdminKingRecruitMode + "-original", Name: cfg.Name, Source: cfg.Source, StartAt: cfg.StartAt, EndAt: cfg.EndAt, ClaimEndAt: cfg.RewardEndAt, Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
	for _, r := range rewards {
		form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(r.ID), ResourceID: r.ResourceID, Amount: 1, Weight: r.Weight, Stock: 0, Guaranteed: r.Guaranteed})
	}
	if err := s.validateKingRecruitActivityPool(cfg, rewards); err != nil {
		return form, err
	}
	var snapshot db.KingRecruitActivitySnapshot
	var source adminKingRecruitSource
	err := gorm.ErrRecordNotFound
	if tx.Migrator().HasTable(&db.KingRecruitActivitySnapshot{}) {
		err = tx.First(&snapshot, "pool_id=?", cfg.ID).Error
	}
	if err == nil {
		if json.Unmarshal(snapshot.Payload, &source) != nil {
			return form, constUtil.ErrAdminConfiguration
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if cfg.ID != "recruit_3" {
			return form, constUtil.ErrAdminConfiguration
		}
		source.Pool = cfg
		if err = tx.First(&source.Rules, 1).Error; err != nil {
			return form, err
		}
	} else {
		return form, err
	}
	if err := validateKingRecruitActivityRules(source.Rules); err != nil {
		return form, err
	}
	form.Options.KingPoolSource = string(adminJSON(source))
	return form, nil
}

// Each issued activity has its own fixed source snapshot. The original global
// daily quota, luck, cooldown, guarantee and prior receipts retain their IDs.
func (s *GameService) adminPersistKingRecruitActivity(tx *gorm.DB, key string, form model.AdminActivityForm, enabled, exists bool) error {
	if !adminKingRecruitPoolID(key) || form.Options.KingPoolSource == "" || form.StartAt <= 0 || form.EndAt <= form.StartAt || form.ClaimEndAt < form.EndAt {
		return constUtil.ErrAdminInput
	}
	var source adminKingRecruitSource
	if json.Unmarshal([]byte(form.Options.KingPoolSource), &source) != nil {
		return constUtil.ErrAdminInput
	}
	if err := validateKingRecruitActivityRules(source.Rules); err != nil {
		return err
	}
	pool := source.Pool
	pool.ID = key
	pool.Name = form.Name
	pool.Source = form.Source
	pool.Enabled = enabled
	pool.StartAt = form.StartAt
	pool.EndAt = form.EndAt
	pool.RewardEndAt = form.ClaimEndAt
	rewards := []db.GameplayPoolReward{}
	for _, prize := range form.Prizes {
		if prize.ID < 1 || prize.ID > math.MaxInt32 || prize.Amount != 1 || prize.Stock != 0 {
			return constUtil.ErrAdminInput
		}
		rewards = append(rewards, db.GameplayPoolReward{PoolID: key, ID: int32(prize.ID), ResourceID: prize.ResourceID, Amount: 1, Weight: prize.Weight, Stock: 0, Guaranteed: prize.Guaranteed})
	}
	if err := s.validateKingRecruitActivityPool(pool, rewards); err != nil {
		return err
	}
	if exists {
		frozen, err := adminKingRecruitActivityFrozen(tx, key)
		if err != nil {
			return err
		}
		if frozen {
			return constUtil.ErrAdminConfigConflict
		}
		if err = tx.Save(&pool).Error; err != nil {
			return err
		}
		if err = tx.Where("pool_id=?", key).Delete(&db.GameplayPoolReward{}).Error; err != nil {
			return err
		}
	} else if err := tx.Create(&pool).Error; err != nil {
		return err
	}
	for i := range rewards {
		if err := tx.Create(&rewards[i]).Error; err != nil {
			return err
		}
	}
	source.Pool = pool
	snapshot := db.KingRecruitActivitySnapshot{PoolID: key, Payload: adminJSON(source)}
	return tx.Save(&snapshot).Error
}

func adminKingRecruitActivityFrozen(tx *gorm.DB, key string) (bool, error) {
	if !adminKingRecruitPoolID(key) {
		return false, nil
	}
	if tx.Migrator().HasTable(&db.KingRecruitActivitySnapshot{}) {
		var snapshot db.KingRecruitActivitySnapshot
		err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&snapshot, "pool_id=?", key).Error
		if err == nil {
			return snapshot.Frozen, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}
	}
	if key == "recruit_3" && tx.Migrator().HasTable(&db.KingRecruitState{}) {
		var count int64
		err := tx.Model(&db.KingRecruitState{}).Count(&count).Error
		return count > 0, err
	}
	return false, nil
}

func (s *GameService) bindKingRecruitActivityRules(tx *gorm.DB, key string) error {
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitRulesScopeKey{}, (*db.KingRecruitRules)(nil))
	if key == "recruit_3" {
		if !tx.Migrator().HasTable(&db.KingRecruitActivitySnapshot{}) {
			return nil
		}
		var count int64
		if err := tx.Model(&db.KingRecruitActivitySnapshot{}).Where("pool_id=?", key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
	}
	var snapshot db.KingRecruitActivitySnapshot
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&snapshot, "pool_id=?", key).Error; err != nil {
		return err
	}
	var source adminKingRecruitSource
	if json.Unmarshal(snapshot.Payload, &source) != nil || source.Pool.ID != key {
		return constUtil.ErrAdminConfiguration
	}
	if err := validateKingRecruitActivityRules(source.Rules); err != nil {
		return err
	}
	if !snapshot.Frozen {
		if err := tx.Model(&snapshot).Update("frozen", true).Error; err != nil {
			return err
		}
	}
	rules := source.Rules
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitRulesScopeKey{}, &rules)
	return nil
}

// A legacy permanent king pool has no historic operating window to invent.
// The realm takes over after its first explicit king schedule, including rows
// later stopped, expired or archived; an empty window then stays closed.
func (s *GameService) adminScheduledKingRecruitPool(tx *gorm.DB, now int64) (string, bool, error) {
	if !tx.Migrator().HasTable(&db.AdminActivityOccurrence{}) {
		return "recruit_3", false, nil
	}
	tx = tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameServiceContextKey, s))
	var count int64
	if err := tx.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND mode=?", s.OriginalServerID(), constUtil.AdminKingRecruitMode).Count(&count).Error; err != nil {
		return "", false, err
	}
	if count == 0 {
		return "recruit_3", false, nil
	}
	if key, err := adminCharacterActivityCurrentKey(tx, constUtil.AdminKingRecruitMode, now, true); err != nil || key != "" {
		return key, true, err
	}
	var rows []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND mode=? AND timing<>? AND enabled=? AND archived=? AND start_at<=? AND claim_end_at>?", s.OriginalServerID(), constUtil.AdminKingRecruitMode, "character-day", true, false, now, now).Order("start_at,id").Find(&rows).Error; err != nil {
		return "", true, err
	}
	key := ""
	owner := GameCalendarOwner(tx.Statement.Context)
	for _, row := range rows {
		if owner < 1 {
			return "", true, constUtil.ErrAdminPermission
		}
		allowed, err := adminAcceptActivityTemplate(tx, owner, row)
		if err != nil {
			return "", true, err
		}
		if !allowed {
			continue
		}
		if key != "" {
			return "", true, constUtil.ErrAdminConfiguration
		}
		key = row.ConsumerKey
	}
	return key, true, nil
}

// AdminKingRecruitResources returns only original, recruitable king candidates.
func (s *GameService) AdminKingRecruitResources(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminKingRecruitResourcesResponse, error) {
	if err := adminOperationUser(db.GetIns().WithContext(ctx), actor, false); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	all := s.AdminCatalogueFiltered(model.AdminItemsRequest{Category: "character", Page: 1, PageSize: 1 << 30})
	out := &model.AdminKingRecruitResourcesResponse{Items: []model.AdminKingRecruitResource{}}
	labels := map[int64]string{1: "SAO", 2: "ALO", 3: "GGO"}
	for _, item := range all.Items {
		hero := s.table("Hero", "id", item.ID)
		if hero == nil || hero["heroClass"] != "0" || (hero["quality"] != "20" && hero["quality"] != "23") {
			continue
		}
		space, _ := strconv.ParseInt(hero["space"], 10, 64)
		if labels[space] == "" {
			continue
		}
		out.Items = append(out.Items, model.AdminKingRecruitResource{AdminItemDTO: item, Space: space, SpaceLabel: labels[space]})
	}
	return out, nil
}

// kingRecruitOpenState 使用与原招募相同的角色、排期和奖池，驱动原主城入口。
// 原客户端会把缺失或关闭的heroRecuritKing入口等级设为99999。
func (s *GameService) kingRecruitOpenState(ctx context.Context, id int64, now time.Time) (*pb.OpenState, error) {
	result := &pb.OpenState{Type: proto.String(constUtil.KingRecruitNativeOpenState), IsOpen: proto.Int32(0)}
	if !s.P1Ready || !db.GetIns().Migrator().HasTable(&db.KingRecruitRules{}) {
		return result, nil
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, _, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		key, _, err := s.adminScheduledKingRecruitPool(tx, now.UnixMilli())
		if err != nil || key == "" {
			return err
		}
		var pool db.GameplayPool
		if err := tx.First(&pool, "id=?", key).Error; err != nil {
			return err
		}
		if !pool.Enabled || player.GetLevel() < pool.MinLevel || (pool.StartAt > 0 && now.UnixMilli() < pool.StartAt) || (pool.EndAt > 0 && now.UnixMilli() >= pool.EndAt) {
			return nil
		}
		var rewards []db.GameplayPoolReward
		if err := tx.Where("pool_id=?", key).Find(&rewards).Error; err != nil {
			return err
		}
		if err := s.validateKingRecruitActivityPool(pool, rewards); err != nil {
			return err
		}
		result.IsOpen = proto.Int32(1)
		return nil
	})
	return result, err
}
