package model

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ValidAdminFieldGGORuntime rejects incomplete probability pools and unsafe arithmetic before publication.
func ValidAdminFieldGGORuntime(g *AdminFieldGGOConfiguration) bool {
	r := g.Runtime
	if r == nil || r.MinClientBuild < 0 || r.MinClientBuild > 2100000000 || r.Enabled && r.MinClientBuild == 0 || (strings.TrimSpace(r.EconomyVersion) == "" || len([]rune(r.EconomyVersion)) > 64) || strings.TrimSpace(r.EconomyVersion) != r.EconomyVersion || r.ProtectedChips < 0 || r.ProtectedChips > 100 {
		return false
	}
	c := r.Resource
	if c.MineCount < 1 || c.MineCount > 50 || c.FishCount < 1 || c.FishCount > 50 ||
		c.Capacity < 1 || c.Capacity > 100 || c.CycleSeconds < 60 || c.CycleSeconds > 86400 || 86400%c.CycleSeconds != 0 ||
		c.MineCooldownSeconds < 1 || c.MineCooldownSeconds > 86400 || c.FishCooldownSeconds < 1 || c.FishCooldownSeconds > 86400 ||
		c.MineGatherMillis < 1 || c.MineGatherMillis > 300000 || c.FishGatherMillis < 1 || c.FishGatherMillis > 300000 ||
		c.Fatigue < 1 || c.Fatigue > 200 || c.MaterialNumerator < 1 || c.MaterialNumerator > 1000 ||
		c.MaterialDenominator != 4 || len(c.Tiers) != 4 {
		return false
	}
	levels := map[int64]bool{}
	var weights int64
	for _, tier := range c.Tiers {
		if tier.Level < 3 || tier.Level > 6 || levels[tier.Level] || tier.Weight < 0 || tier.Weight > 10000 || tier.ChipTriggerBps < 0 || tier.ChipTriggerBps > 10000 {
			return false
		}
		levels[tier.Level] = true
		weights += tier.Weight
		if !validGGOResourceDrops(tier) {
			return false
		}
	}
	if weights != 10000 || len(r.MonsterTiers) != 3 {
		return false
	}
	levels = map[int64]bool{}
	models := map[string]bool{}
	var count int64
	for _, tier := range r.MonsterTiers {
		if (tier.HeroID != "monster_026" && tier.HeroID != "monster_032" && tier.HeroID != "monster_031") || models[tier.HeroID] {
			return false
		}
		models[tier.HeroID] = true
		if tier.Level < 1 || tier.Level > 1000 || levels[tier.Level] || tier.Count < 1 || tier.Count > 50 ||
			tier.HP < 1 || tier.HP > 2000000000 || tier.Attack < 1 || tier.Attack > 2000000000 ||
			tier.PhyDefend < 0 || tier.PhyDefend > 2000000000 || tier.MagicDefend < 0 || tier.MagicDefend > 2000000000 ||
			tier.RespawnSeconds < 1 || tier.RespawnSeconds > 604800 || tier.ChipTriggerBps < 0 || tier.ChipTriggerBps > 10000 ||
			tier.RewardNumerator < 1 || tier.RewardNumerator > 1000 || tier.RewardDenominator < 1 || tier.RewardDenominator > 1000 {
			return false
		}
		levels[tier.Level] = true
		count += tier.Count
	}
	if count > 100 {
		return false
	}
	if len(g.Chips) != 6 {
		return false
	}
	chipIDs := map[string]bool{}
	weights = 0
	for _, chip := range g.Chips {
		if chip.ItemID < "item_9701" || chip.ItemID > "item_9706" || chipIDs[chip.ItemID] {
			return false
		}
		if chip.ExchangeValue == nil || *chip.ExchangeValue < 1 || chip.DropWeight == nil || *chip.DropWeight < 0 || *chip.DropWeight > 10000 ||
			chip.DropAmount == nil || *chip.DropAmount != 1 {
			return false
		}
		chipIDs[chip.ItemID] = true
		weights += *chip.DropWeight
	}
	if weights != 10000 {
		return false
	}
	b := r.Boss
	if b.HeroID != "hero_017" || b.Level < 1 || b.Level > 1000 || b.HP < 1 || b.HP > 2000000000 || b.Attack < 1 || b.Attack > 2000000000 ||
		b.PhyDefend < 0 || b.PhyDefend > 2000000000 || b.MagicDefend < 0 || b.MagicDefend > 2000000000 ||
		b.ParticipationThresholdBps < 1 || b.ParticipationThresholdBps > 10000 || b.DailyLimit < 1 || b.DailyLimit > 100 ||
		b.ActiveSeconds < 1 || b.ActiveSeconds > 86400 || b.PartyRounds < 1 || b.PartyRounds > 20 || b.WarningSeconds < 0 || b.WarningSeconds > 86400 ||
		b.EveningStartHour < 0 || b.EveningStartHour > 23 || b.DayIntervalMinutes < 1 || b.DayIntervalMinutes > 1440 ||
		b.EveningIntervalMinutes < 1 || b.EveningIntervalMinutes > 1440 || b.ActiveSeconds > b.DayIntervalMinutes*60 || b.ActiveSeconds > b.EveningIntervalMinutes*60 ||
		b.SproutMax < 0 || b.SproutMax > 2 || b.SproutBonusBps < 0 || b.SproutBonusBps > 10000 ||
		!validGGOBossResourceAwards(b) ||
		!chipIDs[b.ParticipationItemID] || b.ParticipationAmount < 1 || b.ParticipationAmount > 100 || b.ThresholdAmount < b.ParticipationAmount || b.ThresholdAmount > 100 ||
		len(b.RankRewards) == 0 || len(b.RankRewards) > 100 || len(b.TailRewards) == 0 || len(b.TailRewards) > 6 {
		return false
	}
	ranks := map[int64]bool{}
	for _, award := range b.RankRewards {
		if award.From < 1 || award.To < award.From || award.To > 100 || !chipIDs[award.ItemID] || award.Amount < 1 || award.Amount > 100 {
			return false
		}
		for n := award.From; n <= award.To; n++ {
			if ranks[n] {
				return false
			}
			ranks[n] = true
		}
	}
	weights = 0
	seen := map[string]bool{}
	for _, award := range b.TailRewards {
		if !chipIDs[award.ItemID] || seen[award.ItemID] || award.Weight < 0 || award.Weight > 10000 {
			return false
		}
		seen[award.ItemID] = true
		weights += award.Weight
	}
	if !validAdminGGOPlacement(g) {
		return false
	}
	shop := r.Shop
	return weights == 10000 && shop.PriceNumerator >= 1 && shop.PriceNumerator <= 1000 && shop.PriceDenominator >= 1 && shop.PriceDenominator <= 1000 && shop.RoundTo >= 1 && shop.RoundTo <= 1000 && shop.RefreshCost >= 0 && shop.RefreshCost <= 2000000000
}

