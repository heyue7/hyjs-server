package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strconv"
	"strings"
)

// InitializeAdminGameContent adds no default prizes and preserves achievement claim snapshots.
func InitializeAdminGameContent(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.P5Achievement{}, &db.P5AchievementClaim{}, &db.AdminTitleOverride{})
}

// AdminAchievementKinds exposes every original GetItemConfig switch category, including unavailable scenarios.
func AdminAchievementKinds() []model.AdminAchievementKind {
	kinds := []model.AdminAchievementKind{
		{ID: "vip", Label: "VIP等级", Type: 1, Supported: true, Notes: "原文：会员等级到达X级。读取原Player.vip。"},
		{ID: "level", Label: "主角等级（一）", Type: 2, Supported: true, Notes: "原文：主角等级到达X级。读取原Player.level。"},
		{ID: "level-advanced", Label: "主角等级（二）", Type: 3, Supported: true, Notes: "原第二组文本和入口相同，读取Player.level；分段目标由运营配置。"},
		{ID: "copy-stars", Label: "副本总星数（一）", Type: 4, Supported: true, Notes: "原文：副本星数到达X级。按SCCopyPassedInfo实际主线通关节点的最高星数合计，同一节点不重复计数。"},
		{ID: "copy-stars-advanced", Label: "副本总星数（二）", Type: 5, Supported: true, Notes: "原第二组文本和入口与第一组相同；暂按同一主线累计星数，未取得原服两组额外范围差异证据。目标由运营填。"},
		{ID: "fight", Label: "阵容战斗力", Type: 6, Supported: true, Notes: "原文：战斗力到达X战力。读取原Player.fightPower。"},
		{ID: "equipped-heroes", Label: "穿齐4件装备的上阵人数", Type: 7, Supported: true, Notes: "原文：穿齐4件装备的上阵人数到达X人。核上阵真实角色及装备位置1..4和库存GUID。"},
		{ID: "treasure-heroes", Label: "穿齐2件宝物的上阵人数", Type: 8, Supported: true, Notes: "原文：穿齐2件宝物的上阵人数到达X人。核上阵真实角色及宝物位置5/6和库存GUID。"},
		{ID: "trial", Label: "试炼历史最高星数", Type: 9, Supported: true, Notes: "原文：试炼之地到达X星。读取原SCGetRushInfo.RecordTotalStar对应P7RushState.RecordStars，不使用层数。"},
		{ID: "formation-level", Label: "4名上阵角色等级", Type: 10, Supported: true, Notes: "原文：上阵4名角色的等级到达X级。要求至少4名真实上阵角色，取其等级最小值，表示4人均达到目标。"},
		{ID: "expedition-hours", Label: "累计远征出征小时", Type: 11, Supported: true, Notes: "原文：累计远征出征X小时。兼容聚合：按真实接受任务的已完成整小时累计，每任务封顶冻结计划时长，不乘伙伴数量；原服是否按伙伴乘算未取得证据。"},
		{ID: "wanted-kills", Label: "累计击败悬赏首领", Type: 12, Supported: true, Notes: "原文：累计击败悬赏首领X人。仅统计P12WantedInstance实际KilledAt>0且KillerID为本人，参与伤害不充当击杀。"},
		{ID: "formation-breach", Label: "4名上阵角色突破等级", Type: 13, Supported: true, Notes: "原文：上阵4名角色突破等级到达X级。要求4名真实上阵角色，读取各Warrior.Breached.level最小值。"},
		{ID: "friend-disaster-help", Label: "帮助好友解决厄运", Type: 14, Supported: true, Notes: "原文：帮助好友解决厄运事件X次。计实际P10ExpeditionHelpAction本人HelperID、他人OwnerID的唯一成功帮助回执，重试不重复。"},
		{ID: "pet-count", Label: "累计获得精灵", Type: 15, Supported: true, Notes: "原文：获得X只精灵。兼容历史聚合：真实在库、已回收、成长消耗捐献快照的GUID去重；同一GUID多个生命周期不重复，缺失旧历史不补造。"},
		{ID: "pet-elements", Label: "同种精灵收集元素数", Type: 16, Supported: true, Notes: "原文：同种类精灵收集齐X种元素。原TablePet无独立族ID，兼容按原resName同模型分组、nature元素去重，取最多元素的一组；原服族归类未取得服务端证据。"},
		{ID: "pet-level", Label: "精灵最高等级", Type: 17, Supported: true, Notes: "原文：精灵等级提升至X级。读取真实精灵及保留生命周期快照的Pet.level最大值。"},
		{ID: "pet-breach", Label: "精灵最高突破等级", Type: 18, Supported: true, Notes: "原GetItemConfig第18组对应achievement_pet_tal_count，原中文为精灵突破达到X级；读取Pet.Breached.level最大值。"},
		{ID: "pet-talents", Label: "精灵激活天赋技能数", Type: 19, Supported: true, Notes: "原GetItemConfig第19组对应achievement_pet_tupo，原中文为激活天赋技能达到X个；按原宠物战斗消费者中talentLevel<=突破级的已解锁独立天赋技能计数，取单只最大。"},
		{ID: "pet-intimacy", Label: "精灵最高亲密度星级", Type: 20, Supported: true, Notes: "原文：精灵亲密度达到X星。读取原Pet.Eat.level对应亲密星级最大值，未把经验数当成星级。"},
		{ID: "pet-recruit", Label: "精灵普通扭蛋次数", Type: 21, Supported: true, Notes: "原文：进行X次精灵扭蛋。读取P9PetRecruitProgress.Draws普通招募累计，不计独立引导首抽；原服首抽是否计数未取到服务端证据。"},
	}
	names := []string{"碧绿草原", "异域沙漠", "远古火山", "冰封雪域", "精灵之森"}
	for scene, name := range names {
		for difficulty := 1; difficulty <= 3; difficulty++ {
			supported := scene == 0 && difficulty == 1
			notes := fmt.Sprintf("原文：觉醒之地%s难度%d探索完成度达到100%%。", name, difficulty)
			if supported {
				notes += "读取实际非诊断501探索冻结数据及经过哈希核对的节点/击杀评分，原UI第1组跳转index=5，对应chapter5/copy501，组号与chapter反向。当前501场景4箱×5分+1怪×80分，全部实际节点完成为100分；这是已恢复场景评分映射。"
			} else {
				notes += "条件和原UI已确认；当前没有恢复此地图难度的真实探索消费者，禁止启用，不伪造完成进度。"
			}
			kinds = append(kinds, model.AdminAchievementKind{ID: fmt.Sprintf("awake-%d-%d", scene+1, difficulty), Label: fmt.Sprintf("%s·难度%d完整探索", name, difficulty), Type: int32(22 + scene*3 + difficulty - 1), Supported: supported, Notes: notes, FixedTarget: 100})
		}
	}
	kinds = append(kinds,
		model.AdminAchievementKind{ID: constUtil.ReAchievementLoginKind, Label: "累计登录日数", Type: constUtil.ReAchievementLoginType, Supported: true, Notes: "读取已提交的北京时间登录日，同日重连不重复。"},
		model.AdminAchievementKind{ID: "collection", Label: "累计图鉴角色种类", Type: constUtil.ReAchievementCollectionType, Supported: true, Notes: "按真实已获得的不同角色ID计算；回收不抹除已记录图鉴。"},
		model.AdminAchievementKind{ID: constUtil.ReAchievementStagesKind, Label: "不同主线通关数", Type: constUtil.ReAchievementStagesType, Supported: true, Notes: "仅计原主线表有效关卡且真实星数大于0，同一关不重复。"},
		model.AdminAchievementKind{ID: constUtil.ReAchievementFormationKind, Label: "上阵角色人数", Type: constUtil.ReAchievementFormationType, Supported: true, Notes: "读取真实上阵且在背包中的不同GUID，不计空位和重复GUID。"},
		model.AdminAchievementKind{ID: constUtil.ReAchievementGrowthKind, Label: "OS成长三项资格", Type: constUtil.ReAchievementGrowthType, Supported: true, FixedTarget: 3, Notes: "累计登录、角色等级和不同主线通关数三项同时达成；需分别配置三项门槛。"},
	)
	return kinds
}
func adminAchievementProjection(row db.P5Achievement) model.AdminAchievementDTO {
	out := model.AdminAchievementDTO{ID: row.ID, Name: row.Name, Kind: row.Kind, Type: row.Type, Target: row.Target, Rewards: adminTypedResources(row.Reward), Source: row.Source, Enabled: !row.Disabled, Version: adminVersion(row)}
	for _, kind := range AdminAchievementKinds() {
		if kind.ID == row.Kind {
			out.Supported = kind.Supported
			out.Notes = kind.Notes
		}
	}
	if row.Kind == constUtil.ReAchievementGrowthKind {
		out.Requirements = &model.AdminAchievementRequirements{LoginDays: row.RequiredLoginDays, Level: row.RequiredLevel, Stages: row.RequiredStages}
	}
	if row.Kind == "warrior_training" {
		out.Supported = true
		out.Notes = "旧历史兼容条件；只保留已有合同，不作为原36组新建选项。"
	}
	return out
}

