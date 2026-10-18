package service

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
	"strings"
)

type p5Investment struct {
	Type          string             `json:"type"`
	Baseline      json.RawMessage    `json:"baseline"`
	Expected      string             `json:"expected"`
	Costs         []*pb.BaseResource `json:"costs"`
	Source        string             `json:"source"`
	RefundVersion int                `json:"refundVersion,omitempty"`
	NonRefundable map[string]int64   `json:"nonRefundable,omitempty"`
	WarriorCopies map[string]int64   `json:"warriorCopies,omitempty"`
	UpgradeCoin   int64              `json:"upgradeCoin,omitempty"`
}

func p5EquipmentProgressed(e *pb.Equipment) bool {
	return e.GetLevel() > 1 || e.GetRefineLv() > 0 || e.GetRefineExp() > 0 || e.GetStar() > 0 || e.GetForgeLevel() > 0
}

func p5WarriorProgressed(w *pb.Warrior) bool {
	return w.GetLevel() > 1 || w.GetTalent() > 0 || len(w.GetTalentBranch()) > 0 || growthValues(w.GetTraining()) != [4]int32{} || trainingPending(w.GetTempTraining()) || w.GetBreached().GetLevel() > 1 || w.GetAwake().GetStage() > 0 || w.GetAwake().GetFuseLevel() > 0 || w.GetAwake().GetFuseExp() > 0 || w.GetCooking() != nil || len(w.GetSkillRefine()) > 0
}

func p5InvestmentFingerprint(current proto.Message) (string, error) {
	copy := proto.Clone(current)
	switch value := copy.(type) {
	case *pb.Equipment:
		value.LastHeroId = nil
	case *pb.Warrior:
		value.SkillOrder = nil
	}
	raw, err := proto.Marshal(copy)
	return p3Hash(raw), err
}

