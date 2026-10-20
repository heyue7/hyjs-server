package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// adminGuildConfigDef 只列出当前公会源码真实读取的 GameConfigure 参数。
type adminGuildConfigDef struct {
	Key     string
	Group   string
	Type    string
	Min     float64
	Max     float64
	Integer bool
}

var adminGuildGroupOrder = []struct {
	Key   string
	Label string
}{
	{"basic", "基础"},
	{"donation", "捐赠"},
	{"building", "建筑与祈祷"},
	{"unionFight", "公会战"},
	{"shop", "商店"},
}

var adminGuildConfigDefs = []adminGuildConfigDef{
	{Key: "UnionOpenLevel", Group: "basic", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionApplyMaxNum", Group: "basic", Type: "int", Min: 1, Max: 1000, Integer: true},
	{Key: "UnionCreateCost", Group: "basic", Type: "string"},
	{Key: "UnionViceNum", Group: "basic", Type: "int", Min: 0, Max: 100, Integer: true},
	{Key: "UnionLoginActiveScore", Group: "basic", Type: "int", Min: 0, Max: math.MaxInt32, Integer: true},
	{Key: "UnionLoginScore", Group: "basic", Type: "int", Min: 0, Max: math.MaxInt32, Integer: true},
	{Key: "UnionGetStaminaDaily", Group: "basic", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionNameLengthLimit", Group: "basic", Type: "int", Min: 1, Max: 1024, Integer: true},
	{Key: "UnionDeclarationLengthLimit", Group: "basic", Type: "int", Min: 1, Max: 4096, Integer: true},
	{Key: "UnionNoticeLengthLimit", Group: "basic", Type: "int", Min: 1, Max: 4096, Integer: true},
	{Key: "UnionQuitInterval", Group: "basic", Type: "int", Min: 0, Max: 8760, Integer: true},
	{Key: "UnionUseBadgeScore", Group: "donation", Type: "int", Min: 0, Max: math.MaxInt32, Integer: true},
	{Key: "UnionMaxDailyDonateCoin", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionMaxDailyDonateBadge", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionMinDailyDonateCoin", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionMinDailyDonateBadge", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionDonateCoinContributVal", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionDonateBadgeContributVal", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionDonateCoinDonateVal", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionDonateBadgeDonateVal", Group: "donation", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionFirstContributeCoinScore", Group: "donation", Type: "int", Min: 0, Max: math.MaxInt32, Integer: true},
	{Key: "UnionPrayContributeCost", Group: "building", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionPrayDailyCount", Group: "building", Type: "int", Min: 1, Max: 100, Integer: true},
	{Key: "UnionPrayDuration", Group: "building", Type: "int", Min: 1, Max: 720, Integer: true},
	{Key: "UnionOpenHeroStatueCoinCost", Group: "building", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionOpenHeroStatueBadgeCost", Group: "building", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionOpenGuardTreeCoinCost", Group: "building", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionOpenGuardTreeBadgeCost", Group: "building", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "UnionFightResNum", Group: "unionFight", Type: "int", Min: 1, Max: 1000, Integer: true},
	{Key: "UnionFightDuration", Group: "unionFight", Type: "double", Min: 0.1, Max: 3600},
	{Key: "UnionFightReviveTime", Group: "unionFight", Type: "string"},
	{Key: "UnionFightPlayerMoveSpeed", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFightResConvertOfferFactor", Group: "unionFight", Type: "double", Min: 0, Max: 10000},
	{Key: "UnionFightKillPlayerRewardScore", Group: "unionFight", Type: "double", Min: 0, Max: 1000000000},
	{Key: "UnionFightArriveResourcePointDistance", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFightArriveOther", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFightArriveGuard", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFightResMinDistance", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFightNpcMinDistance", Group: "unionFight", Type: "double", Min: 0.1, Max: 100},
	{Key: "UnionFighterNum", Group: "unionFight", Type: "int", Min: 1, Max: 100, Integer: true},
	{Key: "UFMaxAdjustApplyTimes", Group: "unionFight", Type: "int", Min: 0, Max: 100, Integer: true},
	{Key: "UFMaxPlayerAjustTime", Group: "unionFight", Type: "int", Min: 0, Max: 100, Integer: true},
	{Key: "UFLeaderFocusLastTime", Group: "unionFight", Type: "double", Min: 0.1, Max: 86400},
	{Key: "UFLeaderFocusCDTime", Group: "unionFight", Type: "double", Min: 0.1, Max: 86400},
	{Key: "UnionFightKillPlayerRewardOffer", Group: "unionFight", Type: "double", Min: 0, Max: 1000000000},
	{Key: "UnionFightForceKickOut", Group: "unionFight", Type: "int", Min: 0, Max: 3600, Integer: true},
	{Key: "UnionFightRevivePointOccupySpeed", Group: "unionFight", Type: "double", Min: 0.01, Max: 100},
	{Key: "UnionFightPlayerUseNormalItemCD", Group: "unionFight", Type: "double", Min: 0.1, Max: 3600},
	{Key: "UnionFightPlayerUseDiamondItemCD", Group: "unionFight", Type: "double", Min: 0.1, Max: 3600},
	{Key: "UnionFightApplyLevelLimit", Group: "unionFight", Type: "int", Min: 1, Max: math.MaxInt32, Integer: true},
	{Key: "uFGroupConfig", Group: "unionFight", Type: "string"},
}

var adminGuildLevelColumns = []model.AdminGuildTableColumn{
	{Key: "Level", Label: "公会等级", ValueType: "int", Editable: false},
	{Key: "MaxMemberNum", Label: "公会人数上限", ValueType: "int", Editable: true},
	{Key: "ActiveScore", Label: "活跃度目标值", ValueType: "int", Editable: true},
	{Key: "CommonReward", Label: "成员获得奖励", ValueType: "string", Editable: true},
	{Key: "ViceReward", Label: "副会长获得奖励", ValueType: "string", Editable: true},
	{Key: "ChairmanReward", Label: "会长获得奖励", ValueType: "string", Editable: true},
	{Key: "UnionExpScore", Label: "升级所需公会积分", ValueType: "int", Editable: true},
	{Key: "UnionBadge", Label: "升级所需公会徽章", ValueType: "int", Editable: true},
	{Key: "UnionCoin", Label: "升级所需金币", ValueType: "int", Editable: true},
}

var adminGuildTechColumns = []model.AdminGuildTableColumn{
	{Key: "techLevel", Label: "科技等级", ValueType: "int", Editable: true},
	{Key: "type", Label: "属性类型", ValueType: "string", Editable: true},
	{Key: "prop", Label: "数值加成", ValueType: "string", Editable: true},
	{Key: "UnionBadge", Label: "徽章消耗", ValueType: "int", Editable: true},
	{Key: "UnionCoin", Label: "金币消耗", ValueType: "int", Editable: true},
	{Key: "name", Label: "名称", ValueType: "string", Editable: true},
	{Key: "icon", Label: "图标", ValueType: "string", Editable: true},
}

// adminGuildStored 是 singleton JSON 的业务覆盖，不直接改写原静态表。
var adminGuildSettingsReady atomic.Bool

type adminGuildStored struct {
	Fields      map[string]string            `json:"fields,omitempty"`
	UnionLevels []map[string]string          `json:"unionLevels,omitempty"`
	UnionTech   []map[string]string          `json:"unionTech,omitempty"`
	Policies    *model.AdminGuildPolicyState `json:"policies,omitempty"`
}

// InitializeAdminGuildSettings 只创建后台覆盖行，原公会开关注入仍由原初始化负责。
func (s *GameService) InitializeAdminGuildSettings(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.AdminGuildSetting{}); err != nil {
		return err
	}
	raw, err := json.Marshal(adminGuildStored{})
	if err != nil {
		return err
	}
	row := db.AdminGuildSetting{ID: 1, Settings: string(raw), Version: p6Digest(raw), UpdatedAt: time.Now().UnixMilli()}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	adminGuildSettingsReady.Store(true)
	return nil
}

func adminGuildDecodeStored(raw string) (adminGuildStored, error) {
	var stored adminGuildStored
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return stored, constUtil.ErrAdminConfiguration
	}
	return stored, nil
}

func adminGuildStoredVersion(stored adminGuildStored) (string, string, error) {
	raw, err := json.Marshal(stored)
	if err != nil {
		return "", "", err
	}
	return string(raw), p6Digest(raw), nil
}

func adminGuildDef(key string) *adminGuildConfigDef {
	for i := range adminGuildConfigDefs {
		if adminGuildConfigDefs[i].Key == key {
			return &adminGuildConfigDefs[i]
		}
	}
	return nil
}

func adminGuildCloneRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, len(rows))
	for i, row := range rows {
		copied := make(map[string]string, len(row))
		for key, value := range row {
			copied[key] = value
		}
		out[i] = copied
	}
	return out
}

func adminGuildFieldValues(s *GameService, stored adminGuildStored) map[string]string {
	values := make(map[string]string, len(adminGuildConfigDefs))
	for _, def := range adminGuildConfigDefs {
		values[def.Key] = s.table("GameConfigure", "StaticName", def.Key)["NameValue"]
	}
	for key, value := range stored.Fields {
		if adminGuildDef(key) != nil {
			values[key] = value
		}
	}
	return values
}

func adminGuildFieldGroups(s *GameService, stored adminGuildStored) []model.AdminGuildConfigGroup {
	values := adminGuildFieldValues(s, stored)
	groups := make([]model.AdminGuildConfigGroup, len(adminGuildGroupOrder))
	for i, group := range adminGuildGroupOrder {
		groups[i] = model.AdminGuildConfigGroup{Key: group.Key, Label: group.Label, Fields: []model.AdminGuildConfigField{}}
	}
	for _, def := range adminGuildConfigDefs {
		label := s.table("GameConfigure", "StaticName", def.Key)["Name"]
		if strings.TrimSpace(label) == "" {
			label = def.Key
		}
		for i := range groups {
			if groups[i].Key == def.Group {
				groups[i].Fields = append(groups[i].Fields, model.AdminGuildConfigField{Key: def.Key, Label: label, ValueType: def.Type, Value: values[def.Key], Editable: true, Min: def.Min, Max: def.Max})
			}
		}
	}
	return groups
}

func adminGuildEffectiveLevels(s *GameService, stored adminGuildStored) []map[string]string {
	if len(stored.UnionLevels) > 0 {
		return adminGuildCloneRows(stored.UnionLevels)
	}
	return adminGuildCloneRows(s.Tables["UnionConfig"])
}

func adminGuildEffectiveTech(s *GameService, stored adminGuildStored) []map[string]string {
	if len(stored.UnionTech) > 0 {
		return adminGuildCloneRows(stored.UnionTech)
	}
	return adminGuildCloneRows(s.Tables["UnionTech"])
}

func adminGuildParseNumber(raw, kind string, min, max float64, integer bool) (float64, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return 0, constUtil.ErrAdminInput
	}
	if integer {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || float64(value) < min || float64(value) > max {
			return 0, constUtil.ErrAdminInput
		}
		return float64(value), nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < min || value > max {
		return 0, constUtil.ErrAdminInput
	}
	return value, nil
}

func adminGuildValidateConfigField(def adminGuildConfigDef, value string) error {
	if def.Type == "string" {
		if value == "" || strings.TrimSpace(value) != value || len(value) > 2048 {
			return constUtil.ErrAdminInput
		}
		switch def.Key {
		case "UnionCreateCost":
			cost, err := parsePoolCost(value)
			if err != nil || cost == nil || cost.GetId() == "" || cost.GetNum() < 1 {
				return constUtil.ErrAdminInput
			}
		case "UnionFightReviveTime":
			parts := strings.Split(value, ",")
			if len(parts) < 1 || len(parts) > 64 {
				return constUtil.ErrAdminInput
			}
			for _, part := range parts {
				seconds, err := strconv.ParseInt(part, 10, 32)
				if err != nil || seconds < 1 || seconds > 86400 {
					return constUtil.ErrAdminInput
				}
			}
		case "uFGroupConfig":
			parts := strings.Split(value, ",")
			if len(parts) != 4 {
				return constUtil.ErrAdminInput
			}
			total := float64(0)
			for _, part := range parts {
				ratio, err := strconv.ParseFloat(part, 64)
				if err != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 || ratio > 100 {
					return constUtil.ErrAdminInput
				}
				total += ratio
			}
			if math.Abs(total-100) > 0.001 {
				return constUtil.ErrAdminInput
			}
		}
		return nil
	}
	_, err := adminGuildParseNumber(value, def.Type, def.Min, def.Max, def.Integer)
	return err
}

func adminGuildValidateFields(values map[string]string, levels []map[string]string) error {
	for _, def := range adminGuildConfigDefs {
		value, ok := values[def.Key]
		if !ok {
			return constUtil.ErrAdminInput
		}
		if err := adminGuildValidateConfigField(def, value); err != nil {
			return err
		}
	}
	if len(values) != len(adminGuildConfigDefs) {
		return constUtil.ErrAdminInput
	}
	minCoin, _ := strconv.ParseInt(values["UnionMinDailyDonateCoin"], 10, 64)
	maxCoin, _ := strconv.ParseInt(values["UnionMaxDailyDonateCoin"], 10, 64)
	if minCoin > maxCoin {
		return constUtil.ErrAdminInput
	}
	minBadge, _ := strconv.ParseInt(values["UnionMinDailyDonateBadge"], 10, 64)
	maxBadge, _ := strconv.ParseInt(values["UnionMaxDailyDonateBadge"], 10, 64)
	if minBadge > maxBadge {
		return constUtil.ErrAdminInput
	}
	first, err := strconv.ParseInt(values["UnionFighterNum"], 10, 32)
	if err != nil || first < 1 {
		return constUtil.ErrAdminInput
	}
	if len(levels) > 0 {
		last, err := strconv.ParseInt(levels[len(levels)-1]["MaxMemberNum"], 10, 32)
		if err != nil || first > last {
			return constUtil.ErrAdminInput
		}
	}
	return nil
}

func adminGuildValidateLevels(rows []map[string]string) error {
	if len(rows) < 1 || len(rows) > 20 {
		return constUtil.ErrAdminInput
	}
	keys := map[string]bool{}
	for _, column := range adminGuildLevelColumns {
		keys[column.Key] = true
	}
	previousCapacity := int64(0)
	for index, row := range rows {
		if len(row) != len(keys) {
			return constUtil.ErrAdminInput
		}
		for key := range row {
			if !keys[key] {
				return constUtil.ErrAdminInput
			}
		}
		level, err := strconv.ParseInt(row["Level"], 10, 32)
		if err != nil || level != int64(index+1) {
			return constUtil.ErrAdminInput
		}
		capacity, err := strconv.ParseInt(row["MaxMemberNum"], 10, 32)
		if err != nil || capacity < 1 || capacity < previousCapacity {
			return constUtil.ErrAdminInput
		}
		previousCapacity = capacity
		if _, err = strconv.ParseInt(row["ActiveScore"], 10, 32); err != nil {
			return constUtil.ErrAdminInput
		}
		for _, key := range []string{"CommonReward", "ViceReward", "ChairmanReward"} {
			if row[key] == "" {
				if index != len(rows)-1 {
					return constUtil.ErrAdminInput
				}
				continue
			}
			if _, err = p1RewardResources(row[key]); err != nil {
				return constUtil.ErrAdminInput
			}
		}
		upgradeCost := func(key string) (int64, error) {
			if row[key] == "" {
				if index == len(rows)-1 {
					return 0, nil
				}
				return 0, constUtil.ErrAdminInput
			}
			return strconv.ParseInt(row[key], 10, 32)
		}
		exp, err := upgradeCost("UnionExpScore")
		if err != nil || exp < -1 {
			return constUtil.ErrAdminInput
		}
		badge, err := upgradeCost("UnionBadge")
		if err != nil || badge < -1 {
			return constUtil.ErrAdminInput
		}
		coin, err := upgradeCost("UnionCoin")
		if err != nil || coin < -1 {
			return constUtil.ErrAdminInput
		}
		if index != len(rows)-1 && (exp < 0 || badge < 0 || coin < 0) {
			return constUtil.ErrAdminInput
		}
	}
	return nil
}

func adminGuildValidateTech(rows []map[string]string) error {
	if len(rows) < 8 || len(rows) > 256 {
		return constUtil.ErrAdminInput
	}
	keys := map[string]bool{}
	for _, column := range adminGuildTechColumns {
		keys[column.Key] = true
	}
	levels := map[string]int{}
	for _, row := range rows {
		if len(row) != len(keys) {
			return constUtil.ErrAdminInput
		}
		for key := range row {
			if !keys[key] {
				return constUtil.ErrAdminInput
			}
		}
		level, err := strconv.ParseInt(row["techLevel"], 10, 32)
		if err != nil || level < 1 {
			return constUtil.ErrAdminInput
		}
		kind := row["type"]
		if !p13GuildTechType(kind) {
			return constUtil.ErrAdminInput
		}
		expected := int64(levels[kind] + 1)
		if level != expected {
			return constUtil.ErrAdminInput
		}
		levels[kind] = int(level)
		parts := strings.Split(row["prop"], ",")
		if len(parts) != 2 || parts[0] != kind {
			return constUtil.ErrAdminInput
		}
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return constUtil.ErrAdminInput
		}
		if row["name"] == "" || len(row["name"]) > 64 || row["icon"] == "" || len(row["icon"]) > 64 {
			return constUtil.ErrAdminInput
		}
		for _, key := range []string{"UnionBadge", "UnionCoin"} {
			if row[key] == "" {
				if level != 1 {
					return constUtil.ErrAdminInput
				}
				continue
			}
			cost, err := strconv.ParseInt(row[key], 10, 64)
			if err != nil || cost < 0 {
				return constUtil.ErrAdminInput
			}
		}
	}
	if len(levels) != 8 {
		return constUtil.ErrAdminInput
	}
	return nil
}

