package handler

import (
	"fmt"
	"hyjs-server/model"
	"testing"
)

func ggoRuntimeValidationFixture() *model.AdminFieldGGOConfiguration {
	c := validGGOFieldConfiguration().GGO
	values := []int64{3, 9, 30, 90, 300, 900}
	weights := []int64{5500, 2500, 1300, 500, 180, 20}
	for i := range c.Chips {
		c.Chips[i].ExchangeValue = ggoTestInt64(values[i])
		c.Chips[i].DropWeight = ggoTestInt64(weights[i])
		c.Chips[i].DropAmount = ggoTestInt64(1)
	}
	c.Runtime = &model.AdminFieldGGORuntime{
		EconomyVersion: "desert-v4", ProtectedChips: 3,
		Resource: model.AdminFieldGGOResource{MineCount: 18, FishCount: 12, Capacity: 5, CycleSeconds: 7200, MineCooldownSeconds: 90, FishCooldownSeconds: 60, MineGatherMillis: 3200, FishGatherMillis: 5000, Fatigue: 4, MaterialNumerator: 5, MaterialDenominator: 4, Tiers: []model.AdminFieldGGOResourceTier{
			{Level: 3, Weight: 6000, ChipTriggerBps: 1000}, {Level: 4, Weight: 2800, ChipTriggerBps: 1200}, {Level: 5, Weight: 1000, ChipTriggerBps: 1500}, {Level: 6, Weight: 200, ChipTriggerBps: 1800},
		}},
		MonsterTiers: []model.AdminFieldGGOMobTier{
			{HeroID: "monster_026", Level: 35, Count: 10, HP: 28619, Attack: 5008, PhyDefend: 1431, MagicDefend: 1431, RespawnSeconds: 30, ChipTriggerBps: 2000, RewardNumerator: 1, RewardDenominator: 1},
			{HeroID: "monster_032", Level: 45, Count: 8, HP: 48123, Attack: 8421, PhyDefend: 2406, MagicDefend: 2406, RespawnSeconds: 30, ChipTriggerBps: 3000, RewardNumerator: 3, RewardDenominator: 2},
			{HeroID: "monster_031", Level: 60, Count: 6, HP: 259024, Attack: 45329, PhyDefend: 7000, MagicDefend: 7000, RespawnSeconds: 30, ChipTriggerBps: 4500, RewardNumerator: 2, RewardDenominator: 1},
		},
		Boss: model.AdminFieldGGOBoss{HeroID: "hero_017", HP: 32250000, Attack: 45000, PhyDefend: 7000, MagicDefend: 7000, ParticipationThresholdBps: 300, DailyLimit: 3, ActiveSeconds: 1200, WarningSeconds: 180, EveningStartHour: 18, DayIntervalMinutes: 60, EveningIntervalMinutes: 30, ParticipationItemID: "item_9703", ParticipationAmount: 1, ThresholdAmount: 2, RankRewards: []model.AdminFieldGGORankReward{{From: 1, To: 1, ItemID: "item_9704", Amount: 1}, {From: 2, To: 3, ItemID: "item_9703", Amount: 1}, {From: 4, To: 10, ItemID: "item_9702", Amount: 1}}, TailRewards: []model.AdminFieldGGOWeight{{ItemID: "item_9704", Weight: 8000}, {ItemID: "item_9705", Weight: 1800}, {ItemID: "item_9706", Weight: 200}}},
		Shop: model.AdminFieldGGOShop{PriceNumerator: 7, PriceDenominator: 5, RoundTo: 5, RefreshCost: 15},
	}
	boss := &c.Runtime.Boss
	boss.Level, boss.PartyRounds = 60, 10
	boss.ResourceDrops = []model.AdminFieldDrop{{ResourceID: "diamond", Quantities: []model.AdminFieldQuantity{{Amount: 1500, Weight: 1}}}}
	boss.TailResourceDrops = []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 1, Weight: 1}}}}
	boss.RankResourceDrops = []model.AdminFieldBossRank73{{From: 1, To: 10, Drops: []model.AdminFieldDrop{{ResourceID: "coin", Quantities: []model.AdminFieldQuantity{{Amount: 1, Weight: 1}}}}}}
	boss.SproutMax, boss.SproutBonusBps = 2, 2500
	boss.OverclockRewards = []model.AdminFieldGGOOverclockReward{
		{Sprouts: 0, Quantities: []model.AdminFieldQuantity{{Amount: 6, Weight: 10000}}},
		{Sprouts: 1, Quantities: []model.AdminFieldQuantity{{Amount: 6, Weight: 10000}}},
		{Sprouts: 2, Quantities: []model.AdminFieldQuantity{{Amount: 6, Weight: 10000}}},
	}
	for i := range c.Runtime.Resource.Tiers {
		tier := &c.Runtime.Resource.Tiers[i]
		ore := model.AdminFieldDrop{ResourceID: fmt.Sprintf("MineItem_%03d", tier.Level+6), Quantities: []model.AdminFieldQuantity{{Amount: 3, Weight: 1}}}
		tier.MineDrops = []model.AdminFieldDrop{ore}
		for _, offset := range []int64{6, 12, 18} {
			tier.FishDrops = append(tier.FishDrops, model.AdminFieldDrop{ResourceID: fmt.Sprintf("FishItem_%03d", tier.Level+offset), Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 1}, {Amount: 3, Weight: 1}}})
		}
		if tier.Level == 6 {
			tier.FishDrops = append(tier.FishDrops, model.AdminFieldDrop{ResourceID: "FishItem_025", Quantities: []model.AdminFieldQuantity{{Amount: 0, Weight: 99}, {Amount: 1, Weight: 1}}})
		}
	}
	return c
}