// AdminAchievements lists current rules independently of their frozen player contracts.
func AdminAchievements(ctx context.Context, p *model.AdminPrincipal) (*model.AdminAchievementsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	var rows []db.P5Achievement
	if err := tx.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := &model.AdminAchievementsResponse{Items: []model.AdminAchievementDTO{}, Kinds: AdminAchievementKinds(), Notes: "支持原36组触发与已恢复成就；条件和奖励可分别配置。未恢复的探索场景继续禁用，旧领取快照与已领标记不改。高水位仅记录真实观察到的进度，不补造缺失历史。"}
	for _, r := range rows {
		out.Items = append(out.Items, adminAchievementProjection(r))
	}
	return out, nil
}

// AdminSaveAchievement writes explicit payouts, keeping IDs and all prior claim snapshots.
func (s *GameService) AdminSaveAchievement(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminAchievementDTO, create, archive bool) (*model.AdminAchievementDTO, error) {
	var out model.AdminAchievementDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		var row db.P5Achievement
		if create {
			row.ID = "admin_achievement_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
		} else {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", id).Error; err != nil {
				return err
			}
			if adminVersion(row) != req.Version {
				return constUtil.ErrAdminConfigConflict
			}
		}
		action := "achievement.update"
		if archive {
			row.Disabled = true
			action = "achievement.archive"
		} else {
			typeID := int32(0)
			for _, k := range AdminAchievementKinds() {
				if k.ID == req.Kind {
					typeID = k.Type
					if req.Enabled && !k.Supported {
						return constUtil.ErrAdminInput
					}
					if k.FixedTarget > 0 && req.Target != k.FixedTarget {
						return constUtil.ErrAdminInput
					}
				}
			}
			if typeID == 0 {
				return constUtil.ErrAdminInput
			}
			rewards, err := s.adminTypedResourceSpec(req.Rewards, !req.Enabled)
			if err != nil {
				return err
			}
			row.Name = req.Name
			row.Kind = req.Kind
			row.Type = typeID
			row.Target = req.Target
			if req.Kind == constUtil.ReAchievementGrowthKind {
				if req.Requirements != nil {
					row.RequiredLoginDays = req.Requirements.LoginDays
					row.RequiredLevel = req.Requirements.Level
					row.RequiredStages = req.Requirements.Stages
				}
				// 保存后必须仍是一套可执行的三项资格；旧编辑器漏字段时保留原条件。
				if row.RequiredLoginDays < 1 || row.RequiredLevel < 1 || row.RequiredStages < 1 {
					return constUtil.ErrAdminInput
				}
			} else {
				row.RequiredLoginDays, row.RequiredLevel, row.RequiredStages = 0, 0, 0
			}
			row.Reward = rewards
			row.Source = req.Source
			row.Disabled = !req.Enabled
			if create {
				action = "achievement.create"
			}
		}
		if create {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if err := adminAudit(tx, p, action, row.ID, "kind="+row.Kind+"; source="+row.Source); err != nil {
			return err
		}
		out = adminAchievementProjection(row)
		return nil
	})
	return &out, err
}
func adminTitleEffectList(value string) []model.AdminTitleEffect {
	out := []model.AdminTitleEffect{}
	for _, part := range strings.Split(value, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			continue
		}
		v, err := strconv.ParseInt(pair[1], 10, 64)
		if err == nil {
			out = append(out, model.AdminTitleEffect{Key: pair[0], Value: v})
		}
	}
	return out
}
func adminTitleProjection(original map[string]string, override *db.AdminTitleOverride) model.AdminTitleDTO {
	out := model.AdminTitleDTO{ID: original["titleId"], Name: original["titleName"], Type: int64(tableNumber(original, "type")), Param: original["param"], Effects: adminTitleEffectList(original["effects"]), EffectTime: int64(tableNumber(original, "effectTime")), Show: original["isNeedShow"] != "1", Enabled: true, Notes: "沿用原称号ID和资格类型；只影响新获得者，旧持有属性/有效期快照保持。"}
	if override != nil {
		out.Param = override.Param
		out.Effects = adminTitleEffectList(override.Effects)
		out.EffectTime = override.EffectTime
		out.Show = override.Show
		out.Enabled = override.Enabled
	}
	out.Notes = original["titleSimpDesc"] + "；" + out.Notes
	out.Version = adminVersion(struct {
		Original map[string]string
		Override *db.AdminTitleOverride
	}{original, override})
	if out.Type == 17 || out.Type == 18 {
		out.Notes += "公会赛季称号资格和奖励由已确认结算决定，仅允许开关和展示。"
	}
	return out
}