// validGGOBossResourceAwards validates bounded pools before catalog checks.
func validGGOBossResourceAwards(b AdminFieldGGOBoss) bool {
	if !validGGOBossDrops(b.ResourceDrops) || !validGGOBossDrops(b.TailResourceDrops) || len(b.RankResourceDrops) < 1 || len(b.RankResourceDrops) > 100 || len(b.OverclockRewards) != 3 {
		return false
	}
	ranks := map[int64]bool{}
	for _, rank := range b.RankResourceDrops {
		if rank.From < 1 || rank.To < rank.From || rank.To > 100 || !validGGOBossDrops(rank.Drops) {
			return false
		}
		for i := rank.From; i <= rank.To; i++ {
			if ranks[i] {
				return false
			}
			ranks[i] = true
		}
	}
	sprouts := map[int64]bool{}
	for _, pool := range b.OverclockRewards {
		if pool.Sprouts < 0 || pool.Sprouts > 2 || sprouts[pool.Sprouts] || len(pool.Quantities) < 1 || len(pool.Quantities) > 32 {
			return false
		}
		sprouts[pool.Sprouts] = true
		amounts := map[int64]bool{}
		var weight int64
		for _, q := range pool.Quantities {
			if q.Amount < 1 || q.Amount > math.MaxInt32 || amounts[q.Amount] || q.Weight < 0 || q.Weight > 10000 {
				return false
			}
			amounts[q.Amount] = true
			weight += q.Weight
		}
		if weight != 10000 {
			return false
		}
	}
	return true
}

