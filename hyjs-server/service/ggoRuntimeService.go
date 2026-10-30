package service

import (
	"encoding/json"
	"fmt"

	"hyjs-server/model"
)

func ggoInt64(v int64) *int64       { return &v }
func ggoBool(v bool) *bool          { return &v }
func ggoFloat64(v float64) *float64 { return &v }

// 默认配置仅写入草案。实际生效须显式启用并发布场景快照。
func defaultGGORuntime() *model.AdminFieldGGORuntime {
	return &model.AdminFieldGGORuntime{
		Enabled: false, EconomyVersion: "desert-v4", ProtectedChips: 3,
		SafeArea:   &model.AdminFieldGGOSafeArea{MinX: -11, MaxX: 3, MinZ: -10, MaxZ: 1},
		SafeReturn: &model.AdminFieldGGOPosition{X: ggoFloat64(-7), Y: ggoFloat64(0), Z: ggoFloat64(-6)},
		Points:     []model.AdminFieldGGOPoint{}, EntryPoints: []*model.AdminFieldGGOPosition{},
		Resource: model.AdminFieldGGOResource{
			MineCount: 18, FishCount: 12, Capacity: 5, CycleSeconds: 7200,
			MineCooldownSeconds: 90, FishCooldownSeconds: 60,
			MineGatherMillis: 3200, FishGatherMillis: 5000, Fatigue: 4,
			MaterialNumerator: 5, MaterialDenominator: 4,
			Tiers: defaultGGOResourceTiers(),
		},
		// Per-member values match the original field groups at the same levels;
		// each encounter uses four members at the original group positions.
		MonsterTiers: []model.AdminFieldGGOMobTier{
			{HeroID: "monster_026", Level: 35, Count: 10, HP: 28619, Attack: 5008, PhyDefend: 1431, MagicDefend: 1431, RespawnSeconds: 30, ChipTriggerBps: 2000, RewardNumerator: 1, RewardDenominator: 1},
			{HeroID: "monster_032", Level: 45, Count: 8, HP: 48123, Attack: 8421, PhyDefend: 2406, MagicDefend: 2406, RespawnSeconds: 30, ChipTriggerBps: 3000, RewardNumerator: 3, RewardDenominator: 2},
			{HeroID: "monster_031", Level: 60, Count: 6, HP: 259024, Attack: 45329, PhyDefend: 7000, MagicDefend: 7000, RespawnSeconds: 30, ChipTriggerBps: 4500, RewardNumerator: 2, RewardDenominator: 1},
		},
		Boss: model.AdminFieldGGOBoss{
			HeroID: "hero_017", Level: 60, HP: 134400000, Attack: 45000, PhyDefend: 7000, MagicDefend: 7000,
			ParticipationThresholdBps: 300, DailyLimit: 3, ActiveSeconds: 1200, PartyRounds: 10, WarningSeconds: 180,
			EveningStartHour: 18, DayIntervalMinutes: 60, EveningIntervalMinutes: 30,
			ParticipationItemID: "item_9703", ParticipationAmount: 1, ThresholdAmount: 2,
			RankRewards: []model.AdminFieldGGORankReward{
				{From: 1, To: 1, ItemID: "item_9704", Amount: 1},
				{From: 2, To: 3, ItemID: "item_9703", Amount: 1},
				{From: 4, To: 10, ItemID: "item_9702", Amount: 1},
			},
			TailRewards: []model.AdminFieldGGOWeight{
				{ItemID: "item_9704", Weight: 8000}, {ItemID: "item_9705", Weight: 1800}, {ItemID: "item_9706", Weight: 200},
			},
			ResourceDrops:     defaultGGOBossResourceDrops(),
			RankResourceDrops: defaultGGOBossRankResourceDrops(),
			TailResourceDrops: defaultGGOBossTailResourceDrops(),
			SproutMax:         2, SproutBonusBps: 2500, OverclockRewards: defaultGGOBossOverclockRewards(),
		},
		Shop: model.AdminFieldGGOShop{PriceNumerator: 7, PriceDenominator: 5, RoundTo: 5, RefreshCost: 15},
	}
}

