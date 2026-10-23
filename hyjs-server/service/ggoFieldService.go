package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeGGOFieldConfiguration 迁移全部沙漠草案配置；只改草案模板，保留部署快照和非GGO字段。
func (s *GameService) InitializeGGOFieldConfiguration(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []db.AdminFieldTemplate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("scene = ?", "field_003").Order("id").Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			row := &rows[i]
			raw, changed, err := addGGOFieldConfiguration(row.Configuration)
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			// Revision递增使旧页面持有的版本失效，避免把迁移前内容再次保存覆盖。
			if err := tx.Model(&db.AdminFieldTemplate{}).Where("id = ?", row.ID).Updates(map[string]interface{}{"configuration": raw, "revision": row.Revision + 1, "updated_at": time.Now().UnixMilli()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// addGGOFieldConfiguration 补齐或迁移单个草案的GGO段，未知的非GGO JSON原样保留。
func addGGOFieldConfiguration(raw string) (string, bool, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	if cfg == nil {
		return raw, false, errors.New("沙漠配置为空")
	}
	var ggo *model.AdminFieldGGOConfiguration
	if value, ok := cfg["ggo"]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		ggo = &model.AdminFieldGGOConfiguration{}
		if err := json.Unmarshal(value, ggo); err != nil {
			return raw, false, err
		}
		if ggo.SchemaVersion >= constUtil.AdminFieldGGOSchemaVersion {
			// 小芽迁到全局设置；只移除此字段，保留当前沙漠规则及未知配置。
			var existing map[string]json.RawMessage
			if err := json.Unmarshal(value, &existing); err != nil {
				return raw, false, err
			}
			if _, exists := existing["sproutIcon"]; !exists {
				return raw, false, nil
			}
			delete(existing, "sproutIcon")
			updated, err := json.Marshal(existing)
			if err != nil {
				return raw, false, err
			}
			cfg["ggo"] = updated
			result, err := json.Marshal(cfg)
			return string(result), err == nil, err
		}
		migrateGGOConfiguration(ggo)
	} else {
		ggo = defaultGGOFieldConfiguration()
	}
	seed, err := json.Marshal(ggo)
	if err != nil {
		return raw, false, err
	}
	cfg["ggo"] = seed
	result, err := json.Marshal(cfg)
	return string(result), err == nil, err
}

// migrateGGOConfiguration 只修正旧已知默认职责、话术和测试初始刷新值，保留人工文案、坐标与数值。
func migrateGGOConfiguration(ggo *model.AdminFieldGGOConfiguration) {
	ggo.SchemaVersion = constUtil.AdminFieldGGOSchemaVersion
	ggo.Rules = defaultGGORules()
	roles := knownGGONPCRoles()
	seen := make(map[string]bool, len(ggo.NPCs))
	for i := range ggo.NPCs {
		npc := &ggo.NPCs[i]
		seen[npc.ID] = true
		spec, ok := roles[npc.ID]
		if !ok {
			continue
		}
		if npc.HeroID == "" {
			npc.HeroID = spec.heroID
		}
		// 仅替换旧方案的已知默认职责，人工改过的其他职责保留并在保存时按新政策拒绝。
		if npc.Role == spec.legacyRole {
			npc.Role = spec.role
		}
		if spec.resetsLegacySettlement && npc.SettlesChips {
			npc.SettlesChips = false
		}
		if spec.replacesOldDialogue && npc.Dialogue == spec.oldDialogue {
			npc.Dialogue = spec.dialogue
		}
	}
	if !seen["kohl"] {
		ggo.NPCs = append(ggo.NPCs, model.AdminFieldGGONPC{ID: "kohl", Name: "终端结算员·柯尔", HeroID: "clerk01_npc", Role: "settlement", PointLabel: "安全区终端", Dialogue: "已回到初始安全区。临时终端中的芯片将在这里完成结算。", SettlesChips: true})
	}
	for i := range ggo.Monsters {
		monster := &ggo.Monsters[i]
		heroID, ok := defaultGGOMonsterModels()[monster.ID]
		if !ok {
			continue
		}
		if monster.HeroID == "" {
			monster.HeroID = heroID
		}
		// 旧草案统一使用60000毫秒测试初值；仍按该初值保存的项迁移为30000毫秒，其他自定义值保留。
		if monster.RespawnMS == 60000 {
			monster.RespawnMS = 30000
		}
	}
	for i := range ggo.Bosses {
		boss := &ggo.Bosses[i]
		if heroID, ok := defaultGGOBossModels()[boss.ID]; ok && boss.HeroID == "" {
			boss.HeroID = heroID
		}
	}
	extraction := model.AdminFieldGGOExtraction{
		Arrival:        "已抵达撤离点，可返回初始安全区。",
		Confirmation:   "返回初始安全区？芯片结算请找终端结算员·柯尔。",
		ConfirmButton:  "返回安全区",
		ContinueButton: "继续探索",
		Countdown:      "撤离中，请留在区域内。剩余 {秒数} 秒。",
		Cancelled:      "你已离开撤离范围，本次撤离已取消。",
	}
	legacyExtraction := model.AdminFieldGGOExtraction{
		Arrival:        "已抵达接应点，可交付芯片并撤离。",
		Confirmation:   "交付本次携带的全部芯片，并离开沙域？",
		ConfirmButton:  "交付并撤离",
		ContinueButton: "继续探索",
		Countdown:      "接应中，请留在区域内。剩余 {秒数} 秒。",
		Cancelled:      "你已离开接应范围，本次撤离已取消。",
	}
	replaceGGODefault(&ggo.Extraction.Arrival, legacyExtraction.Arrival, extraction.Arrival)
	replaceGGODefault(&ggo.Extraction.Confirmation, legacyExtraction.Confirmation, extraction.Confirmation)
	replaceGGODefault(&ggo.Extraction.ConfirmButton, legacyExtraction.ConfirmButton, extraction.ConfirmButton)
	replaceGGODefault(&ggo.Extraction.ContinueButton, legacyExtraction.ContinueButton, extraction.ContinueButton)
	replaceGGODefault(&ggo.Extraction.Countdown, legacyExtraction.Countdown, extraction.Countdown)
	replaceGGODefault(&ggo.Extraction.Cancelled, legacyExtraction.Cancelled, extraction.Cancelled)
}

func replaceGGODefault(value *string, old, updated string) {
	if *value == old {
		*value = updated
	}
}

type ggoNPCRole struct {
	heroID                 string
	role                   string
	legacyRole             string
	resetsLegacySettlement bool
	oldDialogue            string
	dialogue               string
	replacesOldDialogue    bool
}

func knownGGONPCRoles() map[string]ggoNPCRole {
	return map[string]ggoNPCRole{
		"shino": {heroID: "hero_005", role: "teleport", legacyRole: "guide"},
		"dane": {
			heroID:                 "hero_029",
			role:                   "extraction",
			legacyRole:             "settlement",
			resetsLegacySettlement: true,
			oldDialogue:            "货带来了？交给我，剩下的路我来安排。",
			dialogue:               "我会把你送回初始安全区；芯片结算请找终端结算员·柯尔。",
			replacesOldDialogue:    true,
		},
		"kuroe": {
			heroID:                 "hero_073",
			role:                   "extraction",
			legacyRole:             "settlement",
			resetsLegacySettlement: true,
			oldDialogue:            "路口暂时没有动静。想撤的话，抓紧。",
			dialogue:               "撤离点已确认，我会送你回初始安全区；芯片结算请找终端结算员·柯尔。",
			replacesOldDialogue:    true,
		},
		"serra": {
			heroID:                 "hero_053",
			role:                   "extraction",
			legacyRole:             "settlement",
			resetsLegacySettlement: true,
			oldDialogue:            "接应已准备好。确认一下，你要全部交付吗？",
			dialogue:               "接应已准备好，可返回初始安全区；芯片结算请找终端结算员·柯尔。",
			replacesOldDialogue:    true,
		},
		"kohl": {heroID: "clerk01_npc", role: "settlement"},
	}
}

func defaultGGOMonsterModels() map[string]string {
	return map[string]string{"scavenger": "monster_026", "assault": "monster_029", "raider": "monster_031", "guard": "monster_032", "worm": "monster_003"}
}

func defaultGGOBossModels() map[string]string {
	return map[string]string{"behemoth": "hero_017", "pale_rider": "hero_016", "death_gun": "hero_006"}
}

func defaultGGORules() model.AdminFieldGGORules {
	return model.AdminFieldGGORules{
		EntryMethod:         "npc_random",
		ProtectionSeconds:   30,
		TerminalName:        "临时终端",
		UnlimitedCapacity:   true,
		InitialSource:       "monsters",
		PvpTransfer:         "winner_terminal",
		DeathReturn:         "safe_area",
		PveLossMode:         "pending",
		ExtractionReturn:    "safe_area",
		SettlementMode:      "safe_npc_or_confirm_leave",
		RetainAfterDeath:    true,
		SafeTradeEnabled:    true,
		TradeAssets:         "pending",
		CreditCap:           "none",
		ShopLimitScope:      "account_weekly",
		WeeklyResetTimezone: "Asia/Shanghai",
		WeeklyServerScope:   "pending",
		LeaveConfirmation:   "离开后将自动结算临时终端中的全部芯片，预计获得 {信用点} 信用点。是否离开？",
		LeaveConfirmButton:  "结算并离开",
		LeaveCancelButton:   "取消",
	}
}

func defaultGGOFieldConfiguration() *model.AdminFieldGGOConfiguration {
	return &model.AdminFieldGGOConfiguration{
		SchemaVersion: constUtil.AdminFieldGGOSchemaVersion,
		Title:         "GGO·残响沙域", ModeName: "遗迹回收行动", CurrencyID: "item_9710", ShopKey: "ggo", CarryIcon: "ggo_chip_marker",
		Rules: defaultGGORules(),
		NPCs: []model.AdminFieldGGONPC{
			{ID: "shino", Name: "行动向导·诗乃", HeroID: "hero_005", Role: "teleport", PointLabel: "入口", Dialogue: "先记住接应点的位置。拿到好东西以后，你会需要它。"},
			{ID: "dane", Name: "荒原接应人·戴恩", HeroID: "hero_029", Role: "extraction", PointLabel: "断桥接应点", Dialogue: "我会把你送回初始安全区；芯片结算请找终端结算员·柯尔。"},
			{ID: "kuroe", Name: "高地观察员·克洛维", HeroID: "hero_073", Role: "extraction", PointLabel: "岩脊接应点", Dialogue: "撤离点已确认，我会送你回初始安全区；芯片结算请找终端结算员·柯尔。"},
			{ID: "serra", Name: "前哨联络员·塞拉", HeroID: "hero_053", Role: "extraction", PointLabel: "遗迹接应点", Dialogue: "接应已准备好，可返回初始安全区；芯片结算请找终端结算员·柯尔。"},
			{ID: "kohl", Name: "终端结算员·柯尔", HeroID: "clerk01_npc", Role: "settlement", PointLabel: "安全区终端", Dialogue: "已回到初始安全区。临时终端中的芯片将在这里完成结算。", SettlesChips: true},
		},
		Monsters: []model.AdminFieldGGOEnemy{
			{ID: "scavenger", Name: "荒原拾荒枪手", HeroID: "monster_026", RespawnMS: 30000},
			{ID: "assault", Name: "遗迹突击兵", HeroID: "monster_029", RespawnMS: 30000},
			{ID: "raider", Name: "沙巾掠夺者", HeroID: "monster_031", RespawnMS: 30000},
			{ID: "guard", Name: "旧营地守卫", HeroID: "monster_032", RespawnMS: 30000},
			{ID: "worm", Name: "晶壳沙虫", HeroID: "monster_003", RespawnMS: 30000},
		},
		Bosses: []model.AdminFieldGGOEnemy{
			{ID: "behemoth", Name: "重火力霸主·猛兽", HeroID: "hero_017", RespawnMS: 3600000},
			{ID: "pale_rider", Name: "疾行猎手·苍白骑士", HeroID: "hero_016", RespawnMS: 3600000},
			{ID: "death_gun", Name: "幽灵追猎者·死枪", HeroID: "hero_006", RespawnMS: 3600000},
		},
		Chips:      []model.AdminFieldGGOChip{{ItemID: "item_9701", Name: "残损芯片"}, {ItemID: "item_9702", Name: "标准芯片"}, {ItemID: "item_9703", Name: "军用芯片"}, {ItemID: "item_9704", Name: "机密芯片"}, {ItemID: "item_9705", Name: "核心芯片"}, {ItemID: "item_9706", Name: "原型芯片"}},
		Extraction: model.AdminFieldGGOExtraction{Arrival: "已抵达撤离点，可返回初始安全区。", Confirmation: "返回初始安全区？芯片结算请找终端结算员·柯尔。", ConfirmButton: "返回安全区", ContinueButton: "继续探索", Countdown: "撤离中，请留在区域内。剩余 {秒数} 秒。", Cancelled: "你已离开撤离范围，本次撤离已取消。"},
	}
}