// RecordP5Investment stores actual successful costs and the exact pre-investment baseline.
// Legacy/untracked progress is never estimated; any later untracked mutation makes reset fail closed.
func RecordP5Investment(g *model.GameState, guid string, before, after proto.Message, costs []*pb.BaseResource, operation ...string) error {
	var state p5Investment
	raw := p5KV(g, "p5.invest:"+guid)
	if raw == "" {
		if len(costs) == 0 {
			return nil
		}
		data, err := json.Marshal(before)
		if err != nil {
			return err
		}
		state = p5Investment{RefundVersion: 1, Type: string(before.ProtoReflect().Descriptor().FullName()), Baseline: data, Source: "本地P5：仅退还接入后实际成功扣费，回到首笔扣费前快照；未追溯原服投入"}
	} else if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return err
	}
	if raw != "" {
		expected, err := p5InvestmentFingerprint(before)
		if err != nil || expected != state.Expected {
			return p5Error()
		}
		// 无扣费且实例未变化的历史回执保持投入原字节，避免重复保存重排成本。
		if len(costs) == 0 && proto.Equal(before, after) {
			return nil
		}
	}
	if err := p5ClassifyWarriorInvestment(&state, before, after, costs, operation); err != nil {
		return err
	}
	amounts := map[string]int64{}
	for _, r := range state.Costs {
		amounts[r.GetId()] += int64(r.GetNum())
	}
	for _, r := range costs {
		if r.GetNum() < 1 || r.GetId() == "" || r.GetGuid() != "" {
			return p5Error()
		}
		amounts[r.GetId()] += int64(r.GetNum())
	}
	state.Costs = nil
	keys := make([]string, 0, len(amounts))
	for key := range amounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		n := amounts[key]
		if n < 1 || n > math.MaxInt32 {
			return p5Error()
		}
		state.Costs = append(state.Costs, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(n))})
	}
	expected, err := p5InvestmentFingerprint(after)
	if err != nil {
		return err
	}
	state.Expected = expected
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	p5SetKV(g, "p5.invest:"+guid, string(encoded))
	return nil
}
func p5InvestmentState(g *model.GameState, guid string, current proto.Message) (*p5Investment, error) {
	raw := p5KV(g, "p5.invest:"+guid)
	if raw == "" {
		return nil, p5Error()
	}
	var state p5Investment
	if json.Unmarshal([]byte(raw), &state) != nil || state.Source == "" || state.Type != string(current.ProtoReflect().Descriptor().FullName()) {
		return nil, p5Error()
	}
	expected, err := p5InvestmentFingerprint(current)
	if err != nil || state.Expected != expected {
		return nil, p5Error()
	}
	return &state, nil
}
func (s *GameService) p5Refund(p *pb.Player, g *model.GameState, guid string, current proto.Message, decompose ...bool) (*pb.ResultObject, error) {
	state, err := p5InvestmentState(g, guid, current)
	if err != nil {
		return nil, err
	}
	refunds := state.Costs
	if w, ok := current.(*pb.Warrior); ok {
		refunds, err = s.p5WarriorRefundCosts(g, state, w, decompose...)
		if err != nil {
			return nil, err
		}
	}
	if equipment, ok := current.(*pb.Equipment); ok {
		refunds, err = p5EquipmentRefundCosts(state, equipment)
		if err != nil {
			return nil, err
		}
	}
	var owner, skillOrder string
	var mainID string
	var mainLevel int32
	var mainExp int64
	var gems []string
	switch value := current.(type) {
	case *pb.Equipment:
		owner = value.GetLastHeroId()
		gems = append([]string(nil), value.GetGems()...)
	case *pb.Warrior:
		skillOrder = value.GetSkillOrder()
		if value.GetGuid() == p.GetHeroWarId() {
			mainID, mainLevel, mainExp = value.GetId(), value.GetLevel(), value.GetExp()
		}
	}
	proto.Reset(current)
	if err = json.Unmarshal(state.Baseline, current); err != nil {
		return nil, err
	}
	switch value := current.(type) {
	case *pb.Equipment:
		value.LastHeroId = proto.String(owner)
		// Current slot inventory survives baseline restoration and is detached
		// explicitly by reset/decompose. Old baseline gems are never duplicated.
		value.Gems = gems
	case *pb.Warrior:
		// 主角身份、当前形态与等级来自全局进展，不随可退培养快照回滚。
		if mainID != "" {
			value.Id, value.Level, value.Exp = proto.String(mainID), proto.Int32(mainLevel), proto.Int64(mainExp)
			if skillOrder == "" {
				value.SkillOrder = nil
			}
		}
		if skillOrder != "" {
			order := strings.Split(skillOrder, ",")
			if value.GetAwake().GetStage() < 2 {
				// 重置移除已解除的原觉醒技能，继续保留两个普通技能的相对顺序。
				awake := s.table("Hero", "id", value.GetId())["AwakeSkillId"]
				basic := make([]string, 0, len(order))
				for _, skill := range order {
					if awake == "" || skill != awake {
						basic = append(basic, skill)
					}
				}
				order = basic
			}
			value.SkillOrder = proto.String(strings.Join(order, ","))
			if _, _, err := s.P5BattleSkillTransport(value); err != nil {
				return nil, err
			}
		}
	}
	// GrantFixedReward commits a cloned GameState; restore before it clones the instance.
	refunds, err = s.p8CookingRefundCosts(refunds)
	if err != nil {
		return nil, err
	}
	gain := &pb.ResultObject{}
	if len(refunds) > 0 {
		gain, err = s.p5GrantTrackedRefund(p, g, refunds)
	}
	if err != nil {
		return nil, err
	}
	p5SetKV(g, "p5.invest:"+guid, "")
	return gain, nil
}

