package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminItemDetail interprets immutable source rows and never reads player state or mutates tables.
func (s *GameService) AdminItemDetail(ctx context.Context, req model.AdminItemDetailRequest) (*model.AdminItemDetailResponse, error) {
	catalogue := s.AdminCatalogueFiltered(model.AdminItemsRequest{IDs: []string{req.ID}, Page: 1, PageSize: 1})
	if len(catalogue.Items) != 1 {
		return nil, constUtil.ErrAdminInput
	}
	var (
		item = catalogue.Items[0]
		out  = &model.AdminItemDetailResponse{Item: item, Sections: []model.AdminItemDetailSection{}}
	)
	add := func(title string, fields []model.AdminItemDetailField) {
		if len(fields) > 0 {
			out.Sections = append(out.Sections, model.AdminItemDetailSection{Title: title, Fields: fields})
		}
	}
	basic := []model.AdminItemDetailField{{Label: "品质", Value: item.QualityLabel}}
	if item.Description != "" {
		basic = append(basic, model.AdminItemDetailField{Label: "简介", Value: item.Description})
	} else {
		basic = append(basic, model.AdminItemDetailField{Label: "简介", Value: "原目录未提供可读简介"})
	}
	types := []string{}
	for _, key := range item.ApplicableTypes {
		for _, option := range catalogue.Facets.ApplicableTypes {
			if option.Value == key {
				types = append(types, option.Label)
			}
		}
	}
	if len(types) > 0 {
		basic = append(basic, model.AdminItemDetailField{Label: "适用类型", Value: strings.Join(types, "、")})
	}
	add("基本信息", basic)
	if item.ID == constUtil.GGOCurrencyID || item.ID == constUtil.GGOOverclockID || strings.HasPrefix(item.ID, "item_970") && len(item.ID) == 9 && item.ID >= "item_9701" && item.ID <= "item_9706" {
		add("道具说明", []model.AdminItemDetailField{{Label: "来源", Value: "本服新增 GGO 道具"}})
		return out, nil
	}
	var (
		tableName string
		key       = "id"
	)
	switch item.Category {
	case "character", "character-exp":
		tableName = "Hero"
	case "equipment":
		tableName = "Equip"
	case "accessory":
		tableName = "Treasure"
	case "pet":
		tableName = "Pet"
	case "pet-equipment":
		tableName = "PetEquip"
	case "fragment":
		tableName = "Frag"
	case "cooking":
		tableName, key = "FishSkill", "FishItemID"
	case "gem":
		tableName, key = "MineSkill", "MineItemID"
	case "material":
		if strings.HasPrefix(item.ID, "FishItem_") {
			tableName, key = "FishSkill", "FishItemID"
		} else if strings.HasPrefix(item.ID, "MineItem_") {
			tableName, key = "MineSkill", "MineItemID"
		}
	case "item":
		tableName, key = "Item", "itemId"
	}
	row := s.adminDetailRow(tableName, key, item.ID)
	if row == nil {
		add("原始数值", []model.AdminItemDetailField{{Label: "数据情况", Value: "未恢复该道具的独立原数值表；不推算属性或效果"}})
		return out, nil
	}
	add("数据依据", []model.AdminItemDetailField{{Label: "来源", Value: "客户端 Table" + tableName + " 原表；展示原值，不含角色养成后的加成"}})
	if value := adminDetailNumber(row["quality"]); value != "" {
		add("原始资质", []model.AdminItemDetailField{{Label: "原表资质", Value: value}})
	}
	stats := []model.AdminItemDetailField{}
	growth := []model.AdminItemDetailField{}
	usage := []model.AdminItemDetailField{}
	appendNumbers := func(target *[]model.AdminItemDetailField, entries [][2]string) {
		for _, entry := range entries {
			if value := adminDetailNumber(row[entry[0]]); value != "" {
				*target = append(*target, model.AdminItemDetailField{Label: entry[1], Value: value})
			}
		}
	}
	switch tableName {
	case "Hero", "Equip":
		appendNumbers(&stats, [][2]string{{"hpBase", "基础生命"}, {"attBase", "基础攻击"}, {"pdefBase", "基础物理防御"}, {"mdefBase", "基础魔法防御"}})
		if tableName == "Hero" {
			appendNumbers(&stats, [][2]string{{"hit", "命中（原表值）"}, {"dodge", "闪避（原表值）"}, {"cri", "暴击（原表值）"}, {"antiCri", "抗暴（原表值）"}, {"criFactor", "暴击系数（原表值）"}, {"damageAdd", "伤害加成（原表值）"}, {"damageReduce", "伤害减免（原表值）"}})
			maxLevel := int64(0)
			for _, level := range s.adminDetailRows("HeroExp") {
				if level["quality"] == row["quality"] && adminCatalogueNumber(level["level"]) > maxLevel {
					maxLevel = adminCatalogueNumber(level["level"])
				}
			}
			if maxLevel > 0 {
				growth = append(growth, model.AdminItemDetailField{Label: "原经验表最高等级", Value: strconv.FormatInt(maxLevel, 10) + "（实际解锁仍受角色与功能条件约束）"})
			}
		} else {
			appendNumbers(&growth, [][2]string{{"attGrow", "每级攻击成长"}, {"hpGrow", "每级生命成长"}, {"pdefGrow", "每级物理防御成长"}, {"mdefGrow", "每级魔法防御成长"}})
			for i := 1; i <= 4; i++ {
				if value := row["hole"+strconv.Itoa(i)]; value != "" {
					label := map[string]string{"0": "未开放", "1": "红色", "2": "紫色", "3": "绿色", "4": "蓝色", "5": "黄色", "6": "白色"}[value]
					if label != "" {
						growth = append(growth, model.AdminItemDetailField{Label: fmt.Sprintf("宝石孔%d", i), Value: label})
					}
				}
			}
		}
	case "Treasure":
		appendNumbers(&stats, [][2]string{{"att", "基础攻击"}, {"hp", "基础生命"}, {"cri", "暴击（原表值）"}, {"perhit", "命中（原表值）"}, {"anticri", "抗暴（原表值）"}, {"perevade", "闪避（原表值）"}})
		appendNumbers(&growth, [][2]string{{"maxStrengthen", "最大强化等级"}, {"maxRefine", "最大精炼等级"}, {"needChipNum", "合成所需碎片"}})
	case "Pet":
		usage = append(usage, model.AdminItemDetailField{Label: "数值说明", Value: "原精灵表的基础生命、攻击、防御字段已标记废弃，不作为当前实际属性展示"})
		for _, talent := range s.adminDetailRows("PetTalent") {
			if talent["petTalentId"] == row["defaultTalent"] && talent["talentLevel"] == "0" {
				stats = append(stats, adminDetailProperties("初始天赋", talent["talent"])...)
				growth = append(growth, adminDetailProperties("天赋成长", talent["talentGrow"])...)
			}
		}
	case "PetEquip":
		stats = append(stats, adminDetailProperties("基础", row["prop"])...)
		usage = append(usage, model.AdminItemDetailField{Label: "所属精灵", Value: s.adminDetailResourceName(row["petId"])})
	case "Frag":
		usage = append(usage, model.AdminItemDetailField{Label: "合成结果", Value: s.adminDetailResourceName(row["target"])})
		appendNumbers(&usage, [][2]string{{"num", "合成所需数量"}, {"price", "原表出售价值（将魂或威名）"}})
	case "MineSkill":
		stats = append(stats, adminDetailProperties("固定属性", row["Value"])...)
		appendNumbers(&growth, [][2]string{{"Level", "宝石等级"}, {"LevelLimit", "解锁等级"}, {"MakingExp", "制作获得经验"}})
		if row["Material"] != "" {
			usage = append(usage, model.AdminItemDetailField{Label: "制作消耗", Value: s.adminDetailResources(row["Material"])})
		}
	case "FishSkill":
		appendNumbers(&growth, [][2]string{{"Quality", "料理品级"}, {"LevelLimit", "解锁等级"}, {"MakingExp", "制作获得经验"}})
		if row["Material"] != "" {
			usage = append(usage, model.AdminItemDetailField{Label: "制作消耗", Value: s.adminDetailResources(row["Material"])})
		}
		kind := row["ValueType"]
		if kind == "onetimeRecover" {
			if value := adminDetailNumber(row["Value"]); value != "" {
				usage = append(usage, model.AdminItemDetailField{Label: "一次恢复", Value: value + "%生命（原料理表）"})
			}
		} else if strings.HasPrefix(kind, "permanent") {
			labels := []string{}
			for _, attr := range strings.Split(row["Value"], ";") {
				if label := adminDetailPropertyLabel(attr); label != "" {
					labels = append(labels, label)
				}
			}
			usage = append(usage, model.AdminItemDetailField{Label: "永久提升属性", Value: strings.Join(labels, "、")}, model.AdminItemDetailField{Label: "原表数值", Value: "原料理表仅列影响属性，单份增幅及累计上限见当前生效配置"})

			// Reuse the serving formula with its persisted rule; no defaults or copied arithmetic.
			current, err := s.adminDetailCookingCurrent(ctx, row)
			if err != nil {
				return nil, err
			}
			add("当前生效数值", current)
		} else if kind == "extraCoin" || kind == "extraExp" || kind == "extraCoinExp" {
			parts := strings.Split(row["Value"], ";")
			if len(parts) == 2 && adminDetailNumber(parts[0]) != "" && adminDetailNumber(parts[1]) != "" {
				label := map[string]string{"extraCoin": "金币加成", "extraExp": "经验加成", "extraCoinExp": "金币与经验加成"}[kind]
				usage = append(usage, model.AdminItemDetailField{Label: label, Value: parts[1] + "% · 持续" + parts[0] + "分钟"})
			}
		}
	case "Item":
		appendNumbers(&usage, [][2]string{{"energy", "使用增加精力"}, {"stamina", "使用增加体力"}, {"coin", "使用增加金币"}, {"diamond", "使用增加钻石"}, {"exp", "经验或能量（原表值）"}})
		candidates := []model.AdminItemDetailField{}
		for _, use := range s.adminDetailRows("ItemUse") {
			if use["itemId"] != item.ID || use["result"] == "" {
				continue
			}
			label := fmt.Sprintf("候选%d", len(candidates)+1)
			if value := adminDetailNumber(use["odds"]); value != "" {
				if row["category"] == "5" {
					label += " · 选择序号 " + value
				} else {
					label += " · 原权重 " + value
				}
			}
			candidates = append(candidates, model.AdminItemDetailField{Label: label, Value: s.adminDetailResources(use["result"])})
		}
		add("使用可获得", candidates)
	}
	appendNumbers(&usage, [][2]string{{"selloutPrice", "原表出售价格"}, {"sellPrice", "原表出售价格"}, {"expBase", "原表提供经验"}})
	add("基础属性", stats)
	add("成长与制作", growth)
	add("用途与效果", usage)
	if len(stats)+len(growth)+len(usage) == 0 {
		add("原始数值", []model.AdminItemDetailField{{Label: "数据情况", Value: "原表未提供可明确解释的独立数值，不补造属性或效果"}})
	}
	return out, nil
}