func adminGuildTableDTO(columns []model.AdminGuildTableColumn, rows []map[string]string) model.AdminGuildTableDTO {
	if rows == nil {
		rows = []map[string]string{}
	}
	return model.AdminGuildTableDTO{Columns: columns, Rows: rows}
}

func adminGuildPolicyState(tx *gorm.DB) (model.AdminGuildPolicyState, []model.AdminGuildCapability, error) {
	var state model.AdminGuildPolicyState
	capabilities := []model.AdminGuildCapability{
		{Key: "guildGate", Label: "公会管理/经济/聊天门禁", Implemented: true, Editable: true, Detail: "独立控制公会管理、经济和聊天入口；关闭后只拒绝新请求。"},
		{Key: "prayerPolicy", Label: "祈祷池发布", Implemented: true, Editable: true, Detail: "保存后重新计算池摘要；旧祈祷记录继续保留原池快照。"},
		{Key: "techPolicy", Label: "科技目录发布", Implemented: true, Editable: true, Detail: "保存后重新计算目录摘要；旧科技报价继续保留原目录。"},
		{Key: "donateBoxPolicy", Label: "捐献宝箱", Implemented: true, Editable: true, Detail: "四档积分与奖励可配置；当天开始累计后沿用当日规则，新规则用于下一日。"},
		{Key: "donationBadgePolicy", Label: "金币捐献徽章", Implemented: true, Editable: true, Detail: "配置单次金币捐献获得的徽章与日限；已发出的奖励保持原数量。"},
		{Key: "unionFightEnrollment", Label: "公会战报名/匹配", Implemented: true, Editable: true},
		{Key: "unionFightChat", Label: "公会战文字/指挥", Implemented: true, Editable: true},
		{Key: "unionFightItem", Label: "公会战道具入口", Implemented: true, Editable: true},
		{Key: "unionFightField", Label: "公会战场景策略", Implemented: true, Editable: true, Detail: "只影响新场次；已冻结地图和奖励不回写。"},
		{Key: "unionFightPlayerBattle", Label: "普通玩家参战策略", Implemented: true, Editable: true, Detail: "只影响新匹配；开始时间需要晚于本次发布。"},
		{Key: "unionTechOverride", Label: "UnionTech 分级覆盖", Implemented: true, Editable: true, Detail: "修改用于新的科技报价；已发出的报价保持原成本。"},
	}
	read := func(table any, first bool, assign func(*gorm.DB) error) error {
		if !tx.Migrator().HasTable(table) {
			return nil
		}
		if first {
			return assign(tx)
		}
		return nil
	}
	if err := read(&db.P13GuildPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13GuildPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.Guild = model.AdminGuildGatePolicy{ManagementEnabled: row.ManagementEnabled, EconomyEnabled: row.EconomyEnabled, ChatEnabled: row.ChatEnabled, Source: row.Source}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13GuildPrayerPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13GuildPrayerPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.Prayer = model.AdminGuildPrayerPolicy{Enabled: row.Enabled, Version: row.Version, Source: row.Source, PoolJSON: row.PoolJSON, PoolHash: row.PoolHash, PublishedAt: row.PublishedAt}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13GuildTechPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13GuildTechPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.Tech = model.AdminGuildTechPolicy{Enabled: row.Enabled, Version: row.Version, Source: row.Source, DirectoryJSON: row.DirectoryJSON, DirectoryHash: row.DirectoryHash, PublishedAt: row.PublishedAt}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13GuildDonateBoxPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13GuildDonateBoxPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			tiers, err := p13GuildDonateBoxReadRules(row.RulesJSON)
			if err != nil {
				return err
			}
			state.DonateBox = model.AdminGuildDonateBoxPolicy{Enabled: row.Enabled, Version: row.Version, Source: row.Source, PublishedAt: row.PublishedAt, Tiers: tiers}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13GuildDonationBadgePolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13GuildDonationBadgePolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.DonationBadge = model.AdminGuildDonationBadgePolicy{Enabled: row.Enabled, Version: row.Version, Source: row.Source, CoinCost: row.CoinCost, BadgeAmount: row.BadgeAmount, DailyLimit: row.DailyLimit, PublishedAt: row.PublishedAt}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13UnionFightPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13UnionFightPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.UnionFight.Enrollment = model.AdminGuildUnionFightEnrollmentPolicy{EnrollmentEnabled: row.EnrollmentEnabled, MatchEnabled: row.MatchEnabled, Source: row.Source}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13UnionFightChatPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13UnionFightChatPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.UnionFight.Chat = model.AdminGuildUnionFightChatPolicy{TextEnabled: row.TextEnabled, TalkEnabled: row.TalkEnabled, TalkCooldownMillis: row.TalkCooldownMillis, Source: row.Source}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13UnionFightItemPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13UnionFightItemPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.UnionFight.Item = model.AdminGuildUnionFightItemPolicy{Enabled: row.Enabled, Source: row.Source}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if err := read(&db.P13UnionFightFieldPolicy{}, true, func(tx *gorm.DB) error {
		var row db.P13UnionFightFieldPolicy
		err := tx.First(&row, 1).Error
		if err == nil {
			state.UnionFight.Field = model.AdminGuildUnionFightFieldPolicy{Enabled: row.Enabled, PreparationMillis: row.PreparationMillis, LeftRevivePointID: row.LeftRevivePointID, RightRevivePointID: row.RightRevivePointID, InitialGuardHP: row.InitialGuardHP, InitialGateHP: row.InitialGateHP, MoveTolerance: row.MoveTolerance, Source: row.Source}
		}
		return err
	}); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return state, capabilities, err
	}
	if tx.Migrator().HasTable(&db.P13UnionFightPlayerBattlePolicy{}) {
		var rows []db.P13UnionFightPlayerBattlePolicy
		if err := tx.Order("id").Find(&rows).Error; err != nil {
			return state, capabilities, err
		}
		for _, row := range rows {
			state.UnionFight.PlayerBattle = append(state.UnionFight.PlayerBattle, model.AdminGuildUnionFightPlayerBattlePolicy{ID: row.ID, Enabled: row.Enabled, Version: row.Version, Source: row.Source, PublishedAt: row.PublishedAt, StartsAt: row.StartsAt, EndsAt: row.EndsAt, ReviveRule: row.ReviveRule})
		}
	}
	return state, capabilities, nil
}

