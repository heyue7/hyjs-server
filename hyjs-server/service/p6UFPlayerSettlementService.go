package service

import (
	"context"
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// FreezeP6UFPlayerInput retains the original pre-result typed14 pair and its
// explicit published factor policy. Actual war/HP/owner facts belong to the
// producer transaction; this API never projects a missing HP or winner.
func (s *GameService) FreezeP6UFPlayerInput(source *pb.SCUFFightRecordReplay) (*model.FrozenBattleInput, error) {
	input, err := p6UFPlayerTypedSource(source)
	if err != nil {
		return nil, err
	}
	return s.FreezeBattleInput(*input)
}

// No tables or caller-provided frozen snapshot can enter this typed constructor.
func p6UFPlayerTypedSource(source *pb.SCUFFightRecordReplay) (*model.FrozenBattleInput, error) {
	if source == nil || source.GetFightId() == "" || source.WinPlayerId != nil || len(source.PlayerList) != 2 || source.AttackerDamageFactor == nil || source.DefenderDamageFactor == nil || len(source.ProtoReflect().GetUnknown()) != 0 || source.GetRoomId() != "" && source.GetRoomId() != "map_forest_002" {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	for _, player := range source.PlayerList {
		if player == nil || player.Inst == nil || len(player.Formation) == 0 {
			return nil, errors.New(constUtil.CombatInvalid)
		}
		for _, hero := range player.Formation {
			if hero == nil || hero.Prop == nil || hero.Inst == nil {
				return nil, errors.New(constUtil.CombatInvalid)
			}
		}
	}
	maximum := source.GetMaxRound()
	if maximum <= 0 {
		maximum = 40
	}
	first, second := source.PlayerList[0].GetInst().GetFirstOrder(), source.PlayerList[1].GetInst().GetFirstOrder()
	return &model.FrozenBattleInput{Attacker: source.PlayerList[0], Defender: source.PlayerList[1], Seed: source.GetRandomSeed(), MaxRound: maximum, BattleType: 14, IsAttToDef: false, DefenderFirst: first == 0 && second == 1, AttackerDamageFactor: source.AttackerDamageFactor, DefenderDamageFactor: source.DefenderDamageFactor}, nil
}

// The explicit diagnostic entry preserves original ordinary-player mode14 and returns no
// settlement authority. Other wrappers retain their existing protocol gates.
func (s *GameService) ComputeOriginalP6UFPlayerProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 14 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	return s.computeFrozenBattleFinalHP(ctx, input, nil, true)
}

func validateP6UFPlayerScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 14 || input.Seed <= 0 || input.MaxRound != 40 || input.DefenderFirst || input.Attacker == nil || input.Defender == nil || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 || input.KFReplayViewer != nil || input.DiagnosticManualAutoStartRoundTime != nil || input.AttackerDamageFactor == nil || input.DefenderDamageFactor == nil || *input.AttackerDamageFactor != 1 || *input.DefenderDamageFactor != 1 {
		return errors.New(constUtil.CombatUnsupported)
	}
	// Team additions have no independently closed ordinary14 producer/effect.
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.Inst == nil || len(player.Inst.ExtBattleAdds) != 0 {
			return errors.New(constUtil.CombatUnsupported)
		}
	}
	// The independently observed wrapper has full40 and original initiative1/0.
	// Positive random seeds remain variable; no observed seed or panel is selected.
	// Factors remain explicit1/1; historical factor0 and injured audit inputs
	// cannot acquire this separate ordinary-player capability.
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

// ComputeP6UFPlayerSettlement is the independently gated original typed14 capability.
// Empty proof rejects every candidate before any business consumer applies cost.
func (s *GameService) ComputeP6UFPlayerSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPlayerScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6UFPlayerNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	out, err := s.ComputeOriginalP6UFPlayerProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6UFPlayerTerminal(frozen, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = constUtil.P6UFPlayerNativeProof
	return out, nil
}

func p6KnownUFPlayerSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return constUtil.P6UFPlayerNativeProof != "" && proof == constUtil.P6UFPlayerNativeProof && validateP6UFPlayerScope(input) == nil
}

func validateP6UFPlayerTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if err := validateP6UFPlayerScope(input); err != nil {
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