// adminDetailRows uses the independent original catalogue snapshot, then existing gameplay tables.
func (s *GameService) adminDetailRows(table string) []map[string]string {
	if s.AdminCatalogueData != nil && len(s.AdminCatalogueData.SourceTables[table]) > 0 {
		return s.AdminCatalogueData.SourceTables[table]
	}
	return s.Tables[table]
}
func (s *GameService) adminDetailRow(table, key, id string) map[string]string {
	for _, row := range s.adminDetailRows(table) {
		if row[key] == id {
			return row
		}
	}
	return nil
}
func adminDetailNumber(raw string) string {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
func adminDetailPropertyLabel(key string) string {
	return map[string]string{"att": "攻击", "hp": "生命", "pdef": "物理防御", "mdef": "魔法防御", "cri": "暴击", "antiCri": "抗暴", "hit": "命中", "dodge": "闪避", "damageAdd": "伤害加成", "damageReduce": "伤害减免"}[key]
}
func adminDetailProperties(prefix, raw string) []model.AdminItemDetailField {
	out := []model.AdminItemDetailField{}
	for _, entry := range strings.Split(raw, ";") {
		parts := strings.Split(entry, ",")
		if len(parts) != 2 {
			continue
		}
		if label, value := adminDetailPropertyLabel(parts[0]), adminDetailNumber(parts[1]); label != "" && value != "" {
			out = append(out, model.AdminItemDetailField{Label: prefix + label, Value: value})
		}
	}
	return out
}
func (s *GameService) adminDetailResourceName(id string) string {
	result := s.AdminCatalogueFiltered(model.AdminItemsRequest{IDs: []string{id}, Page: 1, PageSize: 1})
	if len(result.Items) == 1 {
		return result.Items[0].Name
	}
	return "未恢复名称（" + id + "）"
}
func (s *GameService) adminDetailResources(raw string) string {
	parts := []string{}
	for _, entry := range strings.Split(raw, ";") {
		pair := strings.Split(entry, ",")
		if len(pair) == 2 && adminDetailNumber(pair[1]) != "" {
			if pair[0] == "perhp" {
				parts = append(parts, "恢复生命 "+adminDetailNumber(pair[1])+"%")
			} else {
				parts = append(parts, s.adminDetailResourceName(pair[0])+" × "+adminDetailNumber(pair[1]))
			}
		} else {
			parts = append(parts, "原配置尚不能明确解析")
		}
	}
	return strings.Join(parts, "；")
}

// adminDetailCookingCurrent keeps runtime configuration distinct from original-table facts.
func (s *GameService) adminDetailCookingCurrent(ctx context.Context, row map[string]string) ([]model.AdminItemDetailField, error) {
	var rule db.CookingRule
	err := db.GetIns().WithContext(ctx).Take(&rule, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (!rule.Enabled || strings.TrimSpace(rule.Source) == "") {
		return []model.AdminItemDetailField{{Label: "配置状态", Value: "未配置有效的永久料理规则"}}, nil
	}
	if err != nil {
		return nil, err
	}
	gains, limits, err := s.cookingPermanentPolicy(&rule, row)
	// Mirror the existing consumption validity boundary, without recalculating gains.
	for _, gain := range gains {
		if gain <= 0 || gain > 1000000 || math.IsNaN(gain) || math.IsInf(gain, 0) {
			err = constUtil.ErrAdminConfiguration
		}
	}
	if err != nil {
		return []model.AdminItemDetailField{{Label: "配置状态", Value: "当前永久料理规则无效，未展示增幅与上限"}}, nil
	}
	fields := []model.AdminItemDetailField{{Label: "数值依据", Value: "当前生效配置；与游戏食用时使用同一料理规则"}, {Label: "配置来源", Value: rule.Source}}
	for _, key := range []string{"att", "hp", "pdef", "mdef", "cri", "antiCri"} {
		gain, ok := gains[key]
		if !ok {
			continue
		}
		label := adminDetailPropertyLabel(key)
		fields = append(fields, model.AdminItemDetailField{Label: "每份" + label + "增幅", Value: strconv.FormatFloat(gain, 'f', -1, 64)})
		if limit, capped := limits[key]; capped {
			fields = append(fields, model.AdminItemDetailField{Label: label + "累计上限", Value: strconv.FormatFloat(limit, 'f', -1, 64)})
		} else {
			fields = append(fields, model.AdminItemDetailField{Label: label + "累计上限", Value: "当前配置未设置累计上限"})
		}
	}
	return fields, nil
}