// p5GrantTrackedRefund keeps the public reward batch bound while returning every
// instance in a verified investment ledger. The caller commits all batches with
// the target reset in one profile transaction; any capacity failure rolls back.
func (s *GameService) p5GrantTrackedRefund(p *pb.Player, g *model.GameState, refunds []*pb.BaseResource) (*pb.ResultObject, error) {
	batches := [][]*pb.BaseResource{{}}
	room := int32(constUtil.OpenServerActivityMaximumInstances)
	for _, refund := range refunds {
		if refund == nil || refund.GetNum() < 1 || refund.GetGuid() != "" {
			return nil, p5Error()
		}
		if !strings.HasPrefix(refund.GetId(), "hero_") && !strings.HasPrefix(refund.GetId(), "equip_") && !strings.HasPrefix(refund.GetId(), "trea_") && !strings.HasPrefix(refund.GetId(), "pet_") {
			batches[0] = append(batches[0], refund)
			continue
		}
		remaining := refund.GetNum()
		for remaining > 0 {
			if room == 0 {
				batches = append(batches, []*pb.BaseResource{})
				room = int32(constUtil.OpenServerActivityMaximumInstances)
			}
			number := min(remaining, room)
			batches[len(batches)-1] = append(batches[len(batches)-1], &pb.BaseResource{Id: refund.Id, Num: proto.Int32(number)})
			remaining -= number
			room -= number
		}
	}
	result := &pb.ResultObject{}
	for _, batch := range batches {
		gain, err := s.p8GrantCookingRefund(p, g, batch)
		if err != nil {
			return nil, err
		}
		redMergeGain(result, gain)
	}
	return result, nil
}