func adminGuildPolicyLinks() []model.AdminGuildShopLink {
	return []model.AdminGuildShopLink{
		{Label: "公会商店", Path: "/shops/guild", Note: "双成本、等级与限购继续由已有公会商店后台保存。"},
		{Label: "公会战商店", Path: "/shops/union-fight", Note: "战功商品、库存与刷新继续由已有公会战商店后台保存。"},
	}
}

func adminGuildSettingDTO(s *GameService, row db.AdminGuildSetting, stored adminGuildStored, state model.AdminGuildPolicyState, capabilities []model.AdminGuildCapability) model.AdminGuildSettingDTO {
	return model.AdminGuildSettingDTO{
		Version:      row.Version,
		UpdatedAt:    row.UpdatedAt,
		UpdatedBy:    row.UpdatedBy,
		Groups:       adminGuildFieldGroups(s, stored),
		UnionLevels:  adminGuildTableDTO(adminGuildLevelColumns, adminGuildEffectiveLevels(s, stored)),
		UnionTech:    adminGuildTableDTO(adminGuildTechColumns, adminGuildEffectiveTech(s, stored)),
		Policies:     state,
		Shops:        adminGuildPolicyLinks(),
		Capabilities: capabilities,
	}
}

// AdminGuildSettings 返回有效全局覆盖；查看权限允许只读账号读取。
func (s *GameService) AdminGuildSettings(ctx context.Context, p *model.AdminPrincipal) (*model.AdminGuildSettingDTO, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, p, false); err != nil {
		return nil, err
	}
	var row db.AdminGuildSetting
	err := tx.First(&row, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = db.AdminGuildSetting{ID: 1, Settings: "{}", Version: p6Digest([]byte("{}"))}
		err = nil
	}
	if err != nil {
		return nil, err
	}
	stored, err := adminGuildDecodeStored(row.Settings)
	if err != nil {
		return nil, err
	}
	state, capabilities, err := adminGuildPolicyState(tx)
	if err != nil {
		return nil, err
	}
	out := adminGuildSettingDTO(s, row, stored, state, capabilities)
	return &out, nil
}

