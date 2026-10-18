package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
)

// validateP12FieldPVPScope preserves original type5 and complete current HP/AP.
// Only a private eligibility clone uses the shared original047 actor validator;
// no type11/14 proof or normalized actor reaches the calculation or its digest.
func validateP12FieldPVPScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.BattleType != 5 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.MaxRound != 40 || input.Seed < 0 || input.Seed > math.MaxInt32 || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 {
		return reject()
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	for _, player := range []*pb.BattlePlayer{view.Attacker, view.Defender} {
		if player == nil || player.Inst == nil || len(player.Inst.ExtBattleAdds) != 0 || len(player.Formation) != 1 {
			return reject()
		}
		ap := player.Inst.GetAnger()
		if !combatFinite(ap) || ap < 0 || ap > 300 {
			return reject()
		}
		actor := player.Formation[0]
		if actor == nil || actor.Prop == nil || actor.Inst == nil || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() <= 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
			return reject()
		}
		player.Inst.Anger = proto.Float64(0)
		actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp())
	}
	return validateP6Starter047Scope(view)
}
func p12FieldPVPLegacyProofForInput(input *model.FrozenBattleInput) (string, error) {
	if err := validateP12FieldPVPScope(input); err != nil {
		return "", err
	}
	if constUtil.P12FieldPVPNativeProof == "" {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	return constUtil.P12FieldPVPNativeProof, nil
}

// ComputeP12FieldPVPProtocolFinalHP computes the actual field wrapper with no hints.
func (s *GameService) ComputeP12FieldPVPProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	proof, err := P12FieldPVPProofForInput(input)
	if err != nil {
		return nil, err
	}
	if proof == constUtil.P12FieldPVPMechanismProof {
		return s.computeP12MechanismFinalHP(ctx, input, 5)
	}
	out, err := s.computeFrozenBattleFinalHP(ctx, input, nil, true)
	if err != nil {
		return nil, err
	}
	if err = validateP12FieldPVPTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	return out, nil
}
func validateP12FieldPVPTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if err := validateP12FieldPVPScope(input); err != nil {
		return err
	}
	if err := validateP6Starter047Terminal(input, out); err != nil {
		return err
	}
	if out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	for _, actor := range out.FinalActors {
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.BloodNumber != 1 {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}
func ValidateP12FieldPVPCertifiedReceipt(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if out != nil && out.Outcome != nil && out.Outcome.CapabilityHash == constUtil.P12FieldPVPMechanismProof {
		return p12MechanismTerminal(input, out, 5)
	}

	proof, err := p12FieldPVPLegacyProofForInput(input)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return validateP12FieldPVPTerminal(input, out)
}

// New inputs retain the old native family when eligible; otherwise use the
// separately named local mechanism compatibility family for this exact mode.
func P12FieldPVPProofForInput(input *model.FrozenBattleInput) (string, error) {
	if proof, err := p12FieldPVPLegacyProofForInput(input); err == nil {
		return proof, nil
	}
	if err := p12MechanismScope(input, 5); err != nil {
		return "", err
	}
	return constUtil.P12FieldPVPMechanismProof, nil
}
