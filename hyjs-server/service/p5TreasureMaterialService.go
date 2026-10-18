package service

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p5TreasureMaterialAudit struct {
	Policy     string          `json:"policy"`
	TargetGUID string          `json:"targetGuid"`
	Before     json.RawMessage `json:"before"`
	Investment json.RawMessage `json:"investment"`
}

// p5TreasureMaterialCosts connects the original inherited-EXP consumer to the
// existing exact-cost reset ledger. It never grants a refund during consumption.
// Other consumers keep their existing pristine-only rule.
func (s *GameService) p5TreasureMaterialCosts(p *pb.Player, beforeG, g *model.GameState, material *pb.Treasure, targetGUID string, request proto.Message) ([]*pb.BaseResource, error) {
	strengthen, ok := request.(*pb.CSTreasureStrengthen)
	progressed := material.GetStrengthLevel() > 1 || material.GetRefineLevel() > 0 || material.GetCurrentExp() > 0
	if !ok {
		if progressed {
			return nil, p5Error()
		}
		return nil, nil
	}
	selected := false
	for _, guid := range strengthen.GetGuids() {
		selected = selected || guid == material.GetGuid()
	}
	if targetGUID == "" || strengthen.GetTargetId() != targetGUID || material.GetGuid() == targetGUID || !selected || material.GetRefineLevel() != 0 {
		return nil, p5Error()
	}
	if err := P5GuardInstance(p, beforeG, material.GetGuid()); err != nil {
		return nil, err
	}
	raw := p5KV(beforeG, "p5.invest:"+material.GetGuid())
	if raw == "" {
		if progressed {
			return nil, p5Error()
		}
		return nil, nil
	}
	if p5KV(g, "p5.invest:"+material.GetGuid()) != raw || p5KV(g, constUtil.P5TreasureMaterialAuditPrefix+material.GetGuid()) != "" {
		return nil, p5Error()
	}
	state, err := p5InvestmentState(beforeG, material.GetGuid(), material)
	if err != nil || len(state.Costs) == 0 {
		return nil, p5Error()
	}
	var baseline pb.Treasure
	if json.Unmarshal(state.Baseline, &baseline) != nil || baseline.GetGuid() != material.GetGuid() || baseline.GetTreasureId() != material.GetTreasureId() || baseline.GetStrengthLevel() != 1 || baseline.GetRefineLevel() != 0 || baseline.GetCurrentExp() != 0 {
		return nil, p5Error()
	}
	seen := map[string]bool{}
	costs := make([]*pb.BaseResource, 0, len(state.Costs))
	for _, cost := range state.Costs {
		if cost.GetId() == "" || cost.GetGuid() != "" || cost.GetNum() < 1 || seen[cost.GetId()] {
			return nil, p5Error()
		}
		seen[cost.GetId()] = true
		// Validate identity using one unit; accumulated actual costs can exceed
		// a single batch without changing the existing int32 investment limit.
		supported, e := s.openServerRewardSupported(cost.GetId() + ",1")
		if e != nil || !supported {
			return nil, p5Error()
		}
		costs = append(costs, proto.Clone(cost).(*pb.BaseResource))
	}
	before, err := json.Marshal(material)
	if err != nil {
		return nil, err
	}
	audit, err := json.Marshal(p5TreasureMaterialAudit{Policy: constUtil.P5TreasureMaterialPolicy, TargetGUID: targetGUID, Before: before, Investment: json.RawMessage(raw)})
	if err != nil {
		return nil, err
	}
	// The caller adds these paid costs to the target in this same save. The
	// consumed GUID has no active refund ledger after that atomic transfer.
	p5SetKV(g, constUtil.P5TreasureMaterialAuditPrefix+material.GetGuid(), string(audit))
	p5SetKV(g, "p5.invest:"+material.GetGuid(), "")
	return costs, nil
}