func adminGuildUpdatePolicy(tx *gorm.DB, s *GameService, save *model.AdminGuildPolicySave, now time.Time) error {
	if save == nil {
		return nil
	}
	if save.Guild != nil {
		if !tx.Migrator().HasTable(&db.P13GuildPolicy{}) {
			return constUtil.ErrAdminConfiguration
		}
		var row db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		row.ManagementEnabled = save.Guild.ManagementEnabled
		row.EconomyEnabled = save.Guild.EconomyEnabled
		row.ChatEnabled = save.Guild.ChatEnabled
		row.Source = constUtil.P13GuildSource
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	if save.Prayer != nil {
		if !tx.Migrator().HasTable(&db.P13GuildPrayerPolicy{}) {
			return constUtil.ErrAdminConfiguration
		}
		var row db.P13GuildPrayerPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		changed := row.PoolJSON != save.Prayer.PoolJSON || row.Version != save.Prayer.Version
		row.Enabled = save.Prayer.Enabled
		row.Version = save.Prayer.Version
		row.PoolJSON = save.Prayer.PoolJSON
		row.Source = constUtil.P13GuildPrayerSource
		row.PoolHash = p6Digest([]byte(row.PoolJSON))
		if row.Enabled {
			if strings.TrimSpace(row.Version) == "" || len(row.Version) > 128 {
				return constUtil.ErrAdminInput
			}
			if _, _, err := p13GuildPrayerPool(row.PoolJSON, row.PoolHash); err != nil {
				return constUtil.ErrAdminInput
			}
			if changed || row.PublishedAt <= 0 {
				row.PublishedAt = now.UnixMilli()
			}
		}
		if row.PublishedAt > now.UnixMilli() {
			return constUtil.ErrAdminInput
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	if save.Tech != nil {
		if !tx.Migrator().HasTable(&db.P13GuildTechPolicy{}) {
			return constUtil.ErrAdminConfiguration
		}
		var row db.P13GuildTechPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		changed := row.DirectoryJSON != save.Tech.DirectoryJSON || row.Version != save.Tech.Version
		row.Enabled = save.Tech.Enabled
		row.Version = save.Tech.Version
		row.DirectoryJSON = save.Tech.DirectoryJSON
		row.Source = constUtil.P13GuildTechSource
		row.DirectoryHash = p6Digest([]byte(row.DirectoryJSON))
		if row.Enabled {
			if strings.TrimSpace(row.Version) == "" || len(row.Version) > 128 {
				return constUtil.ErrAdminInput
			}
			if _, err := p13GuildTechDirectoryRead(row.DirectoryJSON, row.DirectoryHash); err != nil {
				return constUtil.ErrAdminInput
			}
			if changed || row.PublishedAt <= 0 {
				row.PublishedAt = now.UnixMilli()
			}
		}
		if row.PublishedAt > now.UnixMilli() {
			return constUtil.ErrAdminInput
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	if save.DonateBox != nil {
		if !tx.Migrator().HasTable(&db.P13GuildDonateBoxPolicy{}) {
			return constUtil.ErrAdminConfiguration
		}
		var row db.P13GuildDonateBoxPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		if save.DonateBox.Version != "" && save.DonateBox.Version != constUtil.P13GuildDonateBoxVersion {
			return constUtil.ErrAdminInput
		}
		if len(save.DonateBox.Tiers) > 0 {
			raw, err := json.Marshal(save.DonateBox.Tiers)
			if err != nil {
				return err
			}
			tiers, err := p13GuildDonateBoxReadRules(string(raw))
			if err != nil {
				return constUtil.ErrAdminInput
			}
			for _, tier := range tiers {
				ok, err := s.openServerRewardSupported(tier.Reward)
				if err != nil {
					return err
				}
				if !ok {
					return constUtil.ErrAdminInput
				}
			}
			row.RulesJSON = string(raw)
		}
		row.Enabled = save.DonateBox.Enabled
		row.Version = constUtil.P13GuildDonateBoxVersion
		row.Source = constUtil.P13GuildDonateBoxSource
		if row.Enabled {
			row.PublishedAt = now.UnixMilli()
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	if save.DonationBadge != nil {
		if !tx.Migrator().HasTable(&db.P13GuildDonationBadgePolicy{}) {
			return constUtil.ErrAdminConfiguration
		}
		var row db.P13GuildDonationBadgePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
			return err
		}
		if save.DonationBadge.Version != "" && save.DonationBadge.Version != constUtil.P13GuildDonationBadgeVersion {
			return constUtil.ErrAdminInput
		}
		if save.DonationBadge.CoinCost > 0 {
			row.CoinCost = save.DonationBadge.CoinCost
		}
		if save.DonationBadge.BadgeAmount > 0 {
			row.BadgeAmount = save.DonationBadge.BadgeAmount
		}
		if save.DonationBadge.DailyLimit > 0 {
			row.DailyLimit = save.DonationBadge.DailyLimit
		}
		if row.CoinCost < 1 || row.BadgeAmount < 1 || row.DailyLimit < 1 || row.DailyLimit > 1000 {
			return constUtil.ErrAdminInput
		}
		minimum, err := s.p10Setting("UnionMinDailyDonateCoin")
		if err != nil {
			return err
		}
		maximum, err := s.p10Setting("UnionMaxDailyDonateCoin")
		if err != nil {
			return err
		}
		if row.CoinCost != minimum || int64(row.CoinCost)*row.DailyLimit > int64(maximum) {
			return constUtil.ErrAdminInput
		}
		row.Enabled = save.DonationBadge.Enabled
		row.Version = constUtil.P13GuildDonationBadgeVersion
		row.Source = constUtil.P13GuildDonationBadgeSource
		if row.Enabled {
			row.PublishedAt = now.UnixMilli()
		}
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
	}
	if uf := save.UnionFight; uf != nil {
		if uf.Enrollment != nil {
			if !tx.Migrator().HasTable(&db.P13UnionFightPolicy{}) {
				return constUtil.ErrAdminConfiguration
			}
			var row db.P13UnionFightPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
				return err
			}
			row.EnrollmentEnabled = uf.Enrollment.EnrollmentEnabled
			row.MatchEnabled = uf.Enrollment.MatchEnabled
			row.Source = constUtil.P13UnionFightSource
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if uf.Chat != nil {
			if !tx.Migrator().HasTable(&db.P13UnionFightChatPolicy{}) {
				return constUtil.ErrAdminConfiguration
			}
			if uf.Chat.TalkCooldownMillis < 1000 || uf.Chat.TalkCooldownMillis > 3600000 {
				return constUtil.ErrAdminInput
			}
			var row db.P13UnionFightChatPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
				return err
			}
			row.TextEnabled = uf.Chat.TextEnabled
			row.TalkEnabled = uf.Chat.TalkEnabled
			row.TalkCooldownMillis = uf.Chat.TalkCooldownMillis
			row.Source = constUtil.P13UnionFightChatSource
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if uf.Item != nil {
			if !tx.Migrator().HasTable(&db.P13UnionFightItemPolicy{}) {
				return constUtil.ErrAdminConfiguration
			}
			var row db.P13UnionFightItemPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
				return err
			}
			row.Enabled = uf.Item.Enabled
			row.Source = constUtil.P13UnionFightItemSource
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if uf.Field != nil {
			if !tx.Migrator().HasTable(&db.P13UnionFightFieldPolicy{}) {
				return constUtil.ErrAdminConfiguration
			}
			if uf.Field.Enabled {
				if uf.Field.PreparationMillis <= 0 || uf.Field.PreparationMillis > 86400000 ||
					uf.Field.InitialGuardHP <= 0 || uf.Field.InitialGateHP <= 0 ||
					math.IsNaN(uf.Field.InitialGuardHP) || math.IsInf(uf.Field.InitialGuardHP, 0) ||
					math.IsNaN(uf.Field.InitialGateHP) || math.IsInf(uf.Field.InitialGateHP, 0) ||
					uf.Field.MoveTolerance < 0 || uf.Field.MoveTolerance > 50 ||
					math.IsNaN(uf.Field.MoveTolerance) || math.IsInf(uf.Field.MoveTolerance, 0) {
					return constUtil.ErrAdminInput
				}
				for side, id := range []int64{uf.Field.LeftRevivePointID, uf.Field.RightRevivePointID} {
					row := s.table("UnionFightRevivePoint", "UFRPId", strconv.FormatInt(id, 10))
					if row == nil || row["InitCamp"] != strconv.Itoa(side) || row["IsCanOccupy"] != "0" {
						return constUtil.ErrAdminInput
					}
				}
			}
			if strings.TrimSpace(uf.Field.Source) == "" || len(uf.Field.Source) > 1024 {
				return constUtil.ErrAdminInput
			}
			var row db.P13UnionFightFieldPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error; err != nil {
				return err
			}
			row.Enabled = uf.Field.Enabled
			row.PreparationMillis = uf.Field.PreparationMillis
			row.LeftRevivePointID = uf.Field.LeftRevivePointID
			row.RightRevivePointID = uf.Field.RightRevivePointID
			row.InitialGuardHP = uf.Field.InitialGuardHP
			row.InitialGateHP = uf.Field.InitialGateHP
			row.MoveTolerance = uf.Field.MoveTolerance
			row.Source = uf.Field.Source
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if len(uf.PlayerBattle) > 0 {
			if !tx.Migrator().HasTable(&db.P13UnionFightPlayerBattlePolicy{}) {
				return constUtil.ErrAdminConfiguration
			}
			seen := map[int64]bool{}
			for _, item := range uf.PlayerBattle {
				if seen[item.ID] || item.ID != 1 && item.ID != constUtil.P13UFPlayerPartialPolicyID {
					return constUtil.ErrAdminInput
				}
				seen[item.ID] = true
				var row db.P13UnionFightPlayerBattlePolicy
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, item.ID).Error; err != nil {
					return err
				}
				changed := row.Enabled != item.Enabled || row.Version != item.Version || row.Source != item.Source || row.StartsAt != item.StartsAt || row.EndsAt != item.EndsAt
				row.Enabled = item.Enabled
				row.Version = item.Version
				row.Source = item.Source
				row.StartsAt = item.StartsAt
				row.EndsAt = item.EndsAt
				row.ReviveRule = constUtil.P13UFPlayerBattleReviveRule
				if row.Enabled {
					if strings.TrimSpace(row.Version) == "" || strings.TrimSpace(row.Source) == "" || row.StartsAt <= 0 || row.EndsAt <= row.StartsAt {
						return constUtil.ErrAdminInput
					}
					if changed || row.PublishedAt <= 0 {
						row.PublishedAt = now.UnixMilli()
					}
					if row.StartsAt < row.PublishedAt {
						return constUtil.ErrAdminInput
					}
				}
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// AdminSaveGuildSettings 在 clockguard、版本和审计下提交全局覆盖，不写原静态表。
func (s *GameService) AdminSaveGuildSettings(ctx context.Context, p *model.AdminPrincipal, req model.AdminGuildSettingSaveRequest) (*model.AdminGuildSettingDTO, error) {
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.AdminGuildSetting
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, 1).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			row = db.AdminGuildSetting{ID: 1, Settings: "{}", Version: p6Digest([]byte("{}"))}
			err = nil
		}
		if err != nil {
			return err
		}
		if row.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		stored, err := adminGuildDecodeStored(row.Settings)
		if err != nil {
			return err
		}
		// 先形成完整白名单字段，再校验未知、重复和跨字段上下限。
		fields := adminGuildFieldValues(s, stored)
		for _, item := range req.Fields {
			if adminGuildDef(item.Key) == nil {
				return constUtil.ErrAdminInput
			}
			fields[item.Key] = item.Value
		}
		levels := adminGuildEffectiveLevels(s, stored)
		if len(req.UnionLevels) > 0 {
			levels = adminGuildCloneRows(req.UnionLevels)
		}
		tech := adminGuildEffectiveTech(s, stored)
		if len(req.UnionTech) > 0 {
			tech = adminGuildCloneRows(req.UnionTech)
		}
		if err := adminGuildValidateFields(fields, levels); err != nil {
			return err
		}
		if err := adminGuildValidateLevels(levels); err != nil {
			return err
		}
		if err := adminGuildValidateTech(tech); err != nil {
			return err
		}
		configTables := make(map[string][]map[string]string, len(s.Tables))
		for key, rows := range s.Tables {
			configTables[key] = rows
		}
		configTables["GameConfigure"] = adminGuildCloneRows(s.Tables["GameConfigure"])
		for _, r := range configTables["GameConfigure"] {
			if value, ok := fields[r["StaticName"]]; ok {
				r["NameValue"] = value
			}
		}
		if err := adminGuildUpdatePolicy(tx, s.withRuntimeTables(configTables), req.Policies, now); err != nil {
			return err
		}
		state, _, err := adminGuildPolicyState(tx)
		if err != nil {
			return err
		}
		stored.Fields = fields
		stored.UnionLevels = levels
		stored.UnionTech = tech
		stored.Policies = &state
		raw, version, err := adminGuildStoredVersion(stored)
		if err != nil {
			return err
		}
		row.Settings = raw
		row.Version = version
		row.UpdatedAt = now.UnixMilli()
		row.UpdatedBy = p.Username
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "guild.settings.update", "global", req.Reason+" | "+version)
	})
	if err != nil {
		return nil, err
	}
	return s.AdminGuildSettings(ctx, p)
}

// AdminGuildRuntimeTables 返回合并已提交覆盖后的请求级表副本，不修改共享 s.Tables。
func AdminGuildRuntimeTables(ctx context.Context, baseTables map[string][]map[string]string) (map[string][]map[string]string, error) {
	if baseTables == nil {
		return nil, nil
	}
	tx := db.GetIns().WithContext(ctx)
	// 正式初始化成功后不再逐请求检查表；隔离测试未初始化时才回退原表。
	if !adminGuildSettingsReady.Load() && !tx.Migrator().HasTable(&db.AdminGuildSetting{}) {
		return baseTables, nil
	}
	var row db.AdminGuildSetting
	if err := tx.First(&row, 1).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return baseTables, nil
		}
		// 同一进程切换隔离 SQLite 库时，未迁移的后备测试仍返回原表。
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "no such table") || strings.Contains(message, "1146") {
			return baseTables, nil
		}
		return nil, err
	}
	stored, err := adminGuildDecodeStored(row.Settings)
	if err != nil {
		return nil, err
	}
	if len(stored.Fields) == 0 && len(stored.UnionLevels) == 0 && len(stored.UnionTech) == 0 {
		return baseTables, nil
	}
	out := make(map[string][]map[string]string, len(baseTables)+1)
	for key, rows := range baseTables {
		out[key] = rows
	}
	if rows := baseTables["GameConfigure"]; rows != nil {
		index := map[string]int{}
		for i, row := range rows {
			index[row["StaticName"]] = i
		}
		changed := false
		for key, value := range stored.Fields {
			if i, ok := index[key]; ok && rows[i]["NameValue"] != value {
				changed = true
				break
			}
		}
		if changed {
			copied := adminGuildCloneRows(rows)
			for key, value := range stored.Fields {
				if i, ok := index[key]; ok {
					copied[i]["NameValue"] = value
				}
			}
			out["GameConfigure"] = copied
		}
	}
	// 与原表完全相同的分级表继续共享只读行，只有真正改值的表才深拷。
	if len(stored.UnionLevels) > 0 && !reflect.DeepEqual(stored.UnionLevels, baseTables["UnionConfig"]) {
		out["UnionConfig"] = adminGuildCloneRows(stored.UnionLevels)
	}
	if len(stored.UnionTech) > 0 && !reflect.DeepEqual(stored.UnionTech, baseTables["UnionTech"]) {
		out["UnionTech"] = adminGuildCloneRows(stored.UnionTech)
	}
	// 该标记仅由已提交全局设置生成，科技消费者据此区分运营覆盖与损坏的原表。
	out["AdminGuildSettings"] = []map[string]string{{"version": strconv.FormatInt(row.UpdatedAt, 10)}}
	return out, nil
}

type adminGuildOwned struct {
	guild    db.P13Guild
	members  []db.P13GuildMember
	profiles map[int64]db.Profile
	players  map[int64]*pb.Player
}

func adminGuildRowVersion(row db.P13Guild) string {
	raw, _ := json.Marshal(row)
	return p6Digest(raw)
}

func adminGuildVersion(row db.P13Guild, members []db.P13GuildMember) string {
	raw, _ := json.Marshal(struct {
		Guild   db.P13Guild
		Members []db.P13GuildMember
	}{row, members})
	return p6Digest(raw)
}

func adminGuildMemberVersion(row db.P13GuildMember) string {
	raw, _ := json.Marshal(row)
	return p6Digest(raw)
}

func adminGuildDecodePlayer(row db.Profile) *pb.Player {
	if row.Player == nil {
		return &pb.Player{}
	}
	var player pb.Player
	if json.Unmarshal([]byte(*row.Player), &player) != nil {
		return &pb.Player{}
	}
	return &player
}

// adminGuildLoadOwned 以成员 profile 联查真实区服归属，空归属和混区都在此拒绝。
func adminGuildLoadOwned(tx *gorm.DB, id string, selected int64, lockProfiles bool) (*adminGuildOwned, error) {
	if selected < 1 {
		return nil, constUtil.ErrAdminInput
	}
	var guild db.P13Guild
	if err := tx.First(&guild, "id=?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, constUtil.ErrAdminInput
		}
		return nil, err
	}
	var members []db.P13GuildMember
	if err := tx.Where("guild_id=?", id).Order("role DESC,player_id").Find(&members).Error; err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, fmt.Errorf("%w：公会成员归属未知", constUtil.ErrAdminInput)
	}
	chairmen := 0
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		if member.Role < constUtil.P13GuildMember || member.Role > constUtil.P13GuildChairman {
			return nil, fmt.Errorf("%w：公会成员职位无效", constUtil.ErrAdminInput)
		}
		if member.Role == constUtil.P13GuildChairman {
			chairmen++
		}
		ids = append(ids, member.PlayerID)
	}
	if chairmen != 1 {
		return nil, fmt.Errorf("%w：公会会长身份不唯一", constUtil.ErrAdminInput)
	}
	query := tx.Model(&db.Profile{}).Select("id", "server_id", "player").Where("id IN ?", ids).Order("id")
	if lockProfiles {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var profiles []db.Profile
	if err := query.Find(&profiles).Error; err != nil {
		return nil, err
	}
	if len(profiles) != len(ids) {
		return nil, fmt.Errorf("%w：公会成员角色归属未知", constUtil.ErrAdminInput)
	}
	out := &adminGuildOwned{guild: guild, members: members, profiles: map[int64]db.Profile{}, players: map[int64]*pb.Player{}}
	for _, profile := range profiles {
		if AdminProfileServerID(profile) != selected {
			return nil, constUtil.ErrAdminPermission
		}
		out.profiles[profile.Id] = profile
		out.players[profile.Id] = adminGuildDecodePlayer(profile)
	}
	return out, nil
}