// adminTitlePresentation resolves original artwork and explains the current target without changing qualification.
func (s *GameService) adminTitlePresentation(value model.AdminTitleDTO, original map[string]string) model.AdminTitleDTO {
	if s.AdminCatalogueData != nil {
		value.IconURL = s.AdminCatalogueData.SpriteURLs[value.ID]
	}
	switch value.Type {
	case 1:
		value.Requirement = "同时上阵 " + value.Param + " 名角色"
	case 2, 3, 4:
		world := map[int64]string{2: "SAO", 3: "ALO", 4: "GGO"}[value.Type]
		value.Requirement = "收集全部 " + world + " 角色"
	case 5, 6, 7, 8:
		quality := map[int64]string{5: "蓝色", 6: "紫色", 7: "橙色", 8: "红色"}[value.Type]
		value.Requirement = "收集全部" + quality + "角色"
	case 9:
		value.Requirement = "角色等级达到 " + value.Param + " 级"
	case 10:
		value.Requirement = "通关主线第 " + value.Param + " 章"
	case 11:
		value.Requirement = "珠宝工艺达到 " + value.Param + " 级"
	case 12:
		value.Requirement = "料理工艺达到 " + value.Param + " 级"
	case 13:
		value.Requirement = "试炼之地通关第 " + value.Param + " 层"
	case 14:
		value.Requirement = "竞技场获胜并达到前 " + value.Param + " 名"
	default:
		value.Requirement = original["titleSimpDesc"]
	}
	if value.Requirement == "" {
		value.Requirement = "原客户端未注明获得条件"
	}
	value.Notes = value.Requirement
	return value
}

