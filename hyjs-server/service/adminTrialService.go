package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func defaultAdminTrialGeneration() model.AdminTrialGeneration {
	return model.AdminTrialGeneration{Floors: constUtil.P7ReRushLastFloor,
		Anchors:           []model.AdminTrialAnchor{{Floor: 1, HP: 3500, Att: 220, Def: 50}, {Floor: 20, HP: 60000, Att: 4000, Def: 700}, {Floor: 40, HP: 450000, Att: 32000, Def: 7000}, {Floor: 60, HP: 1800000, Att: 125000, Def: 28000}, {Floor: 80, HP: 4800000, Att: 350000, Def: 85000}, {Floor: 90, HP: 7600000, Att: 560000, Def: 145000}, {Floor: 102, HP: 12000000, Att: 900000, Def: 240000}},
		DifficultyFactors: []float64{1, 1.25, 1.55}, Roster: []string{"monster_044", "monster_033", "hero_005", "hero_019", "hero_024"}, Positions: []int32{3, 7, 8, 9, 13}, InitialCount: 2, CountEveryFloors: 20, MaxCount: 5}
}

// ValidateAdminTrialGeneration 校验算法形状与原角色/技能资源；不接受无效数值或虚构阵容。
func (s *GameService) ValidateAdminTrialGeneration(cfg model.AdminTrialGeneration) error {
	valid := func(n float64) bool { return n > 0 && n <= math.MaxInt32 && !math.IsNaN(n) && !math.IsInf(n, 0) }
	if cfg.Floors != constUtil.P7ReRushLastFloor || len(cfg.Anchors) < 2 || len(cfg.Anchors) > int(cfg.Floors) || len(cfg.DifficultyFactors) != 3 || len(cfg.Roster) < 1 || len(cfg.Roster) > 5 || len(cfg.Positions) != len(cfg.Roster) || cfg.InitialCount < 1 || cfg.MaxCount < cfg.InitialCount || cfg.MaxCount > int32(len(cfg.Roster)) || cfg.CountEveryFloors < 1 || cfg.CountEveryFloors > cfg.Floors {
		return constUtil.ErrAdminInput
	}
	for i, a := range cfg.Anchors {
		if !valid(a.HP) || !valid(a.Att) || !valid(a.Def) || a.Floor < 1 || a.Floor > cfg.Floors || i > 0 && a.Floor <= cfg.Anchors[i-1].Floor {
			return constUtil.ErrAdminInput
		}
	}
	if cfg.Anchors[0].Floor != 1 || cfg.Anchors[len(cfg.Anchors)-1].Floor != cfg.Floors {
		return constUtil.ErrAdminInput
	}
	for i, f := range cfg.DifficultyFactors {
		if !valid(f) || i > 0 && f < cfg.DifficultyFactors[i-1] {
			return constUtil.ErrAdminInput
		}
	}
	seenID, seenPose := map[string]bool{}, map[int32]bool{}
	for i, id := range cfg.Roster {
		row := s.table("Hero", "id", id)
		if seenID[id] || row == nil || row["resName"] == "" || row["SkillId1"] == "" || s.table("Skill", "SkillId", row["SkillId1"]) == nil || cfg.Positions[i] < 1 || cfg.Positions[i] > 15 || seenPose[cfg.Positions[i]] {
			return constUtil.ErrAdminInput
		}
		seenID[id], seenPose[cfg.Positions[i]] = true, true
	}
	for _, a := range cfg.Anchors {
		for _, factor := range cfg.DifficultyFactors {
			if math.RoundToEven(a.HP*factor) < 1 || math.RoundToEven(a.Att*factor) < 1 || math.RoundToEven(a.Def*factor) < 1 || !valid(a.HP*factor*3) || !valid(a.Att*factor*1.25) || !valid(a.Def*factor*1.1) {
				return constUtil.ErrAdminInput
			}
		}
	}
	return nil
}

func adminTrialPolicyConfig(policy db.P7ReRushPolicy) (model.AdminTrialGeneration, error) {
	cfg := defaultAdminTrialGeneration()
	if policy.GenerationJSON != "" && json.Unmarshal([]byte(policy.GenerationJSON), &cfg) != nil {
		return cfg, constUtil.ErrAdminConfiguration
	}
	return cfg, nil
}
func (s *GameService) adminTrialResponse(tx *gorm.DB, policy db.P7ReRushPolicy) (*model.AdminTrialConfigResponse, error) {
	cfg, err := adminTrialPolicyConfig(policy)
	if err != nil {
		return nil, err
	}
	var floors int64
	if err = tx.Model(&db.P7RushStageRule{}).Where("kind=? AND enabled=?", constUtil.P7RushTrial, true).Distinct("floor").Count(&floors).Error; err != nil {
		return nil, err
	}
	source := policy.Source
	if !policy.Enabled {
		source = ""
		var sources []string
		if err = tx.Model(&db.P7RushStageRule{}).Where("kind=? AND enabled=?", constUtil.P7RushTrial, true).Distinct("source").Pluck("source", &sources).Error; err != nil {
			return nil, err
		}
		source = strings.Join(sources, "；")
	}
	return &model.AdminTrialConfigResponse{Enabled: policy.Enabled, Source: source, Version: policy.Version, PublishedAt: policy.PublishedAt, PublishedFloorCount: floors, Config: cfg}, nil
}

// AdminTrialConfig 读取当前发布来源及算法参数，空历史JSON采用现有Re默认值。
func (s *GameService) AdminTrialConfig(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminTrialConfigResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var policy db.P7ReRushPolicy
	if err := tx.First(&policy, 1).Error; err != nil {
		return nil, err
	}
	return s.adminTrialResponse(tx, policy)
}