func adminGuildMemberDTO(member db.P13GuildMember, player *pb.Player) model.AdminGuildMemberDTO {
	if player == nil {
		player = &pb.Player{}
	}
	return model.AdminGuildMemberDTO{
		PlayerID:              member.PlayerID,
		PlayerName:            player.GetName(),
		Level:                 player.GetLevel(),
		FightPower:            player.GetFightPower(),
		Role:                  member.Role,
		Contribution:          member.Contribution,
		SpendableContribution: player.GetContribution(),
		TodayCoin:             member.TodayCoin,
		TodayBadge:            member.TodayBadge,
		JoinedAt:              member.JoinedAt,
		QuitUntil:             member.QuitUntil,
		LastLoginAt:           member.LastLoginAt,
		DonateDay:             member.DonateDay,
		Version:               adminGuildMemberVersion(member),
	}
}

func adminGuildDetailFrom(s *GameService, tx *gorm.DB, owned *adminGuildOwned) (*model.AdminGuildDetail, error) {
	out := &model.AdminGuildDetail{
		ID:             owned.guild.ID,
		Name:           owned.guild.Name,
		Level:          owned.guild.Level,
		Notice:         owned.guild.Notice,
		Declaration:    owned.guild.Declaration,
		Flag:           owned.guild.Flag,
		Coin:           owned.guild.Coin,
		Badge:          owned.guild.Badge,
		Exp:            owned.guild.Exp,
		DonateScore:    owned.guild.DonateScore,
		ActiveScore:    owned.guild.ActiveScore,
		HeroStatueOpen: owned.guild.HeroStatueOpen,
		GuardTreeOpen:  owned.guild.GuardTreeOpen,
		CreatedAt:      owned.guild.CreatedAt,
		Version:        adminGuildVersion(owned.guild, owned.members),
		Members:        []model.AdminGuildMemberDTO{},
	}
	if tx.Migrator().HasTable(&db.P13UnionFightSetting{}) {
		var setting db.P13UnionFightSetting
		if err := tx.First(&setting, "guild_id=?", owned.guild.ID).Error; err == nil {
			out.UnionFightApply = setting.AutoApply
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	for _, member := range owned.members {
		out.Members = append(out.Members, adminGuildMemberDTO(member, owned.players[member.PlayerID]))
	}
	return out, nil
}

func adminGuildOwnershipSubquery(d *gorm.DB, selected int64) *gorm.DB {
	defaultServer := constUtil.GameDefaultServerID
	return d.Table("p13_guild_members AS m").
		Joins("JOIN profiles AS p ON p.id=m.player_id").
		Select("m.guild_id").
		Group("m.guild_id").
		Having("COUNT(DISTINCT CASE WHEN p.server_id IS NULL OR p.server_id=0 THEN ? ELSE p.server_id END)=1", defaultServer).
		Having("MAX(CASE WHEN p.server_id IS NULL OR p.server_id=0 THEN ? ELSE p.server_id END)=?", defaultServer, selected)
}

// AdminGuilds 只列出成员 profile 全部归属当前选中区服的公会。
func (s *GameService) AdminGuilds(ctx context.Context, q string, page, size int64) (*model.AdminGuildListResponse, error) {
	selected := AdminSelectedServerID(ctx)
	if selected < 1 {
		return nil, fmt.Errorf("%w：请先选择区服", constUtil.ErrAdminInput)
	}
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	d := db.GetIns().WithContext(ctx)
	out := &model.AdminGuildListResponse{Items: []model.AdminGuildSummary{}, Page: page, PageSize: size}
	if !d.Migrator().HasTable(&db.P13Guild{}) || !d.Migrator().HasTable(&db.P13GuildMember{}) {
		return out, nil
	}
	sub := adminGuildOwnershipSubquery(d, selected)
	query := d.Model(&db.P13Guild{}).Where("id IN (?)", sub)
	q = strings.TrimSpace(q)
	if q != "" {
		query = query.Where("(name LIKE ? OR id = ?)", "%"+q+"%", q)
	}
	if err := query.Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var guilds []db.P13Guild
	if err := query.Order("created_at,id").Limit(int(size)).Offset(int((page - 1) * size)).Find(&guilds).Error; err != nil {
		return nil, err
	}
	if len(guilds) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(guilds))
	for _, guild := range guilds {
		ids = append(ids, guild.ID)
	}
	var members []db.P13GuildMember
	if err := d.Where("guild_id IN ?", ids).Order("guild_id,role DESC,player_id").Find(&members).Error; err != nil {
		return nil, err
	}
	playerIDs := make([]int64, 0, len(members))
	for _, member := range members {
		playerIDs = append(playerIDs, member.PlayerID)
	}
	var profiles []db.Profile
	if len(playerIDs) > 0 {
		if err := d.Model(&db.Profile{}).Select("id", "server_id", "player").Where("id IN ?", playerIDs).Find(&profiles).Error; err != nil {
			return nil, err
		}
	}
	players := map[int64]*pb.Player{}
	for _, profile := range profiles {
		players[profile.Id] = adminGuildDecodePlayer(profile)
	}
	byGuild := map[string][]db.P13GuildMember{}
	for _, member := range members {
		byGuild[member.GuildID] = append(byGuild[member.GuildID], member)
	}
	for _, guild := range guilds {
		rows := byGuild[guild.ID]
		if len(rows) == 0 {
			continue
		}
		summary := model.AdminGuildSummary{ID: guild.ID, Name: guild.Name, Level: guild.Level, MemberCount: int64(len(rows)), Coin: guild.Coin, Badge: guild.Badge, Exp: guild.Exp, DonateScore: guild.DonateScore, ActiveScore: guild.ActiveScore, CreatedAt: guild.CreatedAt, Version: adminGuildVersion(guild, rows)}
		for _, member := range rows {
			if member.Role == constUtil.P13GuildChairman {
				if player := players[member.PlayerID]; player != nil {
					summary.ChairmanName = player.GetName()
				}
			}
		}
		out.Items = append(out.Items, summary)
	}
	return out, nil
}

// AdminGuildDetail 读取单会成员、数值和公会场设置，任何混区归属都拒绝。
func (s *GameService) AdminGuildDetail(ctx context.Context, id string) (*model.AdminGuildDetail, error) {
	selected := AdminSelectedServerID(ctx)
	if selected < 1 {
		return nil, fmt.Errorf("%w：请先选择区服", constUtil.ErrAdminInput)
	}
	d := db.GetIns().WithContext(ctx)
	if !d.Migrator().HasTable(&db.P13Guild{}) || !d.Migrator().HasTable(&db.P13GuildMember{}) {
		return nil, constUtil.ErrAdminConfiguration
	}
	owned, err := adminGuildLoadOwned(d, id, selected, false)
	if err != nil {
		return nil, err
	}
	return adminGuildDetailFrom(s, d, owned)
}

func adminGuildChangedFields(old, next db.P13Guild, reason string) string {
	raw, _ := json.Marshal(struct {
		Reason string
		Old    db.P13Guild
		New    db.P13Guild
	}{reason, old, next})
	return string(raw)
}

// AdminUpdateGuild 在公会门禁锁下编辑共享资产、等级、建筑开关、文本和旗帜。
func (s *GameService) AdminUpdateGuild(ctx context.Context, p *model.AdminPrincipal, id string, req model.AdminGuildUpdateRequest) (*model.AdminGuildDetail, error) {
	tables, runtimeErr := AdminGuildRuntimeTables(ctx, s.Tables)
	if runtimeErr != nil {
		return nil, runtimeErr
	}
	s = s.withRuntimeTables(tables)
	selected := AdminSelectedServerID(ctx)
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		owned, err := adminGuildLoadOwned(tx, id, selected, false)
		if err != nil {
			return err
		}
		if adminGuildVersion(owned.guild, owned.members) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Name != strings.TrimSpace(req.Name) || req.Name == "" || len(req.Name) > 64 {
			return constUtil.ErrAdminInput
		}
		if err := s.p13GuildText(req.Name, "UnionNameLengthLimit", false); err != nil {
			return constUtil.ErrAdminInput
		}
		if err := s.p13GuildText(req.Notice, "UnionNoticeLengthLimit", true); err != nil {
			return constUtil.ErrAdminInput
		}
		if err := s.p13GuildText(req.Declaration, "UnionDeclarationLengthLimit", true); err != nil {
			return constUtil.ErrAdminInput
		}
		if req.Level < 1 || req.Coin < 0 || req.Badge < 0 || req.Exp < 0 || req.DonateScore < 0 || req.ActiveScore < 0 || req.HeroStatueOpen < 0 || req.HeroStatueOpen > 1 || req.GuardTreeOpen < 0 || req.GuardTreeOpen > 1 {
			return constUtil.ErrAdminInput
		}
		capacity, err := s.p13GuildCapacity(req.Level)
		if err != nil || int64(capacity) < int64(len(owned.members)) {
			return constUtil.ErrAdminInput
		}
		if len(s.table("UnionLogo", "id", strconv.Itoa(int(req.Flag)))) == 0 {
			return constUtil.ErrAdminInput
		}
		if req.Name != owned.guild.Name {
			var count int64
			if err := tx.Model(&db.P13Guild{}).Where("name=? AND id<>?", req.Name, id).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return constUtil.ErrAdminInput
			}
		}
		var locked db.P13Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", id).Error; err != nil {
			return err
		}
		before := locked
		locked.Name = req.Name
		locked.Level = req.Level
		locked.Notice = req.Notice
		locked.Declaration = req.Declaration
		locked.Flag = req.Flag
		locked.Coin = req.Coin
		locked.Badge = req.Badge
		locked.Exp = req.Exp
		locked.DonateScore = req.DonateScore
		locked.ActiveScore = req.ActiveScore
		locked.HeroStatueOpen = req.HeroStatueOpen
		locked.GuardTreeOpen = req.GuardTreeOpen
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		return adminAudit(tx, p, "guild.update", id, adminGuildChangedFields(before, locked, req.Reason))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminGuildDetail(ctx, id)
}