// AdminTitles returns only title identities actually present in the original resource table.
func (s *GameService) AdminTitles(ctx context.Context, p *model.AdminPrincipal) (*model.AdminTitlesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	var rows []db.AdminTitleOverride
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}
	values := map[string]db.AdminTitleOverride{}
	for _, r := range rows {
		values[r.ID] = r
	}
	out := &model.AdminTitlesResponse{Items: []model.AdminTitleDTO{}, Chapters: []model.AdminCatalogueOption{}}
	for _, chapter := range s.Tables["CopyChapter"] {
		name := chapter["ChapterName"]
		if s.AdminCatalogueData != nil && s.AdminCatalogueData.DisplayNames[name] != "" {
			name = s.AdminCatalogueData.DisplayNames[name]
		}
		out.Chapters = append(out.Chapters, model.AdminCatalogueOption{Value: chapter["ChapterId"], Label: "第 " + chapter["ChapterSequence"] + " 章 · " + name})
	}
	for _, r := range s.Tables["Title"] {
		if r["titleId"] == "" {
			continue
		}
		var override *db.AdminTitleOverride
		if v, ok := values[r["titleId"]]; ok {
			override = &v
		}
		out.Items = append(out.Items, s.adminTitlePresentation(adminTitleProjection(r, override), r))
	}
	return out, nil
}