func (s *GameService) p5WarriorReset(ctx context.Context, id int64, b *pb.CSWarriorReset, stamp int64) ([]proto.Message, error) {
	out := &pb.SCWarriorReset{}
	return s.p5Mutation(ctx, id, "warrior-reset", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		w, err := p5Warrior(g, b.GetGuid())
		if err != nil {
			return err
		}
		isMain := w.GetGuid() == p.GetHeroWarId()
		if isMain {
			class := s.table("Hero", "id", w.GetId())["heroClass"]
			if (class != "100" && class != "101") || p5KV(g, "p5.lock:"+w.GetGuid()) == "1" || p5KV(g, "p5.dispatch:"+w.GetGuid()) != "" {
				return p5Error()
			}
		} else if err := P5GuardInstance(p, g, b.GetGuid()); err != nil {
			return err
		}
		costs, err := p3Resources(s.table("GameConfigure", "StaticName", "WarriorRebuildCost")["NameValue"])
		if err != nil || len(costs) != 1 || costs[0].GetId() != "diamond" {
			return p5Error()
		}
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		out.Cost = costs
		heroID := w.GetId()
		var baseFragments []*pb.BaseResource
		if !isMain {
			baseFragments, err = s.p5WarriorFragmentReward(heroID, 1)
			if err != nil {
				return err
			}
		}
		gain := &pb.ResultObject{}
		if p5KV(g, "p5.invest:"+w.GetGuid()) != "" {
			gain, err = s.p5Refund(p, g, w.GetGuid(), w)
			if err != nil {
				return err
			}
		} else if !p5WarriorResetPristine(w, isMain) {
			return p5Error()
		}
		if isMain {
			// 退还资源会克隆存档，重取主角后回传，保留阵容和穿戴引用。
			main, e := p5Warrior(g, b.GetGuid())
			if e != nil {
				return e
			}
			out.Warriors = []*pb.Warrior{proto.Clone(main).(*pb.Warrior)}
		} else {
			// 伙伴以初始全碎片返还，原GUID退休，避免整卡和碎片同时存在。
			filtered := g.Warriors[:0]
			for _, unit := range g.Warriors {
				if unit.GetGuid() != b.GetGuid() {
					filtered = append(filtered, unit)
				}
			}
			g.Warriors = filtered
			base, e := s.p5GrantTrackedRefund(p, g, baseFragments)
			if e != nil {
				return e
			}
			redMergeGain(gain, base)
			out.Cost = append(out.Cost, &pb.BaseResource{Id: proto.String(heroID), Guid: b.Guid, Num: proto.Int32(1)})
		}
		out.Get = gain
		return s.RecordP1Event(tx, id, fmt.Sprintf("p5:warrior-reset:%d", stamp), constUtil.P1EventDiamondSpent, int64(costs[0].GetNum()), LocalPlayNow(ctx))
	})
}
func (s *GameService) p5EquipmentReset(ctx context.Context, id int64, b *pb.CSEquipReBuild, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipReBuild{}
	return s.p5Mutation(ctx, id, "equipment-reset", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := P5GuardInstance(p, g, b.GetGuid()); err != nil {
			return err
		}
		equip, err := s.equipmentRecord(g, b.GetGuid())
		if err != nil {
			return err
		}
		costs, err := p3Resources(s.table("GameConfigure", "StaticName", "EquipmentRebuildCost")["NameValue"])
		if err != nil || len(costs) != 1 || costs[0].GetId() != "diamond" {
			return p5Error()
		}
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		out.Cost = costs
		equipmentID := equip.GetEquipId()
		baseFragments, err := s.p5WarriorFragmentReward(equipmentID, 1)
		if err != nil {
			return err
		}
		gain := &pb.ResultObject{}
		if p5KV(g, "p5.invest:"+equip.GetGuid()) != "" {
			gain, err = s.p5Refund(p, g, equip.GetGuid(), equip)
			if err != nil {
				return err
			}
		} else if p5EquipmentProgressed(equip) {
			return p5Error()
		}
		// Fixed refunds clone GameState; resolve the restored owned instance
		// again before returning its current physical gem slots.
		equip, err = s.equipmentRecord(g, b.GetGuid())
		if err != nil {
			return err
		}
		gems, err := s.p8RecycleEquipmentGems(p, g, equip)
		if err != nil {
			return err
		}
		redMergeGain(gain, gems)
		filtered := g.Equipment[:0]
		for _, owned := range g.Equipment {
			if owned.GetGuid() != b.GetGuid() {
				filtered = append(filtered, owned)
			}
		}
		g.Equipment = filtered
		base, err := s.p5GrantTrackedRefund(p, g, baseFragments)
		if err != nil {
			return err
		}
		redMergeGain(gain, base)
		out.Cost = append(out.Cost, &pb.BaseResource{Id: proto.String(equipmentID), Guid: b.Guid, Num: proto.Int32(1)})
		out.Get = gain
		return s.RecordP1Event(tx, id, fmt.Sprintf("p5:equipment-reset:%d", stamp), constUtil.P1EventDiamondSpent, int64(costs[0].GetNum()), LocalPlayNow(ctx))
	})
}

