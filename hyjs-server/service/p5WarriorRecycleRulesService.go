package service

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p5ClassifyWarriorInvestment preserves the original purpose of paid resources.
// Existing aggregate ledgers retain version zero and are never relabelled as complete.
func p5ClassifyWarriorInvestment(state *p5Investment, before, after proto.Message, costs []*pb.BaseResource, operation []string) error {
	if len(operation) == 0 {
		return nil
	}
	if equipment, ok := before.(*pb.Equipment); ok {
		return p5ClassifyEquipmentInvestment(state, equipment, after, costs, operation[0])
	}
	if _, ok := before.(*pb.Warrior); !ok {
		return nil
	}
	if state.NonRefundable == nil {
		state.NonRefundable = map[string]int64{}
	}
	if state.WarriorCopies == nil {
		state.WarriorCopies = map[string]int64{}
	}
	for _, cost := range costs {
		key, n := cost.GetId(), int64(cost.GetNum())
		switch operation[0] {
		case "CSWarriorTraining", "CSWarriorTrainingReplace":
			if key == "coin" || key == "diamond" {
				state.NonRefundable[key] += n
			}
		case "CSWarriorTalent":
			if key == "coin" {
				state.NonRefundable[key] += n
			}
		}
		if operation[0] == "CSWarriorStrengthen" && key == "coin" {
			state.UpgradeCoin += n
		}
		if (operation[0] == "CSWarriorTalent" || operation[0] == "CSWarriorAwake") && strings.HasPrefix(key, "hero_") {
			state.WarriorCopies[key] += n
		}
	}
	return nil
}

func p5WarriorResetPristine(w *pb.Warrior, main bool) bool {
	if !main {
		return p5WarriorMaterialPristine(w)
	}
	copy := proto.Clone(w).(*pb.Warrior)
	copy.Level, copy.Exp = proto.Int32(1), proto.Int64(0)
	return p5WarriorMaterialPristine(copy)
}

func p5ClassifyEquipmentInvestment(state *p5Investment, before *pb.Equipment, after proto.Message, costs []*pb.BaseResource, operation string) error {
	if operation != "CSEquipForge" {
		return nil
	}
	current, ok := after.(*pb.Equipment)
	if !ok {
		return p5Error()
	}
	if state.NonRefundable == nil {
		state.NonRefundable = map[string]int64{}
	}
	for _, cost := range costs {
		// Original recycling returns successful forge fragments; failed attempts
		// and forge coins are separate from refundable strengthening coins.
		if cost.GetId() == "coin" || (strings.HasPrefix(cost.GetId(), "frag_equip_") && current.GetForgeLevel() <= before.GetForgeLevel()) {
			state.NonRefundable[cost.GetId()] += int64(cost.GetNum())
		}
	}
	return nil
}

func p5EquipmentRefundCosts(state *p5Investment, equipment *pb.Equipment) ([]*pb.BaseResource, error) {
	if state.RefundVersion == 0 {
		var baseline pb.Equipment
		if json.Unmarshal(state.Baseline, &baseline) != nil {
			return nil, p5Error()
		}
		if equipment.GetForgeLevel() != baseline.GetForgeLevel() {
			return nil, errors.New(constUtil.P5EquipmentLegacyRefundMissing)
		}
		for _, cost := range state.Costs {
			if strings.HasPrefix(cost.GetId(), "frag_equip_") {
				return nil, errors.New(constUtil.P5EquipmentLegacyRefundMissing)
			}
		}
	}
	refunds := make([]*pb.BaseResource, 0, len(state.Costs))
	for _, cost := range state.Costs {
		n := int64(cost.GetNum()) - state.NonRefundable[cost.GetId()]
		if n < 0 || n > math.MaxInt32 {
			return nil, p5Error()
		}
		if n > 0 {
			refunds = append(refunds, &pb.BaseResource{Id: cost.Id, Num: proto.Int32(int32(n))})
		}
	}
	return refunds, nil
}

