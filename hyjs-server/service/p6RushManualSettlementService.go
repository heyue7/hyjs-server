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

// Admit only the independently observed live Trial driver. The frozen typed
// source retains manual initialization; an automatic view only reuses the sealed
// actor and effect validator, without changing the source or running that view.
func validateP6RushManualScope(series *model.FrozenP6RushSeries) error {
	if series == nil || series.Template == nil || series.PlayerAutomatic || (series.Trial && series.Template.BattleType != 9 || !series.Trial && series.Template.BattleType != 10) || series.Template.DiagnosticAIDriverPolicy != "" || series.Template.DiagnosticManualAutoStartRoundTime != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	view := *series
	view.PlayerAutomatic = true
	return validateP6RushStarterScope(&view)
}

// The first actual player command supplies a candidate control time. The whole
// supplied record is never used as playback instructions: the original live AI
// must independently reproduce every camp, clock, target, skill and final HP.
func p6RushManualCalculationInput(series *model.FrozenP6RushSeries, supplied *pb.BattleOperate) (*model.FrozenBattleInput, error) {
	if err := validateP6RushManualScope(series); err != nil {
		return nil, err
	}
	if supplied == nil || supplied.AttackerTransOpt != nil || supplied.DefenderTransOpt != nil || len(supplied.SkillOpts) == 0 {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	first := supplied.SkillOpts[0]
	if first == nil || first.GetCamp() != 0 || first.GetRoundNum() != 1 || first.GetActorUid() != series.Template.Attacker.Formation[0].GetGuid() || first.GetSkillIndex() != 0 || !combatFinite(first.GetRoundTime()) || first.GetRoundTime() < 0 || first.GetRoundTime() > 110 {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	input, err := cloneP6CalculationInput(series.Template)
	if err != nil {
		return nil, err
	}
	input.DiagnosticAIDriverPolicy = constUtil.P6DiagnosticManualAutoStart
	input.DiagnosticManualAutoStartRoundTime = proto.Float64(first.GetRoundTime())
	if err = ValidateP6AIDriverPolicy(input); err != nil {
		return nil, err
	}
	return input, nil
}

func (s *GameService) ComputeP6RushManualAutoSettlement(ctx context.Context, series *model.FrozenP6RushSeries, supplied *pb.BattleOperate, reportedResult int32) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	input, err := p6RushManualCalculationInput(series, supplied)
	if err != nil {
		return nil, err
	}
	proof := p6RushManualProof(series)
	if proof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	// Keep the original record outside the calculation. In particular this does
	// not select EnginePlaybackAI, shift clocks, or resubmit player commands.
	out, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6RushStarterTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = ValidateP6RushManualAutoEnd(series, out, supplied, reportedResult); err != nil {
		return nil, err
	}
	return out, nil
}

// Read-only receipt validation reconstructs just the immutable calculation
// source. It does not rerun an accepted battle after an engine/config change.
func ValidateP6RushManualAutoEnd(series *model.FrozenP6RushSeries, terminal *model.CombatFinalHPOutcome, supplied *pb.BattleOperate, reportedResult int32) error {
	input, err := p6RushManualCalculationInput(series, supplied)
	if err != nil {
		return err
	}
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	proof := p6RushManualProof(series)
	if proof == "" || terminal == nil || terminal.Outcome == nil || !terminal.Outcome.Authoritative || terminal.Outcome.CapabilityHash != proof || terminal.Schema != constUtil.P6FinalActorHPSchema || terminal.EngineVersion != input.EngineVersion || terminal.ClientVersion != input.ClientVersion || terminal.ConfigurationHash != input.ConfigurationHash {
		return reject()
	}
	if err = validateP6RushStarterTerminal(input, terminal); err != nil {
		return err
	}
	input.Tables = nil
	raw, err := json.Marshal(input)
	if err != nil || p6Digest(raw) != terminal.CalculationSourceSHA256 {
		return reject()
	}
	expected := int32(0)
	if terminal.Outcome.Winner == 0 {
		expected = 1
	}
	if reportedResult != expected || !p6RushStarterStrictRecordEqual(supplied, terminal.OriginalOperates) {
		return reject()
	}
	return nil
}

// Keep Trial's historical capability byte-for-byte stable. Elite uses a distinct
// certificate and never changes an already accepted Trial row's identity.
func p6RushManualProof(series *model.FrozenP6RushSeries) string {
	if series == nil || series.Template == nil {
		return ""
	}
	if series.Trial && series.Template.BattleType == 9 {
		return constUtil.P6RushManualNativeProof
	}
	if !series.Trial && series.Template.BattleType == 10 {
		return constUtil.P6RushEliteManualNativeProof
	}
	return ""
}