// AdminTrialPreview 使用与发布相同的生成器预览，保持只读。
func (s *GameService) AdminTrialPreview(ctx context.Context, actor *model.AdminPrincipal, req model.AdminTrialPreviewRequest) (*model.AdminTrialPreviewResponse, error) {
	if err := adminOperationUser(db.GetIns().WithContext(ctx), actor, false); err != nil {
		return nil, err
	}
	if req.Floor < 0 || req.Floor > constUtil.P7ReRushLastFloor {
		return nil, constUtil.ErrAdminInput
	}
	if err := s.ValidateAdminTrialGeneration(req.Config); err != nil {
		return nil, err
	}
	rows, err := s.p7ReRushCatalogueConfigured(true, req.Config)
	if err != nil {
		return nil, err
	}
	out := &model.AdminTrialPreviewResponse{Items: []model.AdminTrialPreviewItem{}}
	names := s.adminTrialEnemyNames()
	for _, row := range rows {
		if row.Kind != constUtil.P7RushTrial || req.Floor > 0 && req.Floor != row.Floor {
			continue
		}
		monster := &pb.BattleMonster{}
		if err = proto.Unmarshal(row.Enemies, monster); err != nil {
			return nil, err
		}
		item := model.AdminTrialPreviewItem{Floor: row.Floor, Star: row.Star, RecommendedPower: row.RecommendedPower, Enemies: []model.AdminTrialEnemy{}}
		for _, hero := range monster.Formation {
			name := names[hero.GetId()]
			if name == "" {
				name = hero.GetId()
			}
			item.Enemies = append(item.Enemies, model.AdminTrialEnemy{ID: hero.GetId(), Name: name, Pose: hero.GetPose(), HP: hero.Prop.GetHp(), Att: hero.Prop.GetAtt(), Def: hero.Prop.GetPdef()})
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// AdminSaveTrialConfig 原子重新生成未来开战目录，保留现有Start/Rule和玩家进度、已选加成。
func (s *GameService) AdminSaveTrialConfig(ctx context.Context, actor *model.AdminPrincipal, req model.AdminTrialSaveRequest) (*model.AdminTrialConfigResponse, error) {
	if req.ExpectedVersion < 0 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		return nil, constUtil.ErrAdminInput
	}
	if err := s.ValidateAdminTrialGeneration(req.Config); err != nil {
		return nil, err
	}
	if err := adminOperationUser(db.GetIns().WithContext(ctx), actor, true); err != nil {
		return nil, err
	}
	rows, err := s.p7ReRushCatalogueConfigured(req.Enabled, req.Config)
	if err != nil {
		return nil, err
	}
	trialRows := make([]db.P7RushStageRule, 0, int(constUtil.P7ReRushLastFloor)*3)
	for _, row := range rows {
		if row.Kind == constUtil.P7RushTrial {
			trialRows = append(trialRows, row)
		}
	}
	rows = trialRows
	raw, err := json.Marshal(req.Config)
	if err != nil {
		return nil, err
	}
	var out *model.AdminTrialConfigResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var policy db.P7ReRushPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if policy.Version != req.ExpectedVersion {
			return constUtil.ErrAdminConfigConflict
		}
		before := policy
		// 原子替换普通试炼目录，精英目录保持独立运营状态。
		if err := tx.Model(&db.P7RushStageRule{}).Where("kind=? AND source<>?", constUtil.P7RushTrial, constUtil.P7ReRushSource).Update("enabled", false).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(&rows, 100).Error; err != nil {
			return err
		}
		policy.Enabled, policy.Source, policy.GenerationJSON, policy.Version, policy.PublishedAt = req.Enabled, constUtil.P7ReRushSource, string(raw), policy.Version+1, time.Now().UnixMilli()
		if err := tx.Save(&policy).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, constUtil.AdminTrialAuditAction, constUtil.AdminTrialAuditTarget, fmt.Sprintf("before=%s after=%s; %s", adminJSON(before), adminJSON(policy), strings.TrimSpace(req.Reason))); err != nil {
			return err
		}
		var err error
		out, err = s.adminTrialResponse(tx, policy)
		return err
	})
	return out, err
}

// adminTrialEnemyNames 复用后台目录的原文本与已解析名称，兼容未列入道具目录的monster资源。
func (s *GameService) adminTrialEnemyNames() map[string]string {
	texts := map[string]string{}
	for _, row := range s.adminDetailRows("Text") {
		if row["id"] != "" && row["text"] != "" {
			texts[row["id"]] = row["text"]
		}
	}
	if s.AdminCatalogueData != nil {
		for key, value := range s.AdminCatalogueData.DisplayNames {
			if texts[key] == "" {
				texts[key] = value
			}
		}
	}
	names := map[string]string{}
	for _, row := range s.adminDetailRows("Hero") {
		id := row["id"]
		names[id] = id
		for _, value := range []string{row["name"], row["Name"], row["nameIndex"], "name_" + id} {
			if value == "" {
				continue
			}
			if text := texts[value]; text != "" {
				names[id] = text
				break
			}
			if strings.HasPrefix(value, "name_") || strings.HasPrefix(value, "intro_") || strings.HasPrefix(value, "desc_") || strings.HasPrefix(value, "description_") {
				continue
			}
			names[id] = value
			break
		}
	}
	if s.AdminCatalogueData != nil {
		for _, item := range s.AdminCatalogueData.Items {
			if item.Name != "" && item.Name != item.ID {
				names[item.ID] = item.Name
			}
		}
	}
	return names
}