// Fixed pools retain original resource IDs and independent quantity draws.
func ggoFixedDrop(id string, amount int64) model.AdminFieldDrop {
	return model.AdminFieldDrop{ResourceID: id, Quantities: []model.AdminFieldQuantity{{Amount: amount, Weight: 1}}}
}

func defaultGGOBossResourceDrops() []model.AdminFieldDrop {
	drops := []model.AdminFieldDrop{ggoFixedDrop("diamond", 1500), ggoFixedDrop("coin", 8000000)}
	for i, n := range []int64{20, 15, 10, 5} {
		drops = append(drops, ggoFixedDrop(fmt.Sprintf("MineItem_%03d", 9+i), n))
	}
	for i, n := range []int64{4, 3, 2, 1} {
		for _, offset := range []int{9, 15, 21} {
			drops = append(drops, ggoFixedDrop(fmt.Sprintf("FishItem_%03d", offset+i), n))
		}
	}
	return append(drops, model.AdminFieldDrop{ResourceID: "FishItem_025", Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 9900}, {Amount: 1, Weight: 100}}})
}

func ggoBossRankDrops(diamonds, mine, fish int64) []model.AdminFieldDrop {
	drops := []model.AdminFieldDrop{ggoFixedDrop("diamond", diamonds), ggoFixedDrop(fmt.Sprintf("MineItem_%03d", mine), 5)}
	for _, id := range []int64{fish, fish + 6, fish + 12} {
		drops = append(drops, ggoFixedDrop(fmt.Sprintf("FishItem_%03d", id), 1))
	}
	return drops
}

func defaultGGOBossRankResourceDrops() []model.AdminFieldBossRank73 {
	return []model.AdminFieldBossRank73{
		{From: 1, To: 1, Drops: ggoBossRankDrops(500, 12, 12)},
		{From: 2, To: 3, Drops: ggoBossRankDrops(250, 11, 11)},
		{From: 4, To: 10, Drops: []model.AdminFieldDrop{ggoFixedDrop("diamond", 100), ggoFixedDrop("MineItem_009", 3)}},
	}
}

func defaultGGOBossTailResourceDrops() []model.AdminFieldDrop {
	return ggoBossRankDrops(300, 12, 12)
}

func defaultGGOBossOverclockRewards() []model.AdminFieldGGOOverclockReward {
	amounts := []int64{6, 18, 28, 64, 128}
	weights := [][]int64{{4000, 3000, 2000, 800, 200}, {3500, 3000, 2200, 1000, 300}, {3000, 3000, 2300, 1300, 400}}
	out := make([]model.AdminFieldGGOOverclockReward, len(weights))
	for sprouts, row := range weights {
		out[sprouts].Sprouts = int64(sprouts)
		for i, weight := range row {
			out[sprouts].Quantities = append(out[sprouts].Quantities, model.AdminFieldQuantity{Amount: amounts[i], Weight: weight})
		}
	}
	return out
}

// AdminGGOFieldBossResourceCatalogValid checks every configured reward against
// the actual original reward consumer, including every nonzero quantity.
func (s *GameService) AdminGGOFieldBossResourceCatalogValid(b model.AdminFieldGGOBoss) bool {
	groups := [][]model.AdminFieldDrop{b.ResourceDrops, b.TailResourceDrops}
	for _, rank := range b.RankResourceDrops {
		groups = append(groups, rank.Drops)
	}
	for _, drops := range groups {
		for _, drop := range drops {
			if !s.AdminFieldBossDropAllowed(drop.ResourceID) {
				return false
			}
			for _, q := range drop.Quantities {
				if !s.AdminFieldBossAmountAllowed(drop.ResourceID, q.Amount) {
					return false
				}
			}
		}
	}
	return true
}

