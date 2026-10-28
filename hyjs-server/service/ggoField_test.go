package service

import (
	"encoding/json"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"testing"
)

func TestGGOSeedPreservesExistingConfiguration(t *testing.T) {
	original := `{"scene":"field_003","minLevel":75,"custom":{"keep":true}}`
	raw, changed, err := addGGOFieldConfiguration(original)
	if err != nil || !changed {
		t.Fatalf("seed: %v %v", changed, err)
	}
	var cfg model.AdminFieldConfiguration
	if err = json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MinLevel != 75 || cfg.GGO == nil || cfg.GGO.SchemaVersion != constUtil.AdminFieldGGOSchemaVersion || len(cfg.GGO.NPCs) != 5 || len(cfg.GGO.Monsters) != 5 || len(cfg.GGO.Bosses) != 3 || len(cfg.GGO.Chips) != 6 {
		t.Fatal("seed or existing fields lost")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	if string(fields["custom"]) != `{"keep":true}` {
		t.Fatal("unknown existing field lost")
	}
	if !cfg.GGO.Rules.UnlimitedCapacity || cfg.GGO.Rules.SearchEnabled || cfg.GGO.Rules.ProtectionSeconds != 30 || (cfg.GGO.Rules.DeathLossPercent == nil || *cfg.GGO.Rules.DeathLossPercent != 50) || cfg.GGO.Rules.WeeklyLimit != nil || cfg.GGO.Rules.WeeklyResetTimezone != "Asia/Shanghai" {
		t.Fatal("rules defaults missing")
	}
	roles := map[string]model.AdminFieldGGONPC{}
	for _, npc := range cfg.GGO.NPCs {
		roles[npc.ID] = npc
		if npc.Position != nil {
			t.Fatal("fabricated coordinate")
		}
	}
	if roles["shino"].Role != "teleport" || roles["dane"].Role != "extraction" || roles["dane"].SettlesChips || roles["kuroe"].Role != "extraction" || roles["serra"].Role != "extraction" || roles["kohl"].HeroID != "clerk01_npc" || roles["kohl"].Role != "settlement" || !roles["kohl"].SettlesChips {
		t.Fatal("npc duties not updated")
	}
	values := []int64{3, 9, 30, 90, 300, 900}
	weights := []int64{5500, 2500, 1300, 500, 180, 20}
	for i, chip := range cfg.GGO.Chips {
		if chip.ExchangeValue == nil || *chip.ExchangeValue != values[i] || chip.DropWeight == nil || *chip.DropWeight != weights[i] || chip.DropAmount == nil || *chip.DropAmount != 1 {
			t.Fatal("approved chip value missing")
		}
	}
	if cfg.GGO.Runtime == nil || cfg.GGO.Runtime.Enabled || cfg.GGO.Runtime.Resource.MineCount != 18 || cfg.GGO.Runtime.Resource.FishCount != 12 {
		t.Fatal("disabled V4 draft defaults missing")
	}
	cfg.GGO.NPCs[1].Dialogue = "用户编辑"
	fields["ggo"], _ = json.Marshal(cfg.GGO)
	edited, _ := json.Marshal(fields)
	repeated, changed, err := addGGOFieldConfiguration(string(edited))
	if err != nil || changed || repeated != string(edited) {
		t.Fatal("existing user configuration overwritten")
	}
}

func TestGGOLegacySchemaMigration(t *testing.T) {
	position := func(value float64) *float64 { return &value }
	exchange := int64(25)
	legacy := model.AdminFieldGGOConfiguration{
		Title: "GGO·残响沙域", ModeName: "遗迹回收行动", CurrencyID: "item_9710", ShopKey: "ggo", CarryIcon: "ggo_chip_marker",
		NPCs: []model.AdminFieldGGONPC{
			{ID: "shino", Name: "自定义诗乃", HeroID: "hero_005", Role: "guide", PointLabel: "入口", Dialogue: "先记住接应点的位置。拿到好东西以后，你会需要它。"},
			{ID: "dane", Name: "荒原接应人·戴恩", HeroID: "hero_029", Role: "settlement", PointLabel: "断桥接应点", Dialogue: "货带来了？交给我，剩下的路我来安排。", Position: &model.AdminFieldGGOPosition{X: position(1), Y: position(2), Z: position(3)}, SettlesChips: true},
			{ID: "kuroe", Name: "自定义克洛维", HeroID: "hero_073", Role: "settlement", PointLabel: "岩脊接应点", Dialogue: "路口暂时没有动静。想撤的话，抓紧。", SettlesChips: true},
			{ID: "serra", Name: "前哨联络员·塞拉", HeroID: "hero_053", Role: "settlement", PointLabel: "遗迹接应点", Dialogue: "自定义接应话术", SettlesChips: true},
		},
		Monsters: []model.AdminFieldGGOEnemy{
			{ID: "scavenger", Name: "荒原拾荒枪手", HeroID: "monster_026", RespawnMS: 60000},
			{ID: "assault", Name: "遗迹突击兵", HeroID: "monster_029", RespawnMS: 60000},
			{ID: "raider", Name: "沙巾掠夺者", HeroID: "monster_031", RespawnMS: 60000},
			{ID: "guard", Name: "旧营地守卫", HeroID: "monster_032", RespawnMS: 120000},
			{ID: "worm", Name: "晶壳沙虫", HeroID: "monster_003", RespawnMS: 60000},
		},
		Bosses: []model.AdminFieldGGOEnemy{
			{ID: "behemoth", Name: "重火力霸主·猛兽", HeroID: "hero_017", RespawnMS: 7200000},
			{ID: "pale_rider", Name: "疾行猎手·苍白骑士", HeroID: "hero_016", RespawnMS: 3600000},
			{ID: "death_gun", Name: "幽灵追猎者·死枪", HeroID: "hero_006", RespawnMS: 3600000},
		},
		Chips: []model.AdminFieldGGOChip{
			{ItemID: "item_9701", Name: "残损芯片", ExchangeValue: &exchange},
			{ItemID: "item_9702", Name: "标准芯片"},
			{ItemID: "item_9703", Name: "军用芯片"},
			{ItemID: "item_9704", Name: "机密芯片"},
			{ItemID: "item_9705", Name: "核心芯片"},
			{ItemID: "item_9706", Name: "原型芯片"},
		},
		Extraction: model.AdminFieldGGOExtraction{Arrival: "已抵达接应点，可交付芯片并撤离。", Confirmation: "交付本次携带的全部芯片，并离开沙域？", ConfirmButton: "交付并撤离", ContinueButton: "继续探索", Countdown: "接应中，请留在区域内。剩余 {秒数} 秒。", Cancelled: "自定义取消提示"},
	}
	raw, _ := json.Marshal(map[string]interface{}{"scene": "field_003", "minLevel": 75, "custom": map[string]bool{"keep": true}, "ggo": legacy})
	migrated, changed, err := addGGOFieldConfiguration(string(raw))
	if err != nil || !changed {
		t.Fatalf("migrate: %v %v", changed, err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal([]byte(migrated), &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["custom"]) != `{"keep":true}` {
		t.Fatal("unknown existing field lost")
	}
	var cfg model.AdminFieldConfiguration
	if err = json.Unmarshal([]byte(migrated), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.GGO.SchemaVersion != constUtil.AdminFieldGGOSchemaVersion || cfg.GGO.Rules.ProtectionSeconds != 30 || cfg.GGO.Rules.WeeklyServerScope != "pending" {
		t.Fatal("schema or rules not migrated")
	}
	npcs := map[string]model.AdminFieldGGONPC{}
	for _, npc := range cfg.GGO.NPCs {
		npcs[npc.ID] = npc
	}
	if len(cfg.GGO.NPCs) != 5 || npcs["shino"].Role != "teleport" || npcs["dane"].Role != "extraction" || npcs["dane"].SettlesChips || npcs["kuroe"].Role != "extraction" || npcs["serra"].Role != "extraction" || npcs["kohl"].Role != "settlement" || !npcs["kohl"].SettlesChips {
		t.Fatal("npc duties not migrated")
	}
	if npcs["shino"].Name != "自定义诗乃" || npcs["kuroe"].Name != "自定义克洛维" || npcs["serra"].Dialogue != "自定义接应话术" {
		t.Fatal("manual npc text lost")
	}
	if npcs["dane"].Dialogue == "货带来了？交给我，剩下的路我来安排。" || npcs["dane"].Position == nil || *npcs["dane"].Position.X != 1 || *npcs["dane"].Position.Y != 2 || *npcs["dane"].Position.Z != 3 {
		t.Fatal("default dialogue or coordinate handling wrong")
	}
	monsters := map[string]int64{}
	for _, monster := range cfg.GGO.Monsters {
		monsters[monster.ID] = monster.RespawnMS
	}
	if monsters["scavenger"] != 30000 || monsters["assault"] != 30000 || monsters["raider"] != 30000 || monsters["worm"] != 30000 || monsters["guard"] != 120000 {
		t.Fatal("monster respawn migration wrong")
	}
	if cfg.GGO.Bosses[0].RespawnMS != 7200000 || cfg.GGO.Chips[0].ExchangeValue == nil || *cfg.GGO.Chips[0].ExchangeValue != 3 || cfg.GGO.Chips[0].DropWeight == nil || *cfg.GGO.Chips[0].DropWeight != 5500 {
		t.Fatal("custom boss or approved V4 chip value lost")
	}
	if cfg.GGO.Extraction.Cancelled != "自定义取消提示" || cfg.GGO.Extraction.Arrival == "已抵达接应点，可交付芯片并撤离。" {
		t.Fatal("extraction text migration wrong")
	}
	repeated, changed, err := addGGOFieldConfiguration(migrated)
	if err != nil || changed || repeated != migrated {
		t.Fatal("migration is not idempotent")
	}
}

func TestGGOSproutMovesWithoutReset55(t *testing.T) {
	raw := `{"scene":"field_003","opaque":{"keep":true},"ggo":{"schemaVersion":2,"sproutIcon":"ggo_sprout","rules":{"deathLossPercent":42},"opaque":{"keep":true}}}`
	updated, changed, err := addGGOFieldConfiguration(raw)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(updated), &doc); err != nil {
		t.Fatal(err)
	}
	g := doc["ggo"].(map[string]interface{})
	if _, ok := g["sproutIcon"]; ok {
		t.Fatal("sprout remained in desert")
	}
	if g["rules"].(map[string]interface{})["deathLossPercent"] != float64(50) || g["opaque"] == nil || doc["opaque"] == nil {
		t.Fatal("unrelated data changed")
	}
	again, changed, err := addGGOFieldConfiguration(updated)
	if err != nil || changed || again != updated {
		t.Fatal("migration repeated", changed, err)
	}
}

func TestGGOV3MigrationPreservesOpaqueFieldsAndIsIdempotent(t *testing.T) {
	base := defaultGGOFieldConfiguration()
	base.SchemaVersion = 2
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var old map[string]json.RawMessage
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	old["opaque"] = json.RawMessage(`{"preserve":true}`)
	var rules map[string]json.RawMessage
	if err = json.Unmarshal(old["rules"], &rules); err != nil {
		t.Fatal(err)
	}
	rules["otherPolicy"] = json.RawMessage(`{"custom":7}`)
	old["rules"], _ = json.Marshal(rules)
	var chips []map[string]json.RawMessage
	if err = json.Unmarshal(old["chips"], &chips); err != nil {
		t.Fatal(err)
	}
	chips[0]["opaque"] = json.RawMessage(`"keep"`)
	old["chips"], _ = json.Marshal(chips)
	wrapped, _ := json.Marshal(map[string]json.RawMessage{"scene": json.RawMessage(`"field_003"`), "ggo": mustGGOTestJSON(t, old)})
	migrated, changed, err := addGGOFieldConfiguration(string(wrapped))
	if err != nil || !changed {
		t.Fatal("migration", changed, err)
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal([]byte(migrated), &doc); err != nil {
		t.Fatal(err)
	}
	var g map[string]json.RawMessage
	if err = json.Unmarshal(doc["ggo"], &g); err != nil {
		t.Fatal(err)
	}
	if string(g["opaque"]) != `{"preserve":true}` {
		t.Fatal("opaque GGO field lost")
	}
	if err = json.Unmarshal(g["rules"], &rules); err != nil {
		t.Fatal(err)
	}
	if string(rules["otherPolicy"]) != `{"custom":7}` {
		t.Fatal("opaque rule lost")
	}
	if err = json.Unmarshal(g["chips"], &chips); err != nil {
		t.Fatal(err)
	}
	if string(chips[0]["opaque"]) != `"keep"` {
		t.Fatal("opaque chip field lost")
	}
	again, changed, err := addGGOFieldConfiguration(migrated)
	if err != nil || changed || again != migrated {
		t.Fatal("migration repeated", changed, err)
	}
}

func mustGGOTestJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Old schema-3 drafts receive only absent fields. Explicit zeros and custom
// reward pools remain exactly as an operator saved them.
func TestGGOBossDraftCompletesOnlyMissingFields(t *testing.T) {
	boss := map[string]any{"partyRounds": int64(7), "sproutMax": int64(0), "custom": "keep"}
	encoded, _ := json.Marshal(map[string]any{"runtime": map[string]any{"boss": boss}})
	var ggo map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &ggo); err != nil {
		t.Fatal(err)
	}
	changed, err := completeGGOV3Models(ggo)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	var got struct {
		Boss struct {
			PartyRounds       int64                                `json:"partyRounds"`
			SproutMax         int64                                `json:"sproutMax"`
			SproutBonusBps    int64                                `json:"sproutBonusBps"`
			ResourceDrops     []model.AdminFieldDrop               `json:"resourceDrops"`
			RankResourceDrops []model.AdminFieldBossRank73         `json:"rankResourceDrops"`
			TailResourceDrops []model.AdminFieldDrop               `json:"tailResourceDrops"`
			OverclockRewards  []model.AdminFieldGGOOverclockReward `json:"overclockRewards"`
			Custom            string                               `json:"custom"`
		} `json:"boss"`
	}
	if err = json.Unmarshal(ggo["runtime"], &got); err != nil {
		t.Fatal(err)
	}
	if got.Boss.PartyRounds != 7 || got.Boss.SproutMax != 0 || got.Boss.SproutBonusBps != 2500 || len(got.Boss.ResourceDrops) != 19 || len(got.Boss.RankResourceDrops) != 3 || len(got.Boss.TailResourceDrops) != 5 || len(got.Boss.OverclockRewards) != 3 || got.Boss.Custom != "keep" {
		t.Fatal("draft migration changed explicit values or omitted defaults", got)
	}
	before := string(ggo["runtime"])
	changed, err = completeGGOV3Models(ggo)
	if err != nil || changed || string(ggo["runtime"]) != before {
		t.Fatal("draft migration not idempotent", changed, err)
	}
}

func TestGGOBossDefaultRewardCatalog(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	boss := defaultGGORuntime().Boss
	if boss.PartyRounds != 10 || boss.SproutMax != 2 || boss.SproutBonusBps != 2500 || !s.AdminGGOFieldBossResourceCatalogValid(boss) {
		t.Fatal("default boss original reward catalog invalid")
	}
	boss.ResourceDrops[0].ResourceID = "MineItem_999"
	if s.AdminGGOFieldBossResourceCatalogValid(boss) {
		t.Fatal("unknown resource accepted")
	}
}
