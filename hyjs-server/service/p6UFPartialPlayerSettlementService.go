package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Current war HP/AP is a separate capability. Normalizing a private eligibility
// clone reuses only closed actor/table validation; the actual calculation and
// source hash always retain the original current HP/AP and original mode14.
func validateP6UFPartialPlayerScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.Attacker == nil || input.Defender == nil {
		return reject()
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	retained := false
	for _, player := range []*pb.BattlePlayer{view.Attacker, view.Defender} {
		if player.Inst == nil || len(player.Formation) != 1 {
			return reject()
		}
		ap := player.Inst.GetAnger()
		// Original TableGameConfigure.UTIL_MAXANGER=300; keep finite fractional AP.
		if !combatFinite(ap) || ap < 0 || ap > 300 {
			return reject()
		}
		if ap != 0 {
			retained = true
		}
		player.Inst.Anger = proto.Float64(0)
		actor := player.Formation[0]
		if actor == nil || actor.Prop == nil || actor.Inst == nil || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() <= 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
			return reject()
		}
		if actor.Inst.GetCurHp() != actor.Prop.GetHp() {
			retained = true
		}
		actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp())
	}
	if !retained {
		return reject()
	}
	return validateP6UFPlayerScope(view)
}

// ComputeP6UFPartialPlayerSettlement leaves historical fullHP192 proofs intact.
// The independent original current-war source comparison gates this entry;
// current membership/war/HP provenance still belongs to the consumer TX.
func (s *GameService) ComputeP6UFPartialPlayerSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPartialPlayerScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6UFPartialPlayerNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6UFPlayerProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPartialPlayerTerminal(frozen, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6UFPartialPlayerNativeProof
	return out, nil
}

func p6KnownUFPartialPlayerSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return proof != "" && constUtil.P6UFPartialPlayerNativeProof != "" && proof == constUtil.P6UFPartialPlayerNativeProof && validateP6UFPartialPlayerScope(input) == nil
}

func validateP6UFPartialPlayerTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if err := validateP6UFPartialPlayerScope(input); err != nil {
		return err
	}
	// Single original047 retains complete original skills/ult/Buff380. This
	// validator checks real initial actor SHA, whole protocol and all finalHP.
	if err := validateP6Starter047Terminal(input, out); err != nil {
		return err
	}
	if out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