func TestGGORuntimeRejectsBrokenPoolsAndRanges(t *testing.T) {
	if !model.ValidAdminFieldGGORuntime(ggoRuntimeValidationFixture()) {
		t.Fatal("valid V4 rejected")
	}
	tests := []struct {
		name   string
		mutate func(*model.AdminFieldGGOConfiguration)
	}{
		{"tier weights", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Resource.Tiers[0].Weight-- }},
		{"duplicate tier", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Resource.Tiers[1].Level = 3 }},
		{"chip pool", func(c *model.AdminFieldGGOConfiguration) { c.Chips[0].DropWeight = ggoTestInt64(5499) }},
		{"missing conversion", func(c *model.AdminFieldGGOConfiguration) { c.Chips[2].ExchangeValue = nil }},
		{"boss tail", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.TailRewards[0].Weight-- }},
		{"overlapping rank", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.RankRewards[1].From = 1 }},
		{"zero multiplier denominator", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Resource.MaterialDenominator = 0 }},
		{"unknown boss", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.HeroID = "hero_016" }},
		{"boss overlapping events", func(c *model.AdminFieldGGOConfiguration) {
			c.Runtime.Boss.ActiveSeconds = c.Runtime.Boss.EveningIntervalMinutes*60 + 1
		}},
		{"boss excessive rounds", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.PartyRounds = 21 }},
		{"boss invalid probability", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.OverclockRewards[0].Quantities[0].Weight-- }},
		{"boss repeated sprout row", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.OverclockRewards[1].Sprouts = 0 }},
		{"boss resource rank overlap", func(c *model.AdminFieldGGOConfiguration) {
			c.Runtime.Boss.RankResourceDrops = append(c.Runtime.Boss.RankResourceDrops, c.Runtime.Boss.RankResourceDrops[0])
		}},
		{"boss empty resource pool", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Boss.ResourceDrops = nil }},
		{"zero shop denominator", func(c *model.AdminFieldGGOConfiguration) { c.Runtime.Shop.PriceDenominator = 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := ggoRuntimeValidationFixture()
			tc.mutate(c)
			if model.ValidAdminFieldGGORuntime(c) {
				t.Fatal("invalid runtime accepted")
			}
		})
	}
}