func adminGuildLockProfiles(tx *gorm.DB, ids []int64) error {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var previous int64
	for _, id := range ids {
		if id == previous {
			continue
		}
		previous = id
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
	}
	return nil
}

func adminGuildMemberByPlayer(rows []db.P13GuildMember, id int64) *db.P13GuildMember {
	for i := range rows {
		if rows[i].PlayerID == id {
			return &rows[i]
		}
	}
	return nil
}

// AdminUpdateGuildMember 编辑实际贡献、当日捐献和职位；转让会长保持唯一会长。
func (s *GameService) AdminUpdateGuildMember(ctx context.Context, p *model.AdminPrincipal, guildID string, playerID int64, req model.AdminGuildMemberUpdateRequest) (*model.AdminGuildDetail, error) {
	tables, runtimeErr := AdminGuildRuntimeTables(ctx, s.Tables)
	if runtimeErr != nil {
		return nil, runtimeErr
	}
	s = s.withRuntimeTables(tables)
	selected := AdminSelectedServerID(ctx)
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		owned, err := adminGuildLoadOwned(tx, guildID, selected, false)
		if err != nil {
			return err
		}
		target := adminGuildMemberByPlayer(owned.members, playerID)
		if target == nil {
			return constUtil.ErrAdminInput
		}
		if adminGuildMemberVersion(*target) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Contribution < 0 || req.TodayCoin < 0 || req.TodayBadge < 0 || req.Role < constUtil.P13GuildMember || req.Role > constUtil.P13GuildChairman {
			return constUtil.ErrAdminInput
		}
		if target.Role == constUtil.P13GuildChairman && req.Role != constUtil.P13GuildChairman {
			return fmt.Errorf("%w：会长需先转让或解散，不能直接降为普通成员", constUtil.ErrAdminInput)
		}
		lockIDs := []int64{target.PlayerID}
		var chairman *db.P13GuildMember
		for i := range owned.members {
			if owned.members[i].Role == constUtil.P13GuildChairman {
				chairman = &owned.members[i]
			}
		}
		if chairman != nil && chairman.PlayerID != target.PlayerID {
			lockIDs = append(lockIDs, chairman.PlayerID)
		}
		if err := adminGuildLockProfiles(tx, lockIDs); err != nil {
			return err
		}
		var oldSpendable, newSpendable *int32
		if req.SpendableContribution != nil {
			// 锁定读取得最新 JSON，避免 MySQL RR 沿用成员读取时的旧快照。
			var profile db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, playerID).Error; err != nil {
				return err
			}
			player, _, err := decodeProfile(&profile)
			if err != nil {
				return err
			}
			actual := player.GetContribution()
			if req.ExpectedSpendableContribution == nil || actual != *req.ExpectedSpendableContribution {
				return constUtil.ErrAdminConfigConflict
			}
			// Patch this one JSON field so unknown fields and every other wallet stay intact.
			var raw map[string]json.RawMessage
			if profile.Player == nil || json.Unmarshal([]byte(*profile.Player), &raw) != nil || raw == nil {
				return constUtil.ErrAdminConfigConflict
			}
			raw["contribution"], err = json.Marshal(*req.SpendableContribution)
			if err != nil {
				return err
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				return err
			}
			if err := tx.Model(&profile).Update("player", string(encoded)).Error; err != nil {
				return err
			}
			oldSpendable, newSpendable = &actual, req.SpendableContribution
		}
		var locked db.P13Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", guildID).Error; err != nil {
			return err
		}
		if req.Role == constUtil.P13GuildVice && target.Role != constUtil.P13GuildVice {
			limit, err := s.p10Setting("UnionViceNum")
			if err != nil {
				return err
			}
			var count int64
			if err := tx.Model(&db.P13GuildMember{}).Where("guild_id=? AND role=?", guildID, constUtil.P13GuildVice).Count(&count).Error; err != nil {
				return err
			}
			if count >= int64(limit) {
				return fmt.Errorf("%w：原副会长名额已满", constUtil.ErrAdminInput)
			}
		}
		if req.Role == constUtil.P13GuildChairman && target.Role != constUtil.P13GuildChairman {
			if chairman == nil {
				return fmt.Errorf("%w：当前会长身份缺失", constUtil.ErrAdminInput)
			}
			if err := tx.Model(&db.P13GuildMember{}).Where("player_id=?", chairman.PlayerID).Update("role", constUtil.P13GuildMember).Error; err != nil {
				return err
			}
		}
		before := *target
		target.Contribution = req.Contribution
		target.TodayCoin = req.TodayCoin
		target.TodayBadge = req.TodayBadge
		target.DonateDay = p13GuildDay(now)
		target.Role = req.Role
		if err := tx.Save(target).Error; err != nil {
			return err
		}
		raw, _ := json.Marshal(struct {
			Reason                   string
			Old                      db.P13GuildMember
			New                      db.P13GuildMember
			OldSpendableContribution *int32 `json:"oldSpendableContribution,omitempty"`
			NewSpendableContribution *int32 `json:"newSpendableContribution,omitempty"`
		}{req.Reason, before, *target, oldSpendable, newSpendable})
		return adminAudit(tx, p, "guild.member.update", fmt.Sprintf("%s/%d", guildID, playerID), string(raw))
	})
	if err != nil {
		return nil, err
	}
	return s.AdminGuildDetail(ctx, guildID)
}