func validGGOBossDrops(drops []AdminFieldDrop) bool {
	if len(drops) < 1 || len(drops) > 32 {
		return false
	}
	ids := map[string]bool{}
	for _, drop := range drops {
		if drop.ResourceID == "" || len(drop.ResourceID) > 128 || strings.TrimSpace(drop.ResourceID) != drop.ResourceID || ids[drop.ResourceID] || len(drop.Quantities) < 1 || len(drop.Quantities) > 32 {
			return false
		}
		ids[drop.ResourceID] = true
		amounts := map[int64]bool{}
		var weight int64
		for _, q := range drop.Quantities {
			if q.Amount < 0 || q.Amount > math.MaxInt32 || amounts[q.Amount] || q.Weight < 0 || q.Weight > 1000000 || weight > math.MaxInt32-q.Weight {
				return false
			}
			amounts[q.Amount] = true
			weight += q.Weight
		}
		if weight < 1 {
			return false
		}
	}
	return true
}

func validAdminGGOPosition(p *AdminFieldGGOPosition) bool {
	if p == nil || p.X == nil || p.Y == nil || p.Z == nil {
		return false
	}
	for _, v := range []*float64{p.X, p.Y, p.Z} {
		if math.IsNaN(*v) || math.IsInf(*v, 0) || math.Abs(*v) > 100000 {
			return false
		}
	}
	return true
}

func validAdminGGOPlacement(g *AdminFieldGGOConfiguration) bool {
	r := g.Runtime
	area := r.SafeArea
	if area != nil {
		for _, n := range []float64{area.MinX, area.MaxX, area.MinZ, area.MaxZ} {
			if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 100000 {
				return false
			}
		}
		if area.MinX >= area.MaxX || area.MinZ >= area.MaxZ {
			return false
		}
	} else if r.Enabled {
		return false
	}
	if r.SafeReturn != nil {
		if !validAdminGGOPosition(r.SafeReturn) {
			return false
		}
		if area != nil && (*r.SafeReturn.X < area.MinX || *r.SafeReturn.X > area.MaxX || *r.SafeReturn.Z < area.MinZ || *r.SafeReturn.Z > area.MaxZ) {
			return false
		}
	} else if r.Enabled {
		return false
	}
	if len(r.EntryPoints) > 100 || r.Enabled && len(r.EntryPoints) == 0 {
		return false
	}
	for _, p := range r.EntryPoints {
		if p != nil && !validAdminGGOPosition(p) || p == nil && r.Enabled {
			return false
		}
	}
	if len(r.Points) > 204 {
		return false
	}
	ids := map[string]bool{}
	kinds := map[string]int64{}
	levels := map[int64]int64{}
	for _, p := range r.Points {
		if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.ID) != p.ID || len(p.ID) > 64 || ids[p.ID] ||
			(p.Kind != "monster" && p.Kind != "mine" && p.Kind != "fish" && p.Kind != "boss" && p.Kind != "exit") ||
			math.IsNaN(p.Radius) || math.IsInf(p.Radius, 0) || p.Radius < 0 || p.Radius > 100 {
			return false
		}
		ids[p.ID] = true
		kinds[p.Kind]++
		if p.Kind == "monster" {
			levels[p.Level]++
		} else if p.Level != 0 {
			return false
		}
		if p.Position != nil && !validAdminGGOPosition(p.Position) || r.Enabled && (p.Position == nil || p.Radius <= 0) {
			return false
		}
		if p.StandPosition != nil && !validAdminGGOPosition(p.StandPosition) || r.Enabled && p.Kind == "fish" && p.StandPosition == nil {
			return false
		}
	}
	if !r.Enabled {
		return true
	}
	if kinds["mine"] != r.Resource.MineCount || kinds["fish"] != r.Resource.FishCount || kinds["boss"] != 1 || kinds["exit"] != 3 {
		return false
	}
	var monsters int64
	for _, tier := range r.MonsterTiers {
		if levels[tier.Level] != tier.Count {
			return false
		}
		monsters += tier.Count
		delete(levels, tier.Level)
	}
	if len(levels) > 0 || kinds["monster"] != monsters {
		return false
	}
	roles := map[string]int{}
	npcIDs := map[string]bool{}
	for _, npc := range g.NPCs {
		if !validAdminGGOPosition(npc.Position) || npc.ID == "" || npcIDs[npc.ID] || npc.HeroID == "" {
			return false
		}
		npcIDs[npc.ID] = true
		roles[npc.Role]++
		if (npc.Role == "teleport" || npc.Role == "settlement") && (*npc.Position.X < area.MinX || *npc.Position.X > area.MaxX || *npc.Position.Z < area.MinZ || *npc.Position.Z > area.MaxZ) {
			return false
		}
	}
	if len(g.NPCs) != 5 || roles["teleport"] != 1 || roles["settlement"] != 1 || roles["extraction"] != 3 {
		return false
	}
	_, ok := MatchAdminGGOExitNPCs(g)
	return ok
}

