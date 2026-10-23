package handler

import (
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"testing"
)

func TestAdminFieldValidGGORejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.AdminFieldConfiguration)
	}{
		{name: "missing schema version", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.SchemaVersion = 0 }},
		{name: "missing rules", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules = model.AdminFieldGGORules{} }},
		{name: "wrong scene", mutate: func(c *model.AdminFieldConfiguration) { c.Scene = "field_004" }},
		{name: "wrong currency", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.CurrencyID = "item_1" }},
		{name: "empty title", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Title = " " }},
		{name: "wrong entry method", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.EntryMethod = "direct" }},
		{name: "protection below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ProtectionSeconds = -1 }},
		{name: "protection above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ProtectionSeconds = 301 }},
		{name: "wrong terminal name", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.TerminalName = "仓库" }},
		{name: "limited capacity", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.UnlimitedCapacity = false }},
		{name: "search enabled", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.SearchEnabled = true }},
		{name: "wrong initial source", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.InitialSource = "players" }},
		{name: "wrong pvp transfer", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.PvpTransfer = "winner_inventory" }},
		{name: "wrong death return", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.DeathReturn = "terminal" }},
		{name: "death loss below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.DeathLossPercent = ggoTestInt64(-1) }},
		{name: "death loss above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.DeathLossPercent = ggoTestInt64(101) }},
		{name: "wrong pve loss mode", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.PveLossMode = "keep" }},
		{name: "wrong extraction return", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ExtractionReturn = "terminal" }},
		{name: "extraction seconds below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ExtractionSeconds = ggoTestInt64(-1) }},
		{name: "extraction seconds above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ExtractionSeconds = ggoTestInt64(301) }},
		{name: "wrong settlement mode", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.SettlementMode = "auto" }},
		{name: "retain disabled", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.RetainAfterDeath = false }},
		{name: "wrong trade assets", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.TradeAssets = "all" }},
		{name: "wrong credit cap", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.CreditCap = "weekly" }},
		{name: "wrong shop limit scope", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.ShopLimitScope = "account_daily" }},
		{name: "weekly limit below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyLimit = ggoTestInt64(0) }},
		{name: "weekly limit above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyLimit = ggoTestInt64(10001) }},
		{name: "reset day below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyResetDay = ggoTestInt64(0) }},
		{name: "reset day above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyResetDay = ggoTestInt64(8) }},
		{name: "reset hour above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyResetHour = ggoTestInt64(24) }},
		{name: "reset minute above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyResetMinute = ggoTestInt64(60) }},
		{name: "wrong reset timezone", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyResetTimezone = "UTC" }},
		{name: "wrong weekly server scope", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.WeeklyServerScope = "server" }},
		{name: "empty leave confirmation", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.LeaveConfirmation = " " }},
		{name: "empty leave confirm button", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.LeaveConfirmButton = "" }},
		{name: "empty leave cancel button", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Rules.LeaveCancelButton = "" }},
		{name: "missing npc", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.NPCs = c.GGO.NPCs[:4] }},
		{name: "extra npc", mutate: func(c *model.AdminFieldConfiguration) {
			c.GGO.NPCs = append(c.GGO.NPCs, model.AdminFieldGGONPC{ID: "extra", Name: "额外", HeroID: "hero_005", Role: "teleport", PointLabel: "入口", Dialogue: "额外"})
		}},
		{name: "illegal npc id", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.NPCs[4].ID = "other" }},
		{name: "illegal npc model", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.NPCs[1].HeroID = "hero_005" }},
		{name: "illegal npc role", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.NPCs[0].Role = "guide" }},
		{name: "illegal npc settlement", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.NPCs[4].SettlesChips = false }},
		{name: "partial npc position", mutate: func(c *model.AdminFieldConfiguration) {
			c.GGO.NPCs[0].Position = &model.AdminFieldGGOPosition{X: ggoTestFloat64(1)}
		}},
		{name: "missing monster", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Monsters = c.GGO.Monsters[:4] }},
		{name: "illegal monster id", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Monsters[0].ID = "other" }},
		{name: "illegal monster model", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Monsters[0].HeroID = "monster_029" }},
		{name: "monster respawn below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Monsters[0].RespawnMS = 999 }},
		{name: "monster respawn above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Monsters[0].RespawnMS = 604800001 }},
		{name: "missing boss", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Bosses = c.GGO.Bosses[:2] }},
		{name: "illegal boss model", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Bosses[0].HeroID = "hero_016" }},
		{name: "missing chip", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips = c.GGO.Chips[:5] }},
		{name: "illegal chip id", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].ItemID = "item_9707" }},
		{name: "negative chip exchange", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].ExchangeValue = ggoTestInt64(-1) }},
		{name: "chip drop weight below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].DropWeight = ggoTestInt64(-1) }},
		{name: "chip drop weight above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].DropWeight = ggoTestInt64(1000001) }},
		{name: "chip drop amount below range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].DropAmount = ggoTestInt64(0) }},
		{name: "chip drop amount above range", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Chips[0].DropAmount = ggoTestInt64(10001) }},
		{name: "empty arrival", mutate: func(c *model.AdminFieldConfiguration) { c.GGO.Extraction.Arrival = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validGGOFieldConfiguration()
			tc.mutate(cfg)
			if adminFieldValidGGO(cfg) {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestAdminFieldValidGGOAllowsPendingValues(t *testing.T) {
	cfg := validGGOFieldConfiguration()
	cfg.GGO.Chips[0].DropWeight = ggoTestInt64(100)
	cfg.GGO.Chips[1].DropAmount = ggoTestInt64(1)
	cfg.GGO.Rules.SafeTradeEnabled = false
	cfg.GGO.Rules.AllowGift = ggoTestBool(false)
	if !adminFieldValidGGO(cfg) {
		t.Fatal("pending or optional values rejected")
	}
}

func validGGOFieldConfiguration() *model.AdminFieldConfiguration {
	return &model.AdminFieldConfiguration{
		Scene: "field_003",
		GGO: &model.AdminFieldGGOConfiguration{
			SchemaVersion: constUtil.AdminFieldGGOSchemaVersion,
			Title:         "GGO·残响沙域", ModeName: "遗迹回收行动", CurrencyID: "item_9710", ShopKey: "ggo", CarryIcon: "ggo_chip_marker", SproutIcon: "ggo_sprout",
			Rules: model.AdminFieldGGORules{
				EntryMethod: "npc_random", ProtectionSeconds: 30, TerminalName: "临时终端", UnlimitedCapacity: true,
				InitialSource: "monsters", PvpTransfer: "winner_terminal", DeathReturn: "safe_area", PveLossMode: "pending",
				ExtractionReturn: "safe_area", SettlementMode: "safe_npc_or_confirm_leave", RetainAfterDeath: true,
				SafeTradeEnabled: true, TradeAssets: "pending", CreditCap: "none", ShopLimitScope: "account_weekly",
				WeeklyResetTimezone: "Asia/Shanghai", WeeklyServerScope: "pending",
				LeaveConfirmation:  "离开后将自动结算临时终端中的全部芯片，预计获得 {信用点} 信用点。是否离开？",
				LeaveConfirmButton: "结算并离开", LeaveCancelButton: "取消",
			},
			NPCs: []model.AdminFieldGGONPC{
				{ID: "shino", Name: "行动向导·诗乃", HeroID: "hero_005", Role: "teleport", PointLabel: "入口", Dialogue: "先记住接应点的位置。"},
				{ID: "dane", Name: "荒原接应人·戴恩", HeroID: "hero_029", Role: "extraction", PointLabel: "断桥接应点", Dialogue: "我会把你送回初始安全区。"},
				{ID: "kuroe", Name: "高地观察员·克洛维", HeroID: "hero_073", Role: "extraction", PointLabel: "岩脊接应点", Dialogue: "撤离点已确认。"},
				{ID: "serra", Name: "前哨联络员·塞拉", HeroID: "hero_053", Role: "extraction", PointLabel: "遗迹接应点", Dialogue: "接应已准备好。"},
				{ID: "kohl", Name: "终端结算员·柯尔", HeroID: "clerk01_npc", Role: "settlement", PointLabel: "安全区终端", Dialogue: "在这里完成结算。", SettlesChips: true},
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
			Chips: []model.AdminFieldGGOChip{
				{ItemID: "item_9701", Name: "残损芯片"},
				{ItemID: "item_9702", Name: "标准芯片"},
				{ItemID: "item_9703", Name: "军用芯片"},
				{ItemID: "item_9704", Name: "机密芯片"},
				{ItemID: "item_9705", Name: "核心芯片"},
				{ItemID: "item_9706", Name: "原型芯片"},
			},
			Extraction: model.AdminFieldGGOExtraction{Arrival: "已抵达撤离点，可返回初始安全区。", Confirmation: "返回初始安全区？", ConfirmButton: "返回安全区", ContinueButton: "继续探索", Countdown: "撤离中。", Cancelled: "已取消。"},
		},
	}
}

func ggoTestInt64(value int64) *int64 {
	return &value
}

func ggoTestBool(value bool) *bool {
	return &value
}

func ggoTestFloat64(value float64) *float64 {
	return &value
}