func defaultGGOResourceTiers() []model.AdminFieldGGOResourceTier {
	weights := []int64{6000, 2800, 1000, 200}
	trigger := []int64{1000, 1200, 1500, 1800}
	ores := [][3]int64{{3, 6, 8}, {3, 6, 9}, {3, 6, 9}, {4, 7, 10}}
	fishChances := [][3]int64{{75, 50, 50}, {75, 40, 40}, {75, 20, 20}, {75, 10, 10}}
	tiers := make([]model.AdminFieldGGOResourceTier, 4)
	for i := range tiers {
		level := int64(i + 3)
		oresID := fmt.Sprintf("MineItem_%03d", level+6)
		mine := model.AdminFieldDrop{ResourceID: oresID, Quantities: []model.AdminFieldQuantity{}}
		for _, amount := range ores[i] {
			mine.Quantities = append(mine.Quantities, model.AdminFieldQuantity{Amount: amount, Weight: 1})
		}
		fish := []model.AdminFieldDrop{}
		base := int64(3)
		if level == 6 {
			base = 4
		}
		for j, offset := range []int64{6, 12, 18} {
			chance := fishChances[i][j]
			fish = append(fish, model.AdminFieldDrop{ResourceID: fmt.Sprintf("FishItem_%03d", level+offset), Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 100 - chance}, {Amount: base, Weight: chance}}})
		}
		if level == 6 {
			fish = append(fish, model.AdminFieldDrop{ResourceID: "FishItem_025", Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 99}, {Amount: 1, Weight: 1}}})
		}
		tiers[i] = model.AdminFieldGGOResourceTier{Level: level, Weight: weights[i], ChipTriggerBps: trigger[i], MineDrops: []model.AdminFieldDrop{mine}, FishDrops: fish}
	}
	return tiers
}

func defaultGGOChips() []model.AdminFieldGGOChip {
	names := []string{"残损芯片", "标准芯片", "军用芯片", "机密芯片", "核心芯片", "原型芯片"}
	values := []int64{3, 9, 30, 90, 300, 900}
	weights := []int64{5500, 2500, 1300, 500, 180, 20}
	chips := make([]model.AdminFieldGGOChip, len(names))
	for i := range chips {
		chips[i] = model.AdminFieldGGOChip{ItemID: fmt.Sprintf("item_%d", 9701+i), Name: names[i], ExchangeValue: ggoInt64(values[i]), DropWeight: ggoInt64(weights[i]), DropAmount: ggoInt64(1)}
	}
	return chips
}

// migrateGGOV3 preserves opaque JSON, customized text and positions while applying approved V4 economics.
// Only drafts pass this function; a published deployment and its frozen economy stay untouched.
func migrateGGOV3(cfg map[string]json.RawMessage, raw json.RawMessage) (string, bool, error) {
	var existing map[string]json.RawMessage
	if err := json.Unmarshal(raw, &existing); err != nil {
		return "", false, err
	}
	if existing == nil {
		return "", false, fmt.Errorf("沙漠配置为空")
	}
	encode := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	existing["schemaVersion"] = encode(3)
	existing["runtime"] = encode(defaultGGORuntime())
	delete(existing, "sproutIcon")
	var rules map[string]json.RawMessage
	if v := existing["rules"]; len(v) > 0 {
		if err := json.Unmarshal(v, &rules); err != nil {
			return "", false, err
		}
	}
	if rules == nil {
		rules = make(map[string]json.RawMessage)
	}
	for key, value := range map[string]any{
		"protectionSeconds": int64(30), "initialSource": "monsters_and_gathering",
		"deathLossPercent": int64(50), "pveLossMode": "destroy",
		"extractionSeconds": int64(10), "safeTradeEnabled": true,
		"tradeAssets": "chips_only", "allowGift": true,
	} {
		rules[key] = encode(value)
	}
	existing["rules"] = encode(rules)
	var chips []map[string]json.RawMessage
	if v := existing["chips"]; len(v) > 0 {
		if err := json.Unmarshal(v, &chips); err != nil {
			return "", false, err
		}
	}
	defaults := defaultGGOChips()
	for _, chip := range chips {
		var id string
		if err := json.Unmarshal(chip["itemId"], &id); err != nil {
			return "", false, err
		}
		for _, def := range defaults {
			if id == def.ItemID {
				chip["exchangeValue"] = encode(*def.ExchangeValue)
				chip["dropWeight"] = encode(*def.DropWeight)
				chip["dropAmount"] = encode(*def.DropAmount)
				break
			}
		}
	}
	existing["chips"] = encode(chips)
	cfg["ggo"] = encode(existing)
	result, err := json.Marshal(cfg)
	return string(result), err == nil, err
}