// AdminSaveTitle stores an override instead of mutating a shared in-memory original table.
func (s *GameService) AdminSaveTitle(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminTitleDTO) (*model.AdminTitleDTO, error) {
	original := s.table("Title", "titleId", id)
	if original == nil {
		return nil, constUtil.ErrAdminInput
	}
	var out model.AdminTitleDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		// Serialize absent-row creation through the original administrator row; duplicate inserts never overwrite a concurrent writer.
		var actor db.AdminUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&actor, p.UserID).Error; err != nil {
			return err
		}
		var old db.AdminTitleOverride
		err := tx.First(&old, "id=?", id).Error
		var prior *db.AdminTitleOverride
		if err == nil {
			// Existing rows use a current locked read; absent rows rely on unique insert CAS without gap-lock upgrades.
			if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id=?", id).Error; err != nil {
				return err
			}
			prior = &old
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		current := adminTitleProjection(original, prior)
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		effects := []string{}
		for _, e := range req.Effects {
			effects = append(effects, fmt.Sprintf("%s,%d", e.Key, e.Value))
		}
		row := db.AdminTitleOverride{ID: id, Param: req.Param, Effects: strings.Join(effects, ";"), EffectTime: req.EffectTime, Show: req.Show, Enabled: req.Enabled}
		// Seasonal winner/title effects remain anchored to the accepted result table.
		if current.Type == 17 || current.Type == 18 {
			row.Param = current.Param
			row.Effects = strings.Join(func() []string {
				v := []string{}
				for _, e := range current.Effects {
					v = append(v, fmt.Sprintf("%s,%d", e.Key, e.Value))
				}
				return v
			}(), ";")
			row.EffectTime = current.EffectTime
		}
		switch current.Type {
		case 1, 9, 11, 12, 13, 14:
			value, err := strconv.ParseInt(row.Param, 10, 32)
			if err != nil || value < 1 || (current.Type == 11 && value > 10) || (current.Type == 12 && value > 6) {
				return constUtil.ErrAdminInput
			}
		case 2, 3, 4, 5, 6, 7, 8:
			row.Param = current.Param
		}
		if current.Type == 10 && s.table("CopyChapter", "ChapterId", row.Param) == nil {
			return constUtil.ErrAdminInput
		}
		if prior == nil {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
			err = result.Error
			if err == nil && result.RowsAffected != 1 {
				return constUtil.ErrAdminConfigConflict
			}
		} else {
			err = tx.Save(&row).Error
		}
		if err != nil {
			return err
		}
		if err = adminAudit(tx, p, "title.update", id, "future-grants-only"); err != nil {
			return err
		}
		out = adminTitleProjection(original, &row)
		return nil
	})
	if err == nil {
		out = s.adminTitlePresentation(out, original)
	}
	return &out, err
}
