package service

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// p5NativeTreasureSaleSettlement retains the sold state and exact paid costs.
// Costs are liquidated with the instance, with no additional resource refund.
type p5NativeTreasureSaleSettlement struct {
	Policy     string          `json:"policy"`
	Cash       int64           `json:"cash"`
	Before     json.RawMessage `json:"before"`
	Investment json.RawMessage `json:"investment,omitempty"`
}

// p5NativeTreasureSalePrice validates the original BagType.Equipment mixed
// sale entry. It only quotes; the caller removes selected GUIDs and grants
// coins once in the existing equipment-sell transaction and response contract.
func (s *GameService) p5NativeTreasureSalePrice(p *pb.Player, g *model.GameState, cost *pb.BaseResource) (int64, error) {
	if p == nil || g == nil || cost == nil || cost.GetNum() != 1 || cost.GetGuid() == "" || !strings.HasPrefix(cost.GetId(), "trea_") {
		return 0, p5Error()
	}
	if p5KV(g, "p5.native-sale:"+cost.GetGuid()) != "" {
		return 0, p5Error()
	}
	if err := P5GuardInstance(p, g, cost.GetGuid()); err != nil {
		return 0, err
	}
	owned, err := p5OwnedInstance(g, cost.GetGuid())
	if err != nil {
		return 0, err
	}
	treasure, ok := owned.(*pb.Treasure)
	if !ok || treasure.GetTreasureId() != cost.GetId() {
		return 0, p5Error()
	}
	if _, err = s.treasureRecord(g, cost.GetGuid()); err != nil {
		return 0, err
	}
	row := s.table("Treasure", "id", treasure.GetTreasureId())
	quality, err := strconv.ParseInt(row["quality"], 10, 32)
	if err != nil || quality < 1 || quality > 18 {
		return 0, errors.New(constUtil.P5NativeTreasureSaleProtected)
	}
	// Grown instances require an intact, wholly tracked origin. An old partial
	// baseline cannot be used to silently dispose of untracked paid progress.
	tracked := p5KV(g, "p5.invest:"+treasure.GetGuid()) != ""
	progressed := treasure.GetStrengthLevel() > 1 || treasure.GetRefineLevel() > 0 || treasure.GetCurrentExp() > 0
	if progressed || tracked {
		state, e := p5InvestmentState(g, treasure.GetGuid(), treasure)
		if e != nil || len(state.Costs) == 0 {
			return 0, errors.New(constUtil.P5NativeTreasureSaleProtected)
		}
		var baseline pb.Treasure
		if json.Unmarshal(state.Baseline, &baseline) != nil || baseline.GetGuid() != treasure.GetGuid() || baseline.GetTreasureId() != treasure.GetTreasureId() || baseline.GetStrengthLevel() != 1 || baseline.GetRefineLevel() != 0 || baseline.GetCurrentExp() != 0 {
			return 0, errors.New(constUtil.P5NativeTreasureSaleProtected)
		}
		seen := map[string]bool{}
		for _, paid := range state.Costs {
			if paid.GetId() == "" || paid.GetGuid() != "" || paid.GetNum() < 1 || seen[paid.GetId()] {
				return 0, p5Error()
			}
			seen[paid.GetId()] = true
			// Validate the resource identity without imposing a reward-batch cap:
			// cumulative paid instance counts may exceed one 100-item grant.
			supported, e := s.openServerRewardSupported(paid.GetId() + ",1")
			if e != nil || !supported {
				return 0, errors.New(constUtil.P5NativeTreasureSaleProtected)
			}
		}
	}
	price, err := strconv.ParseInt(row["sellPrice"], 10, 64)
	if err != nil || price < 1 || price > math.MaxInt32 {
		return 0, p5Error()
	}
	// Original UITreasure.GetTreasureSellCoin: level EXP + remainder + base.
	// Refine investments remain in the audit but do not add another cash term.
	for level := int32(1); level < treasure.GetStrengthLevel(); level++ {
		exp, e := s.treasureLevelExp(treasure, level)
		if e != nil || exp > math.MaxInt32-price {
			return 0, p5Error()
		}
		price += exp
	}
	if int64(treasure.GetCurrentExp()) > math.MaxInt32-price {
		return 0, p5Error()
	}
	price += int64(treasure.GetCurrentExp())
	return price, nil
}

// p5SettleNativeTreasureSale runs only in the existing mixed-sale transaction,
// after every selected GUID validates and before removal/payment. Failed grants
// roll the audit and active investment change back with the whole profile.
func (s *GameService) p5SettleNativeTreasureSale(p *pb.Player, g *model.GameState, cost *pb.BaseResource) error {
	price, err := s.p5NativeTreasureSalePrice(p, g, cost)
	if err != nil {
		return err
	}
	current, err := s.treasureRecord(g, cost.GetGuid())
	if err != nil {
		return err
	}
	before, err := json.Marshal(current)
	if err != nil {
		return err
	}
	settlement := p5NativeTreasureSaleSettlement{Policy: constUtil.P5NativeTreasureSalePolicy, Cash: price, Before: before}
	if raw := p5KV(g, "p5.invest:"+cost.GetGuid()); raw != "" {
		settlement.Investment = json.RawMessage(raw)
	}
	encoded, err := json.Marshal(settlement)
	if err != nil {
		return err
	}
	p5SetKV(g, "p5.native-sale:"+cost.GetGuid(), string(encoded))
	p5SetKV(g, "p5.invest:"+cost.GetGuid(), "")
	return nil
}