func (s *GameService) p5WarriorRecycle(ctx context.Context, id int64, guids []string, decompose bool, stamp int64, b proto.Message) ([]proto.Message, error) {
	var out proto.Message = &pb.SCWarriorSell{}
	op := "warrior-sell"
	if decompose {
		out = &pb.SCWarriorDecompose{}
		op = "warrior-decompose"
	}
	return s.p5Mutation(ctx, id, op, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := p5ListValid(guids); err != nil {
			return err
		}
		gain := &pb.ResultObject{}
		costs := []*pb.BaseResource{}
		seen := map[string]bool{}
		var coin int64
		decomposition := []*pb.BaseResource{}
		for _, guid := range guids {
			if err := P5GuardInstance(p, g, guid); err != nil {
				return err
			}
			w, err := p5Warrior(g, guid)
			if err != nil {
				return err
			}
			row := s.table("Hero", "id", w.GetId())
			if row == nil || row["heroClass"] == "100" || row["heroClass"] == "101" {
				return p5Error()
			}
			// Untracked progressed instances cannot be silently destroyed with base-only returns.
			progressed := p5WarriorProgressed(w)
			if p5KV(g, "p5.invest:"+guid) != "" {
				refund, err := s.p5Refund(p, g, guid, w, decompose)
				if err != nil {
					return err
				}
				redMergeGain(gain, refund)
				if p5WarriorProgressed(w) {
					return p5Error()
				}
			} else if progressed {
				return p5Error()
			}
			if decompose {
				// 基础卡及同名培养卡分别按用户批准的品质规则换光，出售保持金币合同。
				base, err := s.p5WarriorDecomposeReward(w.GetId(), 1)
				if err != nil {
					return err
				}
				decomposition = append(decomposition, base...)
			} else {
				price, err := strconv.ParseInt(row["selloutPrice"], 10, 64)
				if err != nil || price < 0 || price > math.MaxInt32-coin {
					return p5Error()
				}
				coin += price
			}
			costs = append(costs, &pb.BaseResource{Id: proto.String(w.GetId()), Guid: proto.String(guid), Num: proto.Int32(1)})
			seen[guid] = true
		}
		filtered := g.Warriors[:0]
		for _, w := range g.Warriors {
			if !seen[w.GetGuid()] {
				filtered = append(filtered, w)
			}
		}
		g.Warriors = filtered
		if len(decomposition) > 0 {
			reward, err := s.p5GrantTrackedRefund(p, g, decomposition)
			if err != nil {
				return err
			}
			redMergeGain(gain, reward)
		}
		if coin > 0 {
			reward, err := s.GrantFixedReward(p, g, fmt.Sprintf("coin,%d", coin))
			if err != nil {
				return err
			}
			redMergeGain(gain, reward)
		}
		switch result := out.(type) {
		case *pb.SCWarriorSell:
			result.Cost = costs
			result.Get = gain
		case *pb.SCWarriorDecompose:
			result.Cost = costs
			result.Get = gain
		}
		return nil
	})
}