// p5InheritWarriorRefundMetadata carries donor exclusions and same-name copies
// with the existing aggregate costs. The new strengthening fee already includes
// the donor's inherited EXP, so donor fees must not create that EXP a second time.
func p5InheritWarriorRefundMetadata(g *model.GameState, guid string, current proto.Message, donors []*p5Investment) error {
	if len(donors) == 0 {
		return nil
	}
	state, err := p5InvestmentState(g, guid, current)
	if err != nil {
		return err
	}
	if state.NonRefundable == nil {
		state.NonRefundable = map[string]int64{}
	}
	if state.WarriorCopies == nil {
		state.WarriorCopies = map[string]int64{}
	}
	for _, donor := range donors {
		if donor.RefundVersion != 1 {
			state.RefundVersion = 0
		}
		for key, n := range donor.NonRefundable {
			state.NonRefundable[key] += n
		}
		for key, n := range donor.WarriorCopies {
			state.WarriorCopies[key] += n
		}
		for _, cost := range donor.Costs {
			if cost.GetId() == "coin" {
				state.UpgradeCoin -= int64(cost.GetNum())
			}
		}
	}
	if state.UpgradeCoin < 0 {
		return p5Error()
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	p5SetKV(g, "p5.invest:"+guid, string(encoded))
	return nil
}

func p5CarryWarriorBreachExclusions(g *model.GameState, guid string, investment *p5Investment) error {
	var ledger p5BreachRefund
	raw := p5KV(g, constUtil.P5WarriorBreachRefundPrefix+guid)
	if raw == "" {
		return nil
	}
	if json.Unmarshal([]byte(raw), &ledger) != nil {
		return p5Error()
	}
	if investment.NonRefundable == nil {
		investment.NonRefundable = map[string]int64{}
	}
	for key, n := range ledger.Pending {
		investment.NonRefundable[key] += n
	}
	for key, n := range ledger.Expired {
		investment.NonRefundable[key] += n
	}
	return nil
}

func (s *GameService) p5WarriorFragmentReward(heroID string, copies int64) ([]*pb.BaseResource, error) {
	for _, row := range s.Tables["Frag"] {
		if row["target"] != heroID {
			continue
		}
		n, err := strconv.ParseInt(row["num"], 10, 64)
		if err != nil || copies < 1 || n < 1 || n > math.MaxInt32/copies {
			return nil, p5Error()
		}
		return []*pb.BaseResource{{Id: proto.String(row["id"]), Num: proto.Int32(int32(n * copies))}}, nil
	}
	return nil, p5Error()
}

type p5BreachRefund struct {
	Pending map[string]int64 `json:"pending"`
	Expired map[string]int64 `json:"expired"`
}

// p5TrackBreachRefund retires spent crystals only when their current stage completes.
// An expired progress bar remains excluded, even after later stages complete.
func p5TrackBreachRefund(g *model.GameState, guid string, costs []*pb.BaseResource, completed, expired bool) error {
	var ledger p5BreachRefund
	raw := p5KV(g, constUtil.P5WarriorBreachRefundPrefix+guid)
	if raw != "" && json.Unmarshal([]byte(raw), &ledger) != nil {
		return p5Error()
	}
	if ledger.Pending == nil {
		ledger.Pending = map[string]int64{}
	}
	if ledger.Expired == nil {
		ledger.Expired = map[string]int64{}
	}
	if expired {
		for key, n := range ledger.Pending {
			ledger.Expired[key] += n
		}
		ledger.Pending = map[string]int64{}
	}
	for _, cost := range costs {
		ledger.Pending[cost.GetId()] += int64(cost.GetNum())
	}
	if completed {
		ledger.Pending = map[string]int64{}
	}
	encoded, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	p5SetKV(g, constUtil.P5WarriorBreachRefundPrefix+guid, string(encoded))
	return nil
}

func (s *GameService) p5WarriorRefundCosts(g *model.GameState, state *p5Investment, w *pb.Warrior, decompose ...bool) ([]*pb.BaseResource, error) {
	if state.RefundVersion != 1 {
		if len(state.Costs) == 0 {
			return nil, nil
		}
		return nil, errors.New(constUtil.P5WarriorLegacyRefundMissing)
	}
	amounts := map[string]int64{}
	for _, cost := range state.Costs {
		// Consumed upgrade donors become EXP; talent/awakening copies become fragments below.
		if !strings.HasPrefix(cost.GetId(), "hero_") {
			amounts[cost.GetId()] += int64(cost.GetNum())
		}
	}
	for key, n := range state.NonRefundable {
		amounts[key] -= n
	}
	var breach p5BreachRefund
	if raw := p5KV(g, constUtil.P5WarriorBreachRefundPrefix+w.GetGuid()); raw != "" {
		if json.Unmarshal([]byte(raw), &breach) != nil {
			return nil, p5Error()
		}
		for key, n := range breach.Pending {
			amounts[key] -= n
		}
		for key, n := range breach.Expired {
			amounts[key] -= n
		}
	}
	for hero, n := range state.WarriorCopies {
		var fragments []*pb.BaseResource
		var err error
		if len(decompose) > 0 && decompose[0] {
			fragments, err = s.p5WarriorDecomposeReward(hero, n)
		} else {
			fragments, err = s.p5WarriorFragmentReward(hero, n)
		}
		if err != nil {
			return nil, err
		}
		for _, r := range fragments {
			amounts[r.GetId()] += int64(r.GetNum())
		}
	}
	// The original strength fee records all paid EXP, including capped overflow.
	factor, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "HeroStrengthCostExpToCoin")["NameValue"], 10, 64)
	if err != nil || factor < 1 || state.UpgradeCoin%factor != 0 {
		return nil, p5Error()
	}
	experience := state.UpgradeCoin / factor
	// item_066 contains five original 20,000-EXP crystals; residual EXP below 5,000 is discarded.
	amounts["item_066"] += experience / 100000
	amounts["hero_exp_002"] += experience % 100000 / 5000
	keys := make([]string, 0, len(amounts))
	for key := range amounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []*pb.BaseResource{}
	for _, key := range keys {
		n := amounts[key]
		if n < 0 || n > math.MaxInt32 {
			return nil, p5Error()
		}
		if n > 0 {
			result = append(result, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(n))})
		}
	}
	return result, nil
}

// p5WarriorDecomposeReward 只按已批准的基础卡品质折算；培养投入由原分类账另行退还。
func (s *GameService) p5WarriorDecomposeReward(heroID string, copies int64) ([]*pb.BaseResource, error) {
	var light, soul int64
	switch s.table("Hero", "id", heroID)["quality"] {
	case "12":
		light = constUtil.P5DecomposeGreenLight
	case "13":
		light = constUtil.P5DecomposeBlueLight
	case "18":
		light = constUtil.P5DecomposePurpleLight
	case "20":
		light = constUtil.P5DecomposeOrangeLight
	case "23":
		light, soul = constUtil.P5DecomposeRedLight, constUtil.P5DecomposeRedSoul
	default:
		return nil, p5Error()
	}
	if copies < 1 || light > math.MaxInt32/copies || soul > math.MaxInt32/copies {
		return nil, p5Error()
	}
	result := []*pb.BaseResource{{Id: proto.String("light"), Num: proto.Int32(int32(light * copies))}}
	if soul > 0 {
		result = append(result, &pb.BaseResource{Id: proto.String("item_020"), Num: proto.Int32(int32(soul * copies))})
	}
	return result, nil
}
