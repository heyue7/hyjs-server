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
	World int64               `json:"world,omitempty"`
	Pool  db.GameplayPool     `json:"pool"`
	Rules db.KingRecruitRules `json:"rules"`
}
type kingRecruitRulesScopeKey struct{}
type kingRecruitWorldScopeKey struct{}
type kingRecruitEndScopeKey struct{}

func adminKingRecruitPoolID(key string) bool {
	return key == "recruit_3" || strings.HasPrefix(key, constUtil.AdminKingRecruitPrefix)
}
func validateKingRecruitActivityRules(r db.KingRecruitRules) error {
	if r.DailyLimit != 50 || r.LuckLimit != 500 || r.LuckPerDraw != 10 || r.RotationMillis != 172800000 || r.EpochMillis <= 0 {
		return constUtil.ErrAdminConfiguration
	}
	return nil
}
func (s *GameService) validateKingRecruitActivityPool(pool db.GameplayPool, rewards []db.GameplayPoolReward, worlds ...int64) error {
	if err := s.validateRecruitClientPrices(&pool, 3); err != nil {
		return err
	}
	if len(rewards) == 0 || len(rewards) > 1000 {
		return constUtil.ErrAdminInput
	}
	world := int64(0)
	if len(worlds) > 0 {
		world = worlds[0]
	}
	if world < 0 || world > 3 {
		return constUtil.ErrAdminInput
	}
	ids := map[int32]bool{}
	guaranteed := map[string]bool{}
	total := int64(0)
	for _, reward := range rewards {
		hero := s.table("Hero", "id", reward.ResourceID)
		if reward.ID < 1 || ids[reward.ID] || reward.Amount != 1 || reward.Stock != 0 || reward.Weight < 1 || reward.Weight > 1000000000000 || total > math.MaxInt64-reward.Weight || hero == nil || hero["heroClass"] != "0" || (hero["quality"] != "12" && hero["quality"] != "13" && hero["quality"] != "18" && hero["quality"] != "20" && hero["quality"] != "23") || (hero["quality"] == "23" && hero["space"] != "1" && hero["space"] != "2" && hero["space"] != "3") || reward.Guaranteed && hero["quality"] != "20" && hero["quality"] != "23" {
			return constUtil.ErrAdminInput
		}
		if world > 0 && hero["quality"] == "23" && hero["space"] != strconv.FormatInt(world, 10) {
			return constUtil.ErrAdminInput
		}
		ids[reward.ID] = true
		total += reward.Weight
		if reward.Guaranteed && hero["quality"] == "23" {
			guaranteed[hero["space"]] = true
		}
	}
	spaces := []string{"1", "2", "3"}
	if world > 0 {
		spaces = []string{strconv.FormatInt(world, 10)}
	}
	for _, space := range spaces {
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
	source.World = form.Options.KingWorld
	form.Options.KingPoolSource = string(adminJSON(source))
	return form, nil
}

func (s *GameService) adminKingRecruitActivityForm(tx *gorm.DB, cfg db.GameplayPool, rewards []db.GameplayPoolReward) (model.AdminActivityForm, error) {
	form := model.AdminActivityForm{Mode: constUtil.AdminKingRecruitMode, TemplateID: constUtil.AdminKingRecruitMode + "-original", Name: cfg.Name, Source: cfg.Source, StartAt: cfg.StartAt, EndAt: cfg.EndAt, ClaimEndAt: cfg.RewardEndAt, Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
	for _, r := range rewards {
		form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(r.ID), ResourceID: r.ResourceID, Amount: 1, Weight: r.Weight, Stock: 0, Guaranteed: r.Guaranteed})
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
	if err := s.validateKingRecruitActivityPool(cfg, rewards, source.World); err != nil {
		return form, err
	}
	form.Options.KingWorld = source.World
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
	if source.World != form.Options.KingWorld || source.World < 0 || source.World > 3 {
		return constUtil.ErrAdminInput
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
		rewards = append(rewards, db.GameplayPoolReward{PoolID: key, ID: int32(prize.ID), ResourceID: prize.ResourceID, Amount: 1, Weight: prize.Weight, Stock: 0, Guaranteed: s.table("Hero", "id", prize.ResourceID)["quality"] == "20" || s.table("Hero", "id", prize.ResourceID)["quality"] == "23"})
	}
	if err := s.validateKingRecruitActivityPool(pool, rewards, source.World); err != nil {
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
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitWorldScopeKey{}, int64(0))
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
	if source.World < 0 || source.World > 3 {
		return constUtil.ErrAdminConfiguration
	}
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitWorldScopeKey{}, source.World)
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitEndScopeKey{}, source.Pool.EndAt)
	rules := source.Rules
	tx.Statement.Context = context.WithValue(tx.Statement.Context, kingRecruitRulesScopeKey{}, &rules)
	return nil
}

// A legacy permanent king pool has no historic operating window to invent.
// The realm takes over after its first explicit king schedule, including rows
// later stopped, expired or archived; an empty window then stays closed.
func (s *GameService) adminScheduledKingRecruitPool(tx *gorm.DB, now int64) (string, bool, error) {
	candidates, scheduled, err := s.kingRecruitAvailablePools(tx, now)
	if err != nil || len(candidates) == 0 {
		return "", scheduled, err
	}
	owner := GameCalendarOwner(tx.Statement.Context)
	if owner > 0 && tx.Migrator().HasTable(&db.KingRecruitSelection{}) {
		var selected db.KingRecruitSelection
		err := tx.First(&selected, "player_id=?", owner).Error
		if err == nil {
			for _, candidate := range candidates {
				if candidate.Key == selected.PoolID {
					return candidate.Key, scheduled, nil
				}
			}
			// A closed selected world cannot silently spend in a different world.
			return "", scheduled, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", scheduled, err
		}
	}
	return candidates[0].Key, scheduled, nil
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
		if hero == nil || hero["heroClass"] != "0" || (hero["quality"] != "12" && hero["quality"] != "13" && hero["quality"] != "18" && hero["quality"] != "20" && hero["quality"] != "23") {
			continue
		}
		space, _ := strconv.ParseInt(hero["space"], 10, 64)
		if space == 0 && hero["quality"] != "23" {
			labels[0] = "通用"
		}
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
		available, _, err := s.kingRecruitAvailablePools(tx, now.UnixMilli())
		if err != nil || len(available) == 0 {
			return err
		}
		key := available[0].Key
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
		if err := s.bindKingRecruitActivityRules(tx, key); err != nil {
			return err
		}
		world, _ := tx.Statement.Context.Value(kingRecruitWorldScopeKey{}).(int64)
		if err := s.validateKingRecruitActivityPool(pool, rewards, world); err != nil {
			return err
		}
		result.IsOpen = proto.Int32(1)
		return nil
	})
	return result, err
}