// MatchAdminGGOExitNPCs binds three visible extraction NPCs to three distinct
// actionable exit points. Matching requires the model to lie inside the point's
// configured interaction radius and is stable under configuration reordering.
func MatchAdminGGOExitNPCs(g *AdminFieldGGOConfiguration) (map[string]string, bool) {
	if g == nil || g.Runtime == nil {
		return nil, false
	}
	var npcs []AdminFieldGGONPC
	var exits []AdminFieldGGOPoint
	for _, npc := range g.NPCs {
		if npc.Role == "extraction" {
			npcs = append(npcs, npc)
		}
	}
	for _, point := range g.Runtime.Points {
		if point.Kind == "exit" {
			exits = append(exits, point)
		}
	}
	if len(npcs) != 3 || len(exits) != 3 {
		return nil, false
	}
	sort.Slice(npcs, func(i, j int) bool { return npcs[i].ID < npcs[j].ID })
	sort.Slice(exits, func(i, j int) bool { return exits[i].ID < exits[j].ID })
	used := [3]bool{}
	matches := map[string]string{}
	var assign func(int) bool
	assign = func(index int) bool {
		if index == len(npcs) {
			return true
		}
		npc := npcs[index]
		if !validAdminGGOPosition(npc.Position) {
			return false
		}
		for i, exit := range exits {
			if used[i] || !validAdminGGOPosition(exit.Position) || exit.Radius <= 0 || math.IsNaN(exit.Radius) || math.IsInf(exit.Radius, 0) {
				continue
			}
			dx := *npc.Position.X - *exit.Position.X
			dy := *npc.Position.Y - *exit.Position.Y
			dz := *npc.Position.Z - *exit.Position.Z
			if dx*dx+dy*dy+dz*dz > exit.Radius*exit.Radius {
				continue
			}
			used[i] = true
			matches[npc.ID] = exit.ID
			if assign(index + 1) {
				return true
			}
			used[i] = false
			delete(matches, npc.ID)
		}
		return false
	}
	if !assign(0) {
		return nil, false
	}
	return matches, true
}

func validGGOResourceDrops(tier AdminFieldGGOResourceTier) bool {
	if len(tier.MineDrops) != 1 || tier.MineDrops[0].ResourceID != fmt.Sprintf("MineItem_%03d", tier.Level+6) {
		return false
	}
	allowed := map[string]bool{}
	for _, offset := range []int64{6, 12, 18} {
		allowed[fmt.Sprintf("FishItem_%03d", tier.Level+offset)] = true
	}
	if tier.Level == 6 {
		allowed["FishItem_025"] = true
	}
	if len(tier.FishDrops) != len(allowed) {
		return false
	}
	all := append(append([]AdminFieldDrop{}, tier.MineDrops...), tier.FishDrops...)
	seen := map[string]bool{}
	for i, drop := range all {
		if i > 0 && !allowed[drop.ResourceID] || seen[drop.ResourceID] || len(drop.Quantities) < 1 || len(drop.Quantities) > 20 {
			return false
		}
		seen[drop.ResourceID] = true
		var sum int64
		for _, q := range drop.Quantities {
			if q.Amount < 0 || q.Amount > 10000 || q.Weight < 1 || q.Weight > 1000000 || sum > 10000000-q.Weight {
				return false
			}
			sum += q.Weight
		}
	}
	return true
}
