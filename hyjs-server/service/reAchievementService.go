package service

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"math"
	"strconv"
	"strings"
	"time"
)

// InitializeReAchievements 只插入Re现成的缺失ID，保留人工配置和角色冻结领取。
// 在InitializeP5和InitializeAdminGameContent之后执行。
func (s *GameService) InitializeReAchievements(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rules := s.reAchievementRules()
		added := int64(0)
		for _, rule := range rules {
			supported, err := s.openServerRewardSupported(rule.Reward)
			if err != nil {
				return err
			}
			if !supported {
				rule.Disabled = true
				rule.Source += "；原奖励资源缺失，保持关闭"
			}
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule)
			if result.Error != nil {
				return result.Error
			}
			added += result.RowsAffected
		}
		if added > 0 {
			return adminAudit(tx, &model.AdminPrincipal{Username: "system"}, "achievement.re1051.import", "re1051", fmt.Sprintf("新增%d条Re来源配置；原ID、人工配置、冻结领取保持", added))
		}
		return nil
	})
}
func (s *GameService) reAchievementRules() []db.P5Achievement {
	out := []db.P5Achievement{}
	add := func(id, name, kind string, typ, target int32, reward string) {
		out = append(out, db.P5Achievement{ID: id, Name: name, Kind: kind, Type: typ, Target: target, Reward: reward, Source: constUtil.ReAchievementSource})
	}
	add("ach_login", "累计登录1天", constUtil.ReAchievementLoginKind, constUtil.ReAchievementLoginType, 1, "item_002,10")
	add("ach_level", "账号达到20级", "level", 2, 20, "item_006,50")
	add("ach_hero", "图鉴收集5种角色", "collection", constUtil.ReAchievementCollectionType, 5, "item_010,30")
	add("ach_copy", "通关10个不同主线关卡", constUtil.ReAchievementStagesKind, constUtil.ReAchievementStagesType, 10, "diamond,300")
	for _, target := range []int32{5, 10, 30, 45, 60, 80, 100} {
		add("a306_level_"+strconv.Itoa(int(target)), fmt.Sprintf("账号达到%d级", target), "level", 2, target, fmt.Sprintf("diamond,%d", 100+target*10))
	}
	for _, target := range []int32{3, 7, 14, 30, 60} {
		add("a306_login_"+strconv.Itoa(int(target)), fmt.Sprintf("累计登录%d天", target), constUtil.ReAchievementLoginKind, constUtil.ReAchievementLoginType, target, fmt.Sprintf("item_002,%d", 5+target))
	}
	for _, target := range []int32{10, 20, 40, 60, 80} {
		add("a306_collection_"+strconv.Itoa(int(target)), fmt.Sprintf("图鉴收集%d种角色", target), "collection", constUtil.ReAchievementCollectionType, target, fmt.Sprintf("item_006,%d", target*5))
	}
	stages := int32(0)
	seen := map[string]bool{}
	for _, row := range s.Tables["CopyMission"] {
		id := row["MissionId"]
		if id != "" && row["ChapterId"] != "" && !seen[id] {
			stages++
			seen[id] = true
		}
	}
	for _, target := range []int32{30, 60, 100, 150, stages} {
		if target > 0 && target <= stages {
			add("a306_stage_"+strconv.Itoa(int(target)), fmt.Sprintf("通关%d个不同主线关卡", target), constUtil.ReAchievementStagesKind, constUtil.ReAchievementStagesType, target, fmt.Sprintf("diamond,%d", target*5))
		}
	}
	for _, target := range []int32{50, 150, 300, 600, stages * 5} {
		if target <= 0 || target > stages*5 {
			continue
		}
		add("a306_stars_"+strconv.Itoa(int(target)), fmt.Sprintf("主线累计获得%d颗星", target), "copy-stars-advanced", 5, target, fmt.Sprintf("item_005,%d", target))
	}
	add("a306_party_5", "上阵5名角色", constUtil.ReAchievementFormationKind, constUtil.ReAchievementFormationType, 5, "diamond,300")
	for i, hero := range []string{"hero_077", "hero_078", "hero_079"} {
		add("os310_"+hero, fmt.Sprintf("OS成长 · %s", []string{"OS桐人", "OS亚丝娜", "尤娜"}[i]), constUtil.ReAchievementGrowthKind, constUtil.ReAchievementGrowthType, 3, hero+",1")
		r := &out[len(out)-1]
		r.RequiredLoginDays = []int32{7, 14, 21}[i]
		r.RequiredLevel = []int32{40, 60, 80}[i]
		r.RequiredStages = []int32{30, 60, 100}[i]
	}
	return out
}

