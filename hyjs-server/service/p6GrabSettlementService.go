package service

import (
	"context"
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// FreezeP6GrabInput freezes the original pre-result SCGrab player pair. Slot
// ownership, fees, expiry and result receipts remain the producer's transaction.
func (s *GameService) FreezeP6GrabInput(source *pb.SCGrab) (*model.FrozenBattleInput, error) {
	if source == nil || source.GetFightId() == "" || source.WinPlayerId != nil || source.AttPlayer == nil || source.DefPlayer == nil || source.AttPlayer.Inst == nil || source.DefPlayer.Inst == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	for _, player := range []*pb.BattlePlayer{source.AttPlayer, source.DefPlayer} {
		for _, hero := range player.Formation {
			if hero == nil || hero.Prop == nil || hero.Inst == nil {
				return nil, errors.New(constUtil.CombatInvalid)
			}
		}
	}
	maximum := source.GetMaxRoundNum()
	if maximum <= 0 {
		maximum = 40
	}
	// Original SCGrab reads initiative from the raw pair, before either fighter.
	first, second := source.AttPlayer.GetInst().GetFirstOrder(), source.DefPlayer.GetInst().GetFirstOrder()
	return s.FreezeBattleInput(model.FrozenBattleInput{Attacker: source.AttPlayer, Defender: source.DefPlayer, Seed: source.GetRandSeed(), MaxRound: maximum, BattleType: 19, IsAttToDef: false, DefenderFirst: first == 0 && second == 1})
}

// The explicit diagnostic entry preserves original mode19 and returns no
// settlement authority. Other wrappers retain their existing protocol gates.
func (s *GameService) ComputeOriginalP6GrabProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 19 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	return s.computeFrozenBattleFinalHP(ctx, input, nil, true)
}

func validateP6GrabScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 19 || input.Seed <= 0 || input.MaxRound != 40 || input.DefenderFirst || input.Attacker == nil || input.Defender == nil || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 || input.KFReplayViewer != nil || input.DiagnosticManualAutoStartRoundTime != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	// The independently observed wrapper has full40 and original initiative1/0.
	// Positive random seeds remain variable; no observed seed or panel is selected.
	// This view reuses recovered actor/effect validation only. Calculation source
	// and original callback type are never changed or borrowed as a certificate.
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	if validateP6Starter047Scope(view) == nil {
		return nil
	}
	return validateP6ActualPartyScope(view)
}

// ComputeP6GrabSettlement is the independently gated original type19 capability.
// Empty proof rejects every candidate before any business consumer applies cost.
func (s *GameService) ComputeP6GrabSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6GrabScope(frozen); err != nil {
		return s.ComputeP11GenericMechanismSettlement(ctx, frozen)
	}
	if constUtil.P6GrabNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6GrabProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6GrabTerminal(frozen, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6GrabNativeProof
	return out, nil
}

func p6KnownGrabSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	if input != nil && input.BattleType == 19 && p11KnownGenericMechanismProof(input, proof) {
		return true
	}
	return constUtil.P6GrabNativeProof != "" && proof == constUtil.P6GrabNativeProof && validateP6GrabScope(input) == nil
}

func validateP6GrabTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if out != nil && out.Outcome != nil && p11KnownGenericMechanismProof(input, out.Outcome.CapabilityHash) {
		return validateP11GenericMechanismTerminal(input, out)
	}
	if err := validateP6GrabScope(input); err != nil {
		return err
	}
	var err error
	if len(input.Attacker.Formation) == 1 {
		err = validateP6Starter047Terminal(input, out)
	} else {
		err = validateP6ActualPartyTerminal(input, out)
	}
	if err != nil {
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