func (s *GameService) p5EquipmentSell(ctx context.Context, id int64, b *pb.CSEquipmentSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipmentSell{}
	return s.p5Mutation(ctx, id, "equipment-sell", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if len(b.GetCost()) < 1 || len(b.GetCost()) > 100 {
			return p5Error()
		}
		seen := map[string]bool{}
		soldTreasures := map[string]bool{}
		var coin int64
		gain := &pb.ResultObject{}
		for _, cost := range b.GetCost() {
			guid := cost.GetGuid()
			if seen[guid] || cost.GetNum() != 1 {
				return p5Error()
			}
			if err := P5GuardInstance(p, g, guid); err != nil {
				return err
			}
			// Original BagType.Equipment sells low-quality treasures for cash.
			// Investment liquidation stays separate from any reset/refund path.
			if strings.HasPrefix(cost.GetId(), "trea_") {
				price, err := s.p5NativeTreasureSalePrice(p, g, cost)
				if err != nil {
					return err
				}
				if price > math.MaxInt32-coin {
					return p5Error()
				}
				coin += price
				seen[guid], soldTreasures[guid] = true, true
				out.Cost = append(out.Cost, proto.Clone(cost).(*pb.BaseResource))
				continue
			}
			equip, err := s.equipmentRecord(g, guid)
			if err != nil || equip.GetEquipId() != cost.GetId() {
				return p5Error()
			}
			// Original four-hole arrays retain empty strings after unmounting.
			// Only a occupied slot blocks sale; malformed extra slots still fail.
			if len(equip.GetGems()) > 4 {
				return p5Error()
			}
			for _, gem := range equip.GetGems() {
				if gem != "" {
					return p5Error()
				}
			}
			// 先退还可追溯的实际投入，再按原始实例出售；旧培养保持拒绝销毁。
			if p5EquipmentProgressed(equip) {
				refund, err := s.p5Refund(p, g, guid, equip)
				if err != nil {
					return err
				}
				redMergeGain(gain, refund)
				if p5EquipmentProgressed(equip) {
					return p5Error()
				}
			}
			row := s.table("Equip", "id", equip.GetEquipId())
			if row == nil {
				return p5Error()
			}
			price := int64(tableNumber(row, "sellPrice"))
			if price <= 0 {
				price = int64(equip.GetCoinPrice())
			}
			if price < 0 || price > math.MaxInt32-coin {
				return p5Error()
			}
			coin += price
			seen[guid] = true
			out.Cost = append(out.Cost, proto.Clone(cost).(*pb.BaseResource))
		}
		// All selected instances have validated. Settle tracked treasure costs
		// before deletion; the enclosing transaction rolls back on grant failure.
		for _, cost := range b.GetCost() {
			if soldTreasures[cost.GetGuid()] {
				if err := s.p5SettleNativeTreasureSale(p, g, cost); err != nil {
					return err
				}
			}
		}
		filtered := g.Equipment[:0]
		for _, equip := range g.Equipment {
			if !seen[equip.GetGuid()] {
				filtered = append(filtered, equip)
			}
		}
		g.Equipment = filtered
		// Delete the selected treasure GUIDs only after the full mixed batch validates.
		treasures := g.Treasures[:0]
		for _, treasure := range g.Treasures {
			if !soldTreasures[treasure.GetGuid()] {
				treasures = append(treasures, treasure)
			}
		}
		g.Treasures = treasures
		if coin > 0 {
			reward, err := s.GrantFixedReward(p, g, fmt.Sprintf("coin,%d", coin))
			if err != nil {
				return err
			}
			redMergeGain(gain, reward)
		}
		out.Get = gain
		return nil
	})
}
func (s *GameService) p5TreasureSell(ctx context.Context, id int64, b *pb.CSTreasureSell, stamp int64) ([]proto.Message, error) {
	out := &pb.SCTreasureSell{}
	return s.p5Mutation(ctx, id, "treasure-sell", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := p5ListValid(b.GetGuids()); err != nil {
			return err
		}
		seen := map[string]bool{}
		var coin int64
		gain := &pb.ResultObject{}
		for _, guid := range b.GetGuids() {
			if err := P5GuardInstance(p, g, guid); err != nil {
				return err
			}
			var found *pb.Treasure
			for _, t := range g.Treasures {
				if t.GetGuid() == guid {
					found = t
				}
			}
			if found == nil {
				return p5Error()
			}
			// 饰品成长与消耗材料共享账本，出售前恢复首笔实际扣费前状态。
			if found.GetStrengthLevel() > 1 || found.GetRefineLevel() > 0 || found.GetCurrentExp() > 0 {
				refund, err := s.p5Refund(p, g, guid, found)
				if err != nil {
					return err
				}
				redMergeGain(gain, refund)
				if found.GetStrengthLevel() > 1 || found.GetRefineLevel() > 0 || found.GetCurrentExp() > 0 {
					return p5Error()
				}
			}
			row := s.table("Treasure", "id", found.GetTreasureId())
			price := int64(tableNumber(row, "sellPrice"))
			if row == nil || price <= 0 || price > math.MaxInt32-coin {
				return p5Error()
			}
			coin += price
			seen[guid] = true
			out.Cost = append(out.Cost, &pb.BaseResource{Id: proto.String(found.GetTreasureId()), Guid: proto.String(guid), Num: proto.Int32(1)})
		}
		filtered := g.Treasures[:0]
		for _, t := range g.Treasures {
			if !seen[t.GetGuid()] {
				filtered = append(filtered, t)
			}
		}
		g.Treasures = filtered
		reward, err := s.GrantFixedReward(p, g, fmt.Sprintf("coin,%d", coin))
		if err != nil {
			return err
		}
		redMergeGain(gain, reward)
		out.Get = gain
		return err
	})
}