// p5AchievementLoginDays 只读取已提交的原登录日，重复重连/虚构source不增加。
func p5AchievementLoginDays(tx *gorm.DB, owner int64) (int32, error) {
	var facts []db.EventFact
	if err := tx.Select("day,source_key,amount").Where("player_id=? AND kind=? AND amount=? AND occurred_at<=?", owner, constUtil.P1EventLoginDay, 1, LocalPlayNow(tx.Statement.Context).UnixMilli()).Find(&facts).Error; err != nil {
		return 0, err
	}
	days := map[string]bool{}
	for _, fact := range facts {
		if fact.SourceKey != "login:"+fact.Day {
			continue
		}
		if _, err := time.Parse("2006-01-02", fact.Day); err == nil {
			days[fact.Day] = true
		}
	}
	return int32(min(int64(len(days)), int64(math.MaxInt32))), nil
}
func (s *GameService) p5AchievementCopyProgress(g *model.GameState) (int32, int32) {
	highest := map[string]int32{}
	for _, passed := range g.Copy.GetPassedCopyInfo() {
		row := s.table("CopyMission", "MissionId", passed.GetMissionId())
		if row == nil || row["ChapterId"] != passed.GetChapterId() || passed.GetStar() <= 0 {
			continue
		}
		key := passed.GetChapterId() + "/" + passed.GetMissionId()
		highest[key] = max(highest[key], min(int32(5), passed.GetStar()))
	}
	var stars int64
	for _, star := range highest {
		stars += int64(star)
	}
	return int32(min(int64(len(highest)), int64(math.MaxInt32))), int32(min(stars, int64(math.MaxInt32)))
}
func (s *GameService) p5ReGrowthValue(tx *gorm.DB, pid int64, level int32, g *model.GameState, r *db.P5Achievement) (int32, error) {
	if r.RequiredLoginDays < 1 || r.RequiredLevel < 1 || r.RequiredStages < 1 || r.Target != 3 {
		return 0, constUtil.ErrAdminConfiguration
	}
	days, err := p5AchievementLoginDays(tx, pid)
	if err != nil {
		return 0, err
	}
	stages, _ := s.p5AchievementCopyProgress(g)
	var value int32
	if days >= r.RequiredLoginDays {
		value++
	}
	if level >= r.RequiredLevel {
		value++
	}
	if stages >= r.RequiredStages {
		value++
	}
	return value, nil
}

// p5ReGrowthAlreadyAcquired 复用Re首次取得OS角色的规则，不把已有整卡再赠一次。
// 只影响原整卡奖励；运营改成其他奖励时沿普通冻结成就合同。
func p5ReGrowthAlreadyAcquired(tx *gorm.DB, owner int64, g *model.GameState, r *db.P5Achievement) (bool, error) {
	if r.Kind != constUtil.ReAchievementGrowthKind {
		return false, nil
	}
	reward := strings.Split(r.Reward, ",")
	if len(reward) != 2 || reward[1] != "1" || !strings.HasPrefix(reward[0], "hero_") {
		return false, nil
	}
	for _, hero := range g.Warriors {
		if hero.GetId() == reward[0] {
			return true, nil
		}
	}
	var count int64
	if err := tx.Model(&db.P5Collection{}).Where("player_id=? AND hero_id=?", owner, reward[0]).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// p5AchievementWireID carries immutable display requirements without changing the original PB schema.
func p5AchievementWireID(r *db.P5Achievement) string {
	if r.Kind != constUtil.ReAchievementGrowthKind {
		return r.ID
	}
	return fmt.Sprintf("%s~%d~%d~%d", r.ID, r.RequiredLoginDays, r.RequiredLevel, r.RequiredStages)
}

// p5AchievementCanonicalID validates display metadata against the player's accepted snapshot.
// Canonical request bytes keep original receipts compatible across plain/display IDs.
func p5AchievementCanonicalID(ctx context.Context, owner int64, wireID string) (string, error) {
	if !strings.Contains(wireID, "~") {
		if len(wireID) > 64 || wireID == "" {
			return "", p5Error()
		}
		return wireID, nil
	}
	fields := strings.Split(wireID, "~")
	if len(fields) != 4 || fields[0] == "" || len(fields[0]) > 64 {
		return "", p5Error()
	}
	var prior db.P5AchievementClaim
	if err := db.GetIns().WithContext(ctx).First(&prior, "player_id=? AND id=?", owner, fields[0]).Error; err != nil {
		return "", p5Error()
	}
	var frozen db.P5Achievement
	if json.Unmarshal([]byte(prior.Snapshot), &frozen) != nil || frozen.ID != fields[0] || frozen.Kind != constUtil.ReAchievementGrowthKind || frozen.RequiredLoginDays < 1 || frozen.RequiredLevel < 1 || frozen.RequiredStages < 1 || p5AchievementWireID(&frozen) != wireID {
		return "", p5Error()
	}
	return fields[0], nil
}
