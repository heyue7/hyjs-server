package service

import (
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	"time"
)

// InitializeGGOFieldConfiguration 只补齐原沙漠方案缺失的设计配置，保留用户编辑与部署快照。
func (s *GameService) InitializeGGOFieldConfiguration(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row db.AdminFieldTemplate
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id = ?", "original-field_003").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		raw, changed, err := addGGOFieldConfiguration(row.Configuration)
		if err != nil || !changed {
			return err
		}
		return tx.Model(&row).Updates(map[string]interface{}{"configuration": raw, "revision": row.Revision + 1, "updated_at": time.Now().UnixMilli()}).Error
	})
}
func addGGOFieldConfiguration(raw string) (string, bool, error) {
	var cfg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw, false, err
	}
	if cfg == nil {
		return raw, false, errors.New("沙漠配置为空")
	}
	if value, ok := cfg["ggo"]; ok && string(value) != "null" {
		return raw, false, nil
	}
	seed, err := json.Marshal(defaultGGOFieldConfiguration())
	if err != nil {
		return raw, false, err
	}
	cfg["ggo"] = seed
	result, err := json.Marshal(cfg)
	return string(result), err == nil, err
}
func defaultGGOFieldConfiguration() *model.AdminFieldGGOConfiguration {
	return &model.AdminFieldGGOConfiguration{
		Title: "GGO·残响沙域", ModeName: "遗迹回收行动", CurrencyID: "item_9710", ShopKey: "ggo", CarryIcon: "ggo_chip_marker", SproutIcon: "ggo_sprout",
		NPCs: []model.AdminFieldGGONPC{
			{ID: "shino", Name: "行动向导·诗乃", HeroID: "hero_005", Role: "guide", PointLabel: "入口", Dialogue: "先记住接应点的位置。拿到好东西以后，你会需要它。"},
			{ID: "dane", Name: "荒原接应人·戴恩", HeroID: "hero_029", Role: "settlement", PointLabel: "断桥接应点", Dialogue: "货带来了？交给我，剩下的路我来安排。", SettlesChips: true},
			{ID: "kuroe", Name: "高地观察员·克洛维", HeroID: "hero_073", Role: "settlement", PointLabel: "岩脊接应点", Dialogue: "路口暂时没有动静。想撤的话，抓紧。", SettlesChips: true},
			{ID: "serra", Name: "前哨联络员·塞拉", HeroID: "hero_053", Role: "settlement", PointLabel: "遗迹接应点", Dialogue: "接应已准备好。确认一下，你要全部交付吗？", SettlesChips: true},
		},
		Monsters: []model.AdminFieldGGOEnemy{
			{ID: "scavenger", Name: "荒原拾荒枪手", HeroID: "monster_026", RespawnMS: 60000},
			{ID: "assault", Name: "遗迹突击兵", HeroID: "monster_029", RespawnMS: 60000},
			{ID: "raider", Name: "沙巾掠夺者", HeroID: "monster_031", RespawnMS: 60000},
			{ID: "guard", Name: "旧营地守卫", HeroID: "monster_032", RespawnMS: 60000},
			{ID: "worm", Name: "晶壳沙虫", HeroID: "monster_003", RespawnMS: 60000},
		},
		Bosses: []model.AdminFieldGGOEnemy{
			{ID: "behemoth", Name: "重火力霸主·猛兽", HeroID: "hero_017", RespawnMS: 3600000},
			{ID: "pale_rider", Name: "疾行猎手·苍白骑士", HeroID: "hero_016", RespawnMS: 3600000},
			{ID: "death_gun", Name: "幽灵追猎者·死枪", HeroID: "hero_006", RespawnMS: 3600000},
		},
		Chips:      []model.AdminFieldGGOChip{{ItemID: "item_9701", Name: "残损芯片"}, {ItemID: "item_9702", Name: "标准芯片"}, {ItemID: "item_9703", Name: "军用芯片"}, {ItemID: "item_9704", Name: "机密芯片"}, {ItemID: "item_9705", Name: "核心芯片"}, {ItemID: "item_9706", Name: "原型芯片"}},
		Extraction: model.AdminFieldGGOExtraction{Arrival: "已抵达接应点，可交付芯片并撤离。", Confirmation: "交付本次携带的全部芯片，并离开沙域？", ConfirmButton: "交付并撤离", ContinueButton: "继续探索", Countdown: "接应中，请留在区域内。剩余 {秒数} 秒。", Cancelled: "你已离开接应范围，本次撤离已取消。"},
	}
}