// AdminAddGuildMember 复用原资格与人数规则，并在同一事务刷新新成员祈祷投影。
func (s *GameService) AdminAddGuildMember(ctx context.Context, p *model.AdminPrincipal, guildID string, req model.AdminGuildMemberAddRequest) (*model.AdminGuildDetail, error) {
	tables, runtimeErr := AdminGuildRuntimeTables(ctx, s.Tables)
	if runtimeErr != nil {
		return nil, runtimeErr
	}
	s = s.withRuntimeTables(tables)
	selected := AdminSelectedServerID(ctx)
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		owned, err := adminGuildLoadOwned(tx, guildID, selected, false)
		if err != nil {
			return err
		}
		if adminGuildVersion(owned.guild, owned.members) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.PlayerID < 1 {
			return constUtil.ErrAdminInput
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, req.PlayerID).Error; err != nil {
			return err
		}
		if err := AdminCheckProfileScope(tx.Statement.Context, row); err != nil {
			return err
		}
		if AdminProfileServerID(row) != selected {
			return constUtil.ErrAdminPermission
		}
		player := adminGuildDecodePlayer(row)
		member, err := p13GuildMember(tx, req.PlayerID)
		if err != nil {
			return err
		}
		if err := s.p13GuildEligibility(player, member, now); err != nil {
			return err
		}
		capacity, err := s.p13GuildCapacity(owned.guild.Level)
		if err != nil || int64(capacity) <= int64(len(owned.members)) {
			return fmt.Errorf("%w：公会成员已满", constUtil.ErrAdminInput)
		}
		var locked db.P13Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", guildID).Error; err != nil {
			return err
		}
		member.PlayerID = req.PlayerID
		member.GuildID = guildID
		member.Role = constUtil.P13GuildMember
		member.JoinedAt = now.UnixMilli()
		member.QuitUntil = 0
		if err := tx.Save(member).Error; err != nil {
			return err
		}
		if err := tx.Where("player_id=?", req.PlayerID).Delete(&db.P13GuildApply{}).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err := s.RefreshP5GuildPrayerPower(ctx, tx, player, game, now); err != nil {
			return err
		}
		if err := saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		return adminAudit(tx, p, "guild.member.add", fmt.Sprintf("%s/%d", guildID, req.PlayerID), req.Reason)
	})
	if err != nil {
		return nil, err
	}
	return s.AdminGuildDetail(ctx, guildID)
}