// completeGGOV3Models upgrades early V4 drafts without replacing operator choices.
func completeGGOV3Models(ggo map[string]json.RawMessage) (bool, error) {
	if len(ggo["runtime"]) == 0 || string(ggo["runtime"]) == "null" {
		return false, nil
	}
	var runtime map[string]json.RawMessage
	if err := json.Unmarshal(ggo["runtime"], &runtime); err != nil {
		return false, err
	}
	if runtime == nil {
		return false, nil
	}
	changed := false
	if _, present := runtime["minClientBuild"]; !present {
		runtime["minClientBuild"], _ = json.Marshal(int64(0))
		changed = true
	}
	if len(runtime["monsterTiers"]) > 0 {
		var tiers []map[string]json.RawMessage
		if err := json.Unmarshal(runtime["monsterTiers"], &tiers); err != nil {
			return false, err
		}
		for _, tier := range tiers {
			var hero string
			if raw := tier["heroId"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &hero); err != nil {
					return false, err
				}
			}
			if hero != "" {
				continue
			}
			var level int64
			if err := json.Unmarshal(tier["level"], &level); err != nil {
				return false, err
			}
			hero = map[int64]string{35: "monster_026", 45: "monster_032", 60: "monster_031"}[level]
			if hero != "" {
				tier["heroId"], _ = json.Marshal(hero)
				changed = true
			}
		}
		if changed {
			runtime["monsterTiers"], _ = json.Marshal(tiers)
		}
	}
	if len(runtime["resource"]) > 0 {
		var resource map[string]json.RawMessage
		if err := json.Unmarshal(runtime["resource"], &resource); err != nil {
			return false, err
		}
		if resource != nil && len(resource["tiers"]) > 0 {
			var tiers []map[string]json.RawMessage
			if err := json.Unmarshal(resource["tiers"], &tiers); err != nil {
				return false, err
			}
			defaults := defaultGGOResourceTiers()
			resourceChanged := false
			for _, tier := range tiers {
				var level int64
				if err := json.Unmarshal(tier["level"], &level); err != nil {
					return false, err
				}
				for _, def := range defaults {
					if def.Level != level {
						continue
					}
					if len(tier["mineDrops"]) == 0 {
						tier["mineDrops"], _ = json.Marshal(def.MineDrops)
						resourceChanged = true
					}
					if len(tier["fishDrops"]) == 0 {
						tier["fishDrops"], _ = json.Marshal(def.FishDrops)
						resourceChanged = true
					}
				}
			}
			if resourceChanged {
				resource["tiers"], _ = json.Marshal(tiers)
				runtime["resource"], _ = json.Marshal(resource)
				changed = true
			}
		}
	}
	if len(runtime["boss"]) > 0 && string(runtime["boss"]) != "null" {
		var boss map[string]json.RawMessage
		if err := json.Unmarshal(runtime["boss"], &boss); err != nil {
			return false, err
		}
		if boss != nil {
			defaults := defaultGGORuntime().Boss
			values := map[string]any{
				"level": defaults.Level, "partyRounds": defaults.PartyRounds, "resourceDrops": defaults.ResourceDrops,
				"rankResourceDrops": defaults.RankResourceDrops, "tailResourceDrops": defaults.TailResourceDrops,
				"sproutMax": defaults.SproutMax, "sproutBonusBps": defaults.SproutBonusBps,
				"overclockRewards": defaults.OverclockRewards,
			}
			bossChanged := false
			for key, value := range values {
				if _, present := boss[key]; !present {
					boss[key], _ = json.Marshal(value)
					bossChanged = true
				}
			}
			if bossChanged {
				runtime["boss"], _ = json.Marshal(boss)
				changed = true
			}
		}
	}
	if changed {
		ggo["runtime"], _ = json.Marshal(runtime)
	}
	return changed, nil
}
