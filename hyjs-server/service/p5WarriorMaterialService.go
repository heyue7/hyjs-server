package service

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p5WarriorMaterialAudit struct {
	Policy     string          `json:"policy"`
	TargetGUID string          `json:"targetGuid"`
	Before     json.RawMessage `json:"before"`
	Investment json.RawMessage `json:"investment"`
}

// p5WarriorMaterialCosts connects the original manual material consumer to
// the existing reset ledger. The enclosing save still checks formation, help,
// lock and expedition ownership. Other consumers retain their original guards.
func (s *GameService) p5WarriorMaterialCosts(p *pb.Player, beforeG, g *model.GameState, material *pb.Warrior, targetGUID string, request proto.Message) ([]*pb.BaseResource, error) {
	strengthen, ok := request.(*pb.CSWarriorStrengthen)
	if !ok {
		if p5WarriorProgressed(material) {
			return nil, p5Error()
		}
		return nil, nil
	}
	selected := false
	for _, cost := range strengthen.GetCost() {
		selected = selected || cost.GetGuid() == material.GetGuid() && cost.GetId() == material.GetId()
	}
	if targetGUID == "" || strengthen.GetGuid() != targetGUID || material.GetGuid() == targetGUID || !selected {
		return nil, p5Error()
	}
	if err := P5GuardInstance(p, beforeG, material.GetGuid()); err != nil {
		return nil, err
	}
	// Saved growth is passive material state, while a training preview is an
	// unfinished user choice and must be confirmed or discarded before use.
	if trainingPending(material.GetTempTraining()) {
		return nil, p5Error()
	}
	raw := p5KV(beforeG, "p5.invest:"+material.GetGuid())
	if raw == "" {
		if !p5WarriorMaterialPristine(material) {
			return nil, p5Error()
		}
		return nil, nil
	}
	if p5KV(g, "p5.invest:"+material.GetGuid()) != raw || p5KV(g, constUtil.P5WarriorMaterialAuditPrefix+material.GetGuid()) != "" {
		return nil, p5Error()
	}
	state, err := p5InvestmentState(beforeG, material.GetGuid(), material)
	if err != nil || len(state.Costs) == 0 {
		return nil, p5Error()
	}
	var baseline pb.Warrior
	if json.Unmarshal(state.Baseline, &baseline) != nil || baseline.GetGuid() != material.GetGuid() || baseline.GetId() != material.GetId() || !p5WarriorMaterialPristine(&baseline) {
		return nil, p5Error()
	}
	seen := map[string]bool{}
	costs := make([]*pb.BaseResource, 0, len(state.Costs))
	for _, cost := range state.Costs {
		if cost.GetId() == "" || cost.GetGuid() != "" || cost.GetNum() < 1 || seen[cost.GetId()] {
			return nil, p5Error()
		}
		seen[cost.GetId()] = true
		// Validate one unit; existing accumulated-cost bounds apply downstream.
		supported, e := s.openServerRewardSupported(cost.GetId() + ",1")
		if e != nil || !supported {
			return nil, p5Error()
		}
		costs = append(costs, proto.Clone(cost).(*pb.BaseResource))
	}
	// Verify the existing final refund path without paying it now. Permanent
	// dishes stay as original spent units until the final target's aggregate
	// ledger applies its existing 50% round-down once, including nested donors.
	if _, err := s.p8CookingRefundCosts(costs); err != nil {
		return nil, err
	}
	before, err := json.Marshal(material)
	if err != nil {
		return nil, err
	}
	audit, err := json.Marshal(p5WarriorMaterialAudit{Policy: constUtil.P5WarriorMaterialPolicy, TargetGUID: targetGUID, Before: before, Investment: json.RawMessage(raw)})
	if err != nil {
		return nil, err
	}
	// Transfer once in the same profile transaction. Reset later returns fresh
	// original resource units; the consumed donor GUID never becomes refundable.
	p5SetKV(g, constUtil.P5WarriorMaterialAuditPrefix+material.GetGuid(), string(audit))
	p5SetKV(g, "p5.invest:"+material.GetGuid(), "")
	return costs, nil
}

// p5WarriorMaterialPristine distinguishes intrinsic initial fields from prior
// investment. Partial breach progress/time/cost records and an awakened skill
// cannot hide in an apparently level-one baseline. Initial Breached.level1,
// empty Training/Awake messages and an ordinary skill order remain legitimate.
func p5WarriorMaterialPristine(w *pb.Warrior) bool {
	return w.GetLevel() == 1 && w.GetExp() == 0 && w.GetTalent() == 0 && !p5WarriorProgressed(w) && w.GetTraining().GetState() == 0 && (w.GetBreached().GetLevel() == 0 || w.GetBreached().GetLevel() == 1) && w.GetBreached().GetExp() == 0 && w.GetBreached().GetLastAddExpTime() == "" && w.GetBreached().GetCostRecord() == "" && w.GetAwake().GetStage() == 0 && w.GetAwake().GetFuseLevel() == 0 && w.GetAwake().GetFuseExp() == 0 && w.GetAwakenSkill() == ""
}