// AdminRemoveGuildMember 复用原离会流程、24小时冷却和房间/战场撤权。
func (s *GameService) AdminRemoveGuildMember(ctx context.Context, p *model.AdminPrincipal, guildID string, playerID int64, req model.AdminGuildMemberRemoveRequest) (*model.AdminGuildDetail, error) {
	selected := AdminSelectedServerID(ctx)
	now := LocalPlayNow(ctx)
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		owned, err := adminGuildLoadOwned(tx, guildID, selected, false)
		if err != nil {
			return err
		}
		if adminGuildVersion(owned.guild, owned.members) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		target := adminGuildMemberByPlayer(owned.members, playerID)
		if target == nil {
			return constUtil.ErrAdminInput
		}
		if target.Role == constUtil.P13GuildChairman {
			return fmt.Errorf("%w：会长需先转让或解散，不能直接踢出", constUtil.ErrAdminInput)
		}
		if err := adminGuildLockProfiles(tx, []int64{playerID}); err != nil {
			return err
		}
		if err := s.p13GuildLeave(tx, target, now); err != nil {
			return err
		}
		return adminAudit(tx, p, "guild.member.remove", fmt.Sprintf("%s/%d", guildID, playerID), req.Reason)
	})
	if err != nil {
		return nil, err
	}
	s.AfterP13GuildRoomRevoke(ctx, []int64{playerID})
	s.AfterP13UnionFightFieldRevoke(ctx, []int64{playerID}, now)
	return s.AdminGuildDetail(ctx, guildID)
}

// AdminDissolveGuild 验证精确会名与版本，按ID升序锁全员后复用原离会和解散流程。
func (s *GameService) AdminDissolveGuild(ctx context.Context, p *model.AdminPrincipal, guildID string, req model.AdminGuildDissolveRequest) (*model.AdminGuildDissolveResponse, error) {
	selected := AdminSelectedServerID(ctx)
	now := LocalPlayNow(ctx)
	var revoked []int64
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if err := s.P13MonthlyBeforeGuildMutation(tx, now); err != nil {
			return err
		}
		owned, err := adminGuildLoadOwned(tx, guildID, selected, false)
		if err != nil {
			return err
		}
		if adminGuildVersion(owned.guild, owned.members) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.ConfirmName != owned.guild.Name {
			return constUtil.ErrAdminInput
		}
		ids := make([]int64, 0, len(owned.members))
		for _, member := range owned.members {
			ids = append(ids, member.PlayerID)
		}
		if err := adminGuildLockProfiles(tx, ids); err != nil {
			return err
		}
		var locked db.P13Guild
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", guildID).Error; err != nil {
			return err
		}
		for i := range owned.members {
			if err := s.p13GuildLeave(tx, &owned.members[i], now); err != nil {
				return err
			}
		}
		if err := tx.Where("guild_id=?", guildID).Delete(&db.P13GuildApply{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&locked).Error; err != nil {
			return err
		}
		revoked = ids
		return adminAudit(tx, p, "guild.dissolve", guildID, req.Reason+" | "+owned.guild.Name)
	})
	if err != nil {
		return nil, err
	}
	s.AfterP13GuildRoomRevoke(ctx, revoked)
	s.AfterP13UnionFightFieldRevoke(ctx, revoked, now)
	return &model.AdminGuildDissolveResponse{ID: guildID, Dissolved: true}, nil
}
